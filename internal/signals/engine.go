package signals

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/enzotriches/golo/internal/domain"
	"github.com/enzotriches/golo/internal/odds"
)

type Store interface {
	ListArmedStrategies() ([]StoredStrategy, error)
	ListCollectingLockedTests() ([]LockedTest, error)
	AdmitLockedOccurrence(LockedOccurrence) (bool, error)
	ListOpenLockedOccurrencesForMatch(string) ([]LockedOccurrence, error)
	UpdateLockedOccurrenceStatus(string, LockedOccurrenceStatus, time.Time) error
	SaveSignalDecision(Decision) error
	GetSignalByDedupKey(string) (Decision, bool, error)
	ListOpenSignalsForMatch(string) ([]Decision, error)
	UpdateSignalStatus(id string, status Status, resolvedAt time.Time) error
}

type Notifier interface {
	SendSignal(context.Context, Decision) error
	SendResolution(context.Context, Decision) error
}

type Engine struct {
	store               Store
	notifier            Notifier
	mu                  sync.RWMutex
	settings            Settings
	modelQualified      map[int]bool
	modelContract       ModelContract
	baselineProbability func(domain.MatchState, int) float64
	hardEnabled         bool
	hardTelegram        bool
	now                 func() time.Time
}

func NewEngine(
	store Store,
	notifier Notifier,
	settings Settings,
	modelQualified map[int]bool,
	modelContract ModelContract,
	baselineProbability func(domain.MatchState, int) float64,
	hardEnabled, hardTelegram bool,
) *Engine {
	return &Engine{
		store: store, notifier: notifier, settings: settings, modelQualified: modelQualified,
		modelContract: modelContract, baselineProbability: baselineProbability,
		hardEnabled: hardEnabled, hardTelegram: hardTelegram, now: time.Now,
	}
}

func (e *Engine) UpdateSettings(settings Settings) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.settings = settings
}

func (e *Engine) Health() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return map[string]any{
		"oneGoalModelQualified":     e.modelQualified[1],
		"twoGoalModelQualified":     e.modelQualified[2],
		"alertDeploymentEnabled":    e.hardEnabled,
		"telegramDeploymentEnabled": e.hardTelegram,
		"settings":                  e.settings,
	}
}

// Evaluate considers every armed strategy that matches the current live
// state. Rejected decisions are upserted for auditability; Telegram receives
// only qualified decisions.
func (e *Engine) Evaluate(ctx context.Context, match domain.Match, state domain.MatchState, prediction domain.Prediction, event odds.Event) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if err := e.evaluateLocked(match, state, prediction, event); err != nil {
		return err
	}
	strategies, err := e.store.ListArmedStrategies()
	if err != nil {
		return err
	}
	now := e.now()
	for _, stored := range strategies {
		def := stored.Definition
		if !stored.Armed || !def.Enabled || !def.MatchesLive(state) {
			continue
		}
		line := float64(state.Score.Home+state.Score.Away+def.AdditionalGoals) - 0.5
		quote, found := odds.FindTotalsQuote(event, e.settings.Bookmaker, line)
		decision := Decision{
			ID: newID(), StrategyID: def.ID, StrategyVersion: def.Version, StrategyName: def.Name,
			MatchID: match.ID, HomeTeam: match.HomeTeamName, AwayTeam: match.AwayTeamName,
			Competition: match.CompetitionName, MatchSecond: state.ClockSeconds,
			StartGoals: state.Score.Home + state.Score.Away, AdditionalGoals: def.AdditionalGoals,
			MarketLine: line, Prediction: prediction, State: state, Qualification: stored.Report,
			Gates: map[string]bool{}, CreatedAt: now, Status: StatusRejected,
		}
		decision.DedupKey = fmt.Sprintf("%s:%s:%d:%.1f", match.ID, def.ID, def.Version, line)
		if found {
			decision.Quote = quote
		}

		e.gate(&decision, def.MinimumOdds, def.MinimumModelEdge, found, now, e.settings, true)
		if len(decision.Failures) == 0 {
			decision.Status = StatusQualified
		}
		sort.Strings(decision.Failures)

		previous, exists, err := e.store.GetSignalByDedupKey(decision.DedupKey)
		if err != nil {
			return err
		}
		if exists && previous.Status != StatusRejected {
			continue
		}
		if exists {
			decision.ID = previous.ID
			decision.CreatedAt = previous.CreatedAt
		}
		if err := e.store.SaveSignalDecision(decision); err != nil {
			return err
		}
		if decision.Status == StatusQualified && e.hardEnabled && e.hardTelegram &&
			e.settings.AlertEngineEnabled && e.settings.TelegramEnabled && e.notifier != nil {
			if err := e.notifier.SendSignal(ctx, decision); err != nil {
				return fmt.Errorf("send signal: %w", err)
			}
		}
	}
	return nil
}

func (e *Engine) evaluateLocked(match domain.Match, state domain.MatchState, prediction domain.Prediction, event odds.Event) error {
	tests, err := e.store.ListCollectingLockedTests()
	if err != nil {
		return err
	}
	now := e.now()
	for _, test := range tests {
		def := test.Contract.Definition
		if now.Before(test.StartedAt) || !def.Enabled || !def.MatchesLive(state) {
			continue
		}
		if prediction.ModelVersion != test.Contract.Model.ModelVersion ||
			prediction.FeatureVersion != test.Contract.Model.FeatureVersion ||
			e.modelContract.ModelSHA256 != test.Contract.Model.ModelSHA256 {
			continue
		}
		line := float64(state.Score.Home+state.Score.Away+def.AdditionalGoals) - 0.5
		quote, found := odds.FindTotalsQuote(event, test.Contract.Bookmaker, line)
		decision := Decision{
			ID: newID(), StrategyID: def.ID, StrategyVersion: def.Version, StrategyName: def.Name,
			MatchID: match.ID, HomeTeam: match.HomeTeamName, AwayTeam: match.AwayTeamName,
			Competition: match.CompetitionName, MatchSecond: state.ClockSeconds,
			StartGoals: state.Score.Home + state.Score.Away, AdditionalGoals: def.AdditionalGoals,
			MarketLine: line, Prediction: prediction, State: state,
			Qualification: test.ValidationReport, Gates: map[string]bool{},
			CreatedAt: now, Status: StatusRejected,
		}
		if found {
			decision.Quote = quote
		}
		lockedSettings := e.settings
		lockedSettings.Bookmaker = test.Contract.Bookmaker
		lockedSettings.MinimumDataQuality = test.Contract.MinimumDataQuality
		lockedSettings.MaximumFeedLag = test.Contract.MaximumFeedLag
		lockedSettings.MaximumQuoteAge = test.Contract.MaximumQuoteAge
		lockedSettings.PostGoalCooldown = test.Contract.PostGoalCooldown
		lockedSettings.AllowTwoGoalAlerts = test.Contract.AllowTwoGoalAlerts
		e.gate(&decision, def.MinimumOdds, def.MinimumModelEdge, found, now, lockedSettings, false)
		sort.Strings(decision.Failures)
		modelProbability := prediction.Probabilities.GoalBeforeFullTime
		if def.AdditionalGoals == 2 {
			modelProbability = prediction.Probabilities.TwoOrMoreBeforeFullTime
		}
		baseline := 0.0
		if e.baselineProbability != nil {
			baseline = e.baselineProbability(state, def.AdditionalGoals)
		}
		_, err := e.store.AdmitLockedOccurrence(LockedOccurrence{
			ID: newID(), TestID: test.ID, MatchID: match.ID,
			SignalEligible: len(decision.Failures) == 0,
			Status:         LockedOccurrenceOpen, TriggerSecond: state.ClockSeconds,
			StartGoals: state.Score.Home + state.Score.Away, AdditionalGoals: def.AdditionalGoals,
			ModelProbability: modelProbability, BaselineProbability: baseline,
			MarketProbability: decision.MarketProbability, Quote: decision.Quote,
			StateSnapshot: state, Prediction: prediction, Gates: decision.Gates,
			GateFailures: append([]string(nil), decision.Failures...), CreatedAt: now,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) gate(d *Decision, minimumOdds, minimumEdge float64, quoteFound bool, now time.Time, settings Settings, requireLocked bool) {
	add := func(name string, passed bool) {
		d.Gates[name] = passed
		if !passed {
			d.Failures = append(d.Failures, name)
		}
	}
	add("model_qualified", e.modelQualified[d.AdditionalGoals])
	add("market_enabled", d.AdditionalGoals == 1 || settings.AllowTwoGoalAlerts)
	add("historical_validation", d.Qualification.ValidationQualified && d.Qualification.ModelValidationQualified)
	if requireLocked {
		add("locked_test_qualification", d.Qualification.Qualified)
	}
	add("data_quality", d.Prediction.DataQuality >= settings.MinimumDataQuality)
	add("prediction_ok", d.Prediction.Status == domain.PredictionStatusOK)
	add("feed_fresh", time.Duration(d.State.FeedLagMs)*time.Millisecond <= settings.MaximumFeedLag)
	add("quote_available", quoteFound)
	if !quoteFound {
		return
	}
	add("market_active", !d.Quote.Suspended && !d.Quote.Stopped)
	add("quote_timestamp", !d.Quote.UpdatedAt.IsZero())
	add("quote_fresh", !d.Quote.UpdatedAt.IsZero() && now.Sub(d.Quote.UpdatedAt) >= 0 && now.Sub(d.Quote.UpdatedAt) <= settings.MaximumQuoteAge)
	add("score_matches_quote", d.Quote.HomeScore == d.State.Score.Home && d.Quote.AwayScore == d.State.Score.Away)
	add("deep_link", d.Quote.DeepLink != "")
	add("minimum_odds", d.Quote.Over >= minimumOdds)
	if d.State.LastGoalSecond == nil {
		add("post_goal_cooldown", true)
	} else {
		add("post_goal_cooldown", time.Duration(d.State.ClockSeconds-*d.State.LastGoalSecond)*time.Second >= settings.PostGoalCooldown)
	}
	marketProbability, err := odds.FairOverProbability(d.Quote.Over, d.Quote.Under)
	add("paired_market", err == nil)
	if err != nil {
		return
	}
	d.MarketProbability = marketProbability
	if d.AdditionalGoals == 1 {
		d.ModelProbability = d.Prediction.Probabilities.GoalBeforeFullTime
	} else {
		d.ModelProbability = d.Prediction.Probabilities.TwoOrMoreBeforeFullTime
	}
	d.Edge = d.ModelProbability - d.MarketProbability
	add("model_edge", d.Edge >= minimumEdge)
	validation := d.Qualification.Validation
	if validation.MatchCount == 0 {
		validation = d.Qualification.Holdout
	}
	add("conservative_price", validation.RateLow > 1/d.Quote.Over)
}

func (e *Engine) Settle(ctx context.Context, state domain.MatchState) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	open, err := e.store.ListOpenSignalsForMatch(state.MatchID)
	if err != nil {
		return err
	}
	now := e.now()
	locked, err := e.store.ListOpenLockedOccurrencesForMatch(state.MatchID)
	if err != nil {
		return err
	}
	for _, occurrence := range locked {
		status := LockedOccurrenceStatus("")
		added := state.Score.Home + state.Score.Away - occurrence.StartGoals
		if added >= occurrence.AdditionalGoals {
			status = LockedOccurrenceWon
		} else {
			switch state.Status {
			case domain.MatchStatusFinished:
				status = LockedOccurrenceLost
			case domain.MatchStatusCancelled, domain.MatchStatusError:
				status = LockedOccurrenceVoid
			}
		}
		if status != "" {
			if err := e.store.UpdateLockedOccurrenceStatus(occurrence.ID, status, now); err != nil {
				return err
			}
		}
	}
	for _, decision := range open {
		status := Status("")
		added := state.Score.Home + state.Score.Away - decision.StartGoals
		if added >= decision.AdditionalGoals {
			status = StatusWon
		} else {
			switch state.Status {
			case domain.MatchStatusFinished:
				status = StatusLost
			case domain.MatchStatusCancelled, domain.MatchStatusError:
				status = StatusVoid
			}
		}
		if status == "" {
			continue
		}
		if err := e.store.UpdateSignalStatus(decision.ID, status, now); err != nil {
			return err
		}
		decision.Status = status
		decision.ResolvedAt = &now
		if e.hardTelegram && e.settings.TelegramEnabled && e.notifier != nil {
			if err := e.notifier.SendResolution(ctx, decision); err != nil {
				return err
			}
		}
	}
	return nil
}

func newID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}
