package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/enzotriches/golo/internal/adminauth"
	"github.com/enzotriches/golo/internal/api"
	"github.com/enzotriches/golo/internal/config"
	"github.com/enzotriches/golo/internal/domain"
	"github.com/enzotriches/golo/internal/evaluation"
	"github.com/enzotriches/golo/internal/eventstore"
	"github.com/enzotriches/golo/internal/features"
	"github.com/enzotriches/golo/internal/odds"
	"github.com/enzotriches/golo/internal/odds/oddsapiio"
	"github.com/enzotriches/golo/internal/odds/parlayapi"
	"github.com/enzotriches/golo/internal/odds/theoddsapi"
	"github.com/enzotriches/golo/internal/predictor"
	"github.com/enzotriches/golo/internal/providers"
	"github.com/enzotriches/golo/internal/providers/mock"
	"github.com/enzotriches/golo/internal/providers/sportmonks"
	"github.com/enzotriches/golo/internal/publisher"
	"github.com/enzotriches/golo/internal/quality"
	"github.com/enzotriches/golo/internal/reducer"
	"github.com/enzotriches/golo/internal/signals"
	"github.com/enzotriches/golo/internal/telegram"
)

func main() {
	log.Println("Starting Golo Engine (Real-Time Football Probability Engine v1.0)...")

	cfg := config.Load()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Initialize SQLite Database EventStore
	store, err := eventstore.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize SQLite store: %v", err)
	}
	defer store.Close()
	log.Println("SQLite EventStore initialized at:", cfg.DBPath)

	// 2. Load Model Artifact & Predictor
	predEngine, err := predictor.NewPredictor(cfg.ModelPath)
	if err != nil {
		log.Fatalf("Failed to load predictor model: %v", err)
	}
	log.Println("Loaded Baseline Model Artifact:", cfg.ModelPath)

	// 3. Initialize Core Domain Processing Components
	fe := features.NewFeatureEngine()
	red := reducer.NewReducer()
	eval := quality.NewEvaluator()
	pub := publisher.NewPublisher(cfg.FirebaseDatabaseURL, cfg.FirebaseAuth)
	tg := telegram.New(cfg.TelegramBotToken, cfg.TelegramWebhookSecret, store)
	signalSettings, err := store.LoadSignalSettings()
	if err != nil {
		log.Fatalf("Failed to load signal settings: %v", err)
	}
	if err := signalSettings.Validate(); err != nil {
		log.Printf("Stored signal settings are invalid (%v); reverting to safe defaults", err)
		signalSettings = signals.DefaultSettings()
	}
	// Environment flags are the safe deployment defaults; the dashboard may
	// subsequently pause/resume within the hard ALERT_ENGINE_ENABLED ceiling.
	signalSettings.Bookmaker = cfg.OddsBookmaker
	if err := store.SaveSignalSettings(signalSettings); err != nil {
		log.Fatalf("Failed to save signal settings: %v", err)
	}
	modelContract := signals.ModelContract{
		ModelVersion: predEngine.ModelVersion(), ModelSHA256: predEngine.SHA256(),
		FeatureVersion:   predEngine.FeatureVersion(),
		OneGoalQualified: predEngine.AlertQualified(1), TwoGoalQualified: predEngine.AlertQualified(2),
		BaselineGoalsPer90: predEngine.BaselineGoalsPer90(),
	}
	signalEngine := signals.NewEngine(store, tg, signalSettings, map[int]bool{
		1: predEngine.AlertQualified(1),
		2: predEngine.AlertQualified(2),
	}, modelContract, predEngine.BaselineProbability, cfg.AlertEngineEnabled, cfg.TelegramEnabled)
	oddsState := &liveOddsCache{provider: "not configured"}
	shadowOddsState := &liveOddsCache{provider: "parlay-api-shadow", configured: false}
	oddsMonitor := &providerMonitor{primary: oddsState, shadow: shadowOddsState}
	if cfg.OddsAPIKey != "" {
		var oddsProvider odds.Provider
		switch cfg.OddsProvider {
		case "the-odds-api":
			oddsProvider = theoddsapi.New(cfg.OddsAPIKey, cfg.OddsAPIBaseURL, cfg.OddsBookmaker, cfg.OddsSportKeys)
		case "odds-api-io":
			oddsProvider = oddsapiio.New(cfg.OddsAPIKey, cfg.OddsAPIBaseURL, cfg.OddsBookmaker)
		default:
			log.Fatalf("unknown ODDS_PROVIDER %q", cfg.OddsProvider)
		}
		oddsState.provider, oddsState.configured = oddsProvider.Name(), true
		go runOddsLoop(ctx, oddsProvider, cfg.OddsPollInterval, oddsState, store, nil)
	}
	if cfg.ParlayShadowEnabled {
		if cfg.ParlayAPIKey == "" {
			log.Println("Parlay shadow requested but PARLAY_API_KEY is missing; shadow collection remains disabled")
		} else {
			shadowProvider := parlayapi.New(
				cfg.ParlayAPIKey, cfg.ParlayAPIBaseURL, cfg.ParlayBookmaker, cfg.ParlaySportKeys,
			)
			shadowOddsState.provider, shadowOddsState.configured = shadowProvider.Name(), true
			go runOddsLoop(ctx, shadowProvider, cfg.OddsPollInterval, shadowOddsState, store, func() {
				oddsMonitor.CompareAndPersist(store)
			})
		}
	}

	// 4. Initialize Data Provider (selected via PROVIDER env var: mock | sportmonks)
	prov := newProvider(cfg)
	log.Printf("Data Provider initialized: %s", prov.Name())

	// 5. Ingestion & Inference Loop
	go runIngestionLoop(ctx, prov, cfg.PollInterval, store, red, fe, eval, predEngine, pub, signalEngine, oddsState)

	// 6. Start HTTP API Server
	addr := fmt.Sprintf(":%s", cfg.Port)

	admin := adminauth.New(cfg.AdminPasswordHash, cfg.AdminSessionSecret, 8*time.Hour)
	server := api.NewServerWithAdmin(store, pub, nil, predEngine.ModelVersion(), api.AdminDependencies{
		Auth: admin, Telegram: tg, SignalEngine: signalEngine,
		DatasetPath: cfg.HistoricalDatasetPath, AllowedOrigin: cfg.AllowedWebOrigin,
		ProviderHealth: oddsMonitor.Health, ModelContract: modelContract,
	})

	go func() {
		log.Printf("Golo HTTP API Server running at http://localhost%s", addr)
		if err := server.ListenAndServe(addr); err != nil {
			log.Printf("Server stopped: %v", err)
		}
	}()

	// 7. Handle Graceful Shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down Golo engine gracefully...")
}

func runIngestionLoop(
	ctx context.Context,
	prov providers.Provider,
	pollInterval time.Duration,
	store *eventstore.SQLiteStore,
	red *reducer.Reducer,
	fe *features.FeatureEngine,
	eval *quality.Evaluator,
	predEngine *predictor.Predictor,
	pub *publisher.Publisher,
	signalEngine *signals.Engine,
	oddsState *liveOddsCache,
) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Keep track of match state and last-seen provider event ID per match.
	// The cursor matters for providers (e.g. sportmonks) that return the
	// full cumulative event list on every call rather than only new events
	// since the last call (which the in-process mock provider does) — without
	// it, every already-seen event would be re-reduced into state on every
	// poll tick.
	matchStates := make(map[string]domain.MatchState)
	lastEventIDs := make(map[string]string)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			matches, err := prov.ListLiveMatches(ctx)
			if err != nil {
				log.Printf("Error fetching live matches: %v", err)
				continue
			}

			for _, m := range matches {
				if err := store.SaveMatch(m); err != nil {
					log.Printf("Failed to save match: %v", err)
				}

				// A live-match feed may also return fixtures that haven't
				// kicked off yet or have just ended. Predicting on those
				// publishes a probability for a match that isn't running.
				if !isPredictable(m.Status) {
					if signalEngine != nil {
						if state, ok := matchStates[m.ID]; ok {
							state.Status = m.Status
							_ = signalEngine.Settle(ctx, state)
						}
					}
					continue
				}

				state, exists := matchStates[m.ID]
				if !exists {
					state = domain.InitialState(m)
				}

				// Fetch only events newer than the last one we've already reduced.
				events, err := prov.FetchEventsSince(ctx, m.ID, lastEventIDs[m.ID])
				if err != nil {
					log.Printf("Failed to fetch events for %s: %v", m.ID, err)
				} else if len(events) > 0 {
					lastEventIDs[m.ID] = highestProviderEventID(lastEventIDs[m.ID], events)

					// 1. Save canonical events
					if err := store.SaveEvents(events); err != nil {
						log.Printf("Failed to save events: %v", err)
					}

					// 2. Reduce state deterministically
					for _, ev := range events {
						state = red.Reduce(state, ev)
					}
				}

				// 2.5. Fold in the provider's authoritative view — clock,
				// status, score, cumulative statistics. This runs on every
				// tick, including ticks with no new events, because the match
				// clock advances whether or not anything happened: without it
				// every horizon would stay frozen at the last goal or card.
				// It runs after the events so that where a provider publishes
				// an authoritative value, that value wins over the reduction.
				if snap, ok := prov.Snapshot(m.ID); ok {
					state = red.ApplySnapshot(state, snap)
				} else if len(events) == 0 {
					// Event-only provider with nothing new: state is unchanged,
					// so re-running the model would just republish the same
					// numbers.
					continue
				}

				matchStates[m.ID] = state

				// 3. Save match state
				if err := store.SaveMatchState(state); err != nil {
					log.Printf("Failed to save match state: %v", err)
				}

				// 4. Extract features
				feats, snapshot, err := fe.ExtractFeatures(state)
				if err != nil {
					log.Printf("Failed to extract features: %v", err)
					continue
				}
				if err := store.SaveSnapshot(snapshot); err != nil {
					// Discarded silently until now, which would have left holes
					// in the stored feature history with nothing to notice them.
					log.Printf("Failed to save feature snapshot: %v", err)
				}

				// 5. Evaluate data quality
				qualityScore := eval.EvaluateDataQuality(state)

				// 6. Predict probabilities
				pred, err := predEngine.Predict(state, feats, qualityScore)
				if err != nil {
					log.Printf("Failed to predict probabilities: %v", err)
					continue
				}

				// 7. Save prediction
				if err := store.SavePrediction(pred); err != nil {
					log.Printf("Failed to save prediction: %v", err)
				}
				if signalEngine != nil {
					matched, _ := odds.MatchEvent(m, state, oddsState.Events())
					if err := signalEngine.Evaluate(ctx, m, state, pred, matched); err != nil {
						log.Printf("Signal evaluation failed for %s: %v", m.ID, err)
					}
					if err := signalEngine.Settle(ctx, state); err != nil {
						log.Printf("Signal settlement failed for %s: %v", m.ID, err)
					}
				}

				// 7.5. Compute this match's own "goal in next 10m" track
				// record, for the per-card "how right have we been" meter.
				trackRecord := publisher.TrackRecord{}
				outcome, err := store.GetOutcomeForMatch(m.ID)
				if err != nil {
					log.Printf("Failed to load track record data: %v", err)
				} else if matchPredictions, err := store.GetMatchPredictions(m.ID); err != nil {
					log.Printf("Failed to load match predictions for track record: %v", err)
				} else {
					tr := evaluation.ComputeMatchTrackRecord(matchPredictions, outcome)
					trackRecord = publisher.TrackRecord{AccuracyPct: tr.AccuracyPct, ResolvedCount: tr.ResolvedCount}
				}

				// 8. Publish update
				if err := pub.Publish(ctx, state, pred, trackRecord); err != nil {
					log.Printf("Failed to publish update: %v", err)
				}
			}
		}
	}
}

type liveOddsCache struct {
	mu          sync.RWMutex
	events      []odds.Event
	lastSuccess time.Time
	lastError   time.Time
	errorCount  int
	message     string
	provider    string
	configured  bool
	observation odds.ProviderObservation
}

func (c *liveOddsCache) Merge(events []odds.Event, observation odds.ProviderObservation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	byID := make(map[string]odds.Event, len(c.events)+len(events))
	for _, event := range c.events {
		byID[event.ID] = event
	}
	for _, event := range events {
		byID[event.ID] = event
	}
	c.events = c.events[:0]
	for _, event := range byID {
		c.events = append(c.events, event)
	}
	c.lastSuccess, c.message = time.Now(), ""
	c.observation = observation
}

func (c *liveOddsCache) RecordError(err error, observation odds.ProviderObservation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastError, c.errorCount, c.message = time.Now(), c.errorCount+1, err.Error()
	c.observation = observation
}

func (c *liveOddsCache) Events() []odds.Event {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]odds.Event(nil), c.events...)
}

func (c *liveOddsCache) Health() any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return map[string]any{
		"provider": c.provider, "configured": c.configured,
		"healthy":       c.configured && !c.lastSuccess.IsZero() && time.Since(c.lastSuccess) < 2*time.Minute,
		"lastSuccessAt": c.lastSuccess, "lastErrorAt": c.lastError,
		"errorCount": c.errorCount, "message": c.message, "cachedEvents": len(c.events),
		"latestObservation": c.observation,
	}
}

type oddsAuditStore interface {
	SaveProviderObservation(odds.ProviderObservation) error
	SaveProviderComparison(odds.ProviderComparison) error
}

func runOddsLoop(
	ctx context.Context,
	provider odds.Provider,
	interval time.Duration,
	cache *liveOddsCache,
	store oddsAuditStore,
	afterSuccess func(),
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		events, err := provider.FetchLive(ctx)
		now := time.Now()
		if err != nil {
			log.Printf("Live odds poll failed: %v", err)
			observation := odds.ProviderObservation{Provider: provider.Name(), ObservedAt: now, Error: err.Error()}
			cache.RecordError(err, observation)
			if store != nil {
				_ = store.SaveProviderObservation(observation)
			}
		} else {
			clean, observation := odds.ValidateSnapshot(provider.Name(), events, now, 2*time.Minute)
			cache.Merge(clean, observation)
			if store != nil {
				_ = store.SaveProviderObservation(observation)
			}
			if afterSuccess != nil {
				afterSuccess()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type providerMonitor struct {
	mu         sync.RWMutex
	primary    *liveOddsCache
	shadow     *liveOddsCache
	comparison odds.ProviderComparison
}

func (m *providerMonitor) CompareAndPersist(store oddsAuditStore) {
	if m == nil || m.primary == nil || m.shadow == nil {
		return
	}
	comparison := odds.CompareSnapshots(
		m.primary.provider, m.primary.Events(), m.shadow.provider, m.shadow.Events(), time.Now(),
	)
	m.mu.Lock()
	m.comparison = comparison
	m.mu.Unlock()
	if store != nil {
		_ = store.SaveProviderComparison(comparison)
	}
}

func (m *providerMonitor) Health() any {
	if m == nil || m.primary == nil {
		return map[string]any{"provider": "not configured", "healthy": false}
	}
	primary, _ := m.primary.Health().(map[string]any)
	m.mu.RLock()
	comparison := m.comparison
	m.mu.RUnlock()
	primary["primary"] = m.primary.Health()
	primary["shadow"] = m.shadow.Health()
	primary["latestComparison"] = comparison
	return primary
}

// isPredictable reports whether a match is in a state where a live goal
// probability is meaningful. A fixture that hasn't kicked off has no elapsed
// time to reason about, and one that's finished, abandoned or unreadable has
// no remaining time — publishing a number for either would be noise.
// Half time counts: the match is paused, but the second half is still to come.
func isPredictable(status domain.MatchStatus) bool {
	switch status {
	case domain.MatchStatusLive, domain.MatchStatusHalfTime, domain.MatchStatusPaused:
		return true
	default:
		return false
	}
}

// newProvider selects the live data provider based on cfg.Provider.
// The SportMonks API key is read only from server-side config here — it must
// never be forwarded to internal/api/server.go or the frontend.
func newProvider(cfg config.Config) providers.Provider {
	switch cfg.Provider {
	case "sportmonks":
		if cfg.SportMonksAPIKey == "" {
			log.Fatal("PROVIDER=sportmonks requires SPORTMONKS_API_KEY to be set")
		}
		return sportmonks.New(cfg.SportMonksAPIKey, cfg.SportMonksBaseURL, cfg.PriorityCompetitions)
	case "mock", "":
		return mock.NewMockProvider()
	default:
		log.Fatalf("unknown PROVIDER %q (expected mock or sportmonks)", cfg.Provider)
		return nil
	}
}

// highestProviderEventID returns the largest numeric ProviderEventID seen
// across current and previously-seen events, so it can be used as the next
// FetchEventsSince cursor. Falls back to the previous cursor if no event ID
// in the batch parses as a number (e.g. the mock provider's "pev_N" IDs,
// which the mock provider ignores as a cursor anyway).
func highestProviderEventID(previous string, events []domain.MatchEvent) string {
	best := previous
	bestVal, bestOK := parseInt64(previous)

	for _, ev := range events {
		val, ok := parseInt64(ev.ProviderEventID)
		if !ok {
			continue
		}
		if !bestOK || val > bestVal {
			best = ev.ProviderEventID
			bestVal = val
			bestOK = true
		}
	}

	return best
}

func parseInt64(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	var v int64
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return 0, false
	}
	return v, true
}
