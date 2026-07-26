package signals

import (
	"context"
	"testing"
	"time"

	"github.com/enzotriches/golo/internal/domain"
	"github.com/enzotriches/golo/internal/odds"
	"github.com/enzotriches/golo/internal/scenario"
)

type memoryStore struct {
	strategies        []StoredStrategy
	decisions         map[string]Decision
	lockedTests       []LockedTest
	lockedOccurrences map[string]LockedOccurrence
}

func (s *memoryStore) ListArmedStrategies() ([]StoredStrategy, error)   { return s.strategies, nil }
func (s *memoryStore) ListCollectingLockedTests() ([]LockedTest, error) { return s.lockedTests, nil }
func (s *memoryStore) AdmitLockedOccurrence(occurrence LockedOccurrence) (bool, error) {
	if s.lockedOccurrences == nil {
		s.lockedOccurrences = map[string]LockedOccurrence{}
	}
	key := occurrence.TestID + ":" + occurrence.MatchID
	if _, exists := s.lockedOccurrences[key]; exists {
		return false, nil
	}
	occurrence.RuleCohort = true
	occurrence.SignalCohort = occurrence.SignalEligible
	s.lockedOccurrences[key] = occurrence
	return true, nil
}
func (s *memoryStore) ListOpenLockedOccurrencesForMatch(matchID string) ([]LockedOccurrence, error) {
	out := make([]LockedOccurrence, 0)
	for _, occurrence := range s.lockedOccurrences {
		if occurrence.MatchID == matchID && occurrence.Status == LockedOccurrenceOpen {
			out = append(out, occurrence)
		}
	}
	return out, nil
}
func (s *memoryStore) UpdateLockedOccurrenceStatus(id string, status LockedOccurrenceStatus, at time.Time) error {
	for key, occurrence := range s.lockedOccurrences {
		if occurrence.ID == id {
			occurrence.Status, occurrence.ResolvedAt = status, &at
			s.lockedOccurrences[key] = occurrence
		}
	}
	return nil
}
func (s *memoryStore) SaveSignalDecision(d Decision) error {
	if s.decisions == nil {
		s.decisions = map[string]Decision{}
	}
	s.decisions[d.DedupKey] = d
	return nil
}
func (s *memoryStore) GetSignalByDedupKey(key string) (Decision, bool, error) {
	d, ok := s.decisions[key]
	return d, ok, nil
}
func (s *memoryStore) ListOpenSignalsForMatch(matchID string) ([]Decision, error) {
	var out []Decision
	for _, d := range s.decisions {
		if d.MatchID == matchID && d.Status == StatusQualified {
			out = append(out, d)
		}
	}
	return out, nil
}
func (s *memoryStore) UpdateSignalStatus(id string, status Status, at time.Time) error {
	for key, d := range s.decisions {
		if d.ID == id {
			d.Status, d.ResolvedAt = status, &at
			s.decisions[key] = d
		}
	}
	return nil
}

type memoryNotifier struct{ signals, resolutions int }

func (n *memoryNotifier) SendSignal(context.Context, Decision) error     { n.signals++; return nil }
func (n *memoryNotifier) SendResolution(context.Context, Decision) error { n.resolutions++; return nil }

func testStrategy() StoredStrategy {
	def := scenario.DefaultDefinition()
	def.ID, def.Name, def.Enabled = "late", "Late goal", true
	def.Conditions = []scenario.Condition{{Field: scenario.FieldMinute, Operator: scenario.OpGreaterOrEqual, Value: 70}}
	return StoredStrategy{
		Definition: def, Armed: true,
		Report: scenario.QualificationReport{
			Qualified: true, ValidationQualified: true, ModelValidationQualified: true,
			Validation: scenario.Result{MatchCount: 200, HitRate: .75, RateLow: .68, RateHigh: .80, OddsHigh: 1.47},
			Holdout:    scenario.Result{MatchCount: 200, HitRate: .75, RateLow: .68, RateHigh: .80, OddsHigh: 1.47},
		},
	}
}

func testSignalInput(now time.Time) (domain.Match, domain.MatchState, domain.Prediction, odds.Event) {
	match := domain.Match{ID: "m1", HomeTeamName: "A", AwayTeamName: "B", CompetitionName: "Liga"}
	state := domain.MatchState{
		MatchID: "m1", Status: domain.MatchStatusLive, ClockSeconds: 4200,
		Score: domain.ScoreState{Home: 1, Away: 1}, FeedLagMs: 100,
	}
	prediction := domain.Prediction{
		Status: domain.PredictionStatusOK, DataQuality: .95,
		Probabilities: domain.Probabilities{GoalBeforeFullTime: .75, TwoOrMoreBeforeFullTime: .40},
	}
	event := odds.Event{Quotes: []odds.Quote{{
		Bookmaker: "Bet365", Market: "totals", Line: 2.5, Over: 1.80, Under: 2.10,
		UpdatedAt: now.Add(-5 * time.Second), DeepLink: "https://book.example/market",
		HomeScore: 1, AwayScore: 1,
	}}}
	return match, state, prediction, event
}

func TestEngineAppliesAllGatesAndDeduplicates(t *testing.T) {
	now := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	store := &memoryStore{strategies: []StoredStrategy{testStrategy()}}
	notifier := &memoryNotifier{}
	settings := DefaultSettings()
	settings.AlertEngineEnabled, settings.TelegramEnabled = true, true
	engine := NewEngine(store, notifier, settings, map[int]bool{1: true}, ModelContract{}, nil, true, true)
	engine.now = func() time.Time { return now }
	match, state, prediction, event := testSignalInput(now)

	if err := engine.Evaluate(context.Background(), match, state, prediction, event); err != nil {
		t.Fatal(err)
	}
	if notifier.signals != 1 || len(store.decisions) != 1 {
		t.Fatalf("send=%d decisions=%d", notifier.signals, len(store.decisions))
	}
	for _, d := range store.decisions {
		if d.Status != StatusQualified || len(d.Failures) != 0 {
			t.Fatalf("qualified input rejected: %+v", d.Failures)
		}
	}
	if err := engine.Evaluate(context.Background(), match, state, prediction, event); err != nil {
		t.Fatal(err)
	}
	if notifier.signals != 1 {
		t.Fatalf("duplicate sent %d times", notifier.signals)
	}
}

func TestEngineRejectsStaleScoreAndPostGoalQuote(t *testing.T) {
	now := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*domain.MatchState, *odds.Event){
		"stale quote": func(_ *domain.MatchState, event *odds.Event) {
			event.Quotes[0].UpdatedAt = now.Add(-2 * time.Minute)
		},
		"score mismatch": func(_ *domain.MatchState, event *odds.Event) {
			event.Quotes[0].HomeScore = 0
		},
		"post goal cooldown": func(state *domain.MatchState, _ *odds.Event) {
			second := state.ClockSeconds - 30
			state.LastGoalSecond = &second
		},
		"suspended market": func(_ *domain.MatchState, event *odds.Event) {
			event.Quotes[0].Suspended = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := &memoryStore{strategies: []StoredStrategy{testStrategy()}}
			engine := NewEngine(store, nil, DefaultSettings(), map[int]bool{1: true}, ModelContract{}, nil, false, false)
			engine.now = func() time.Time { return now }
			match, state, prediction, event := testSignalInput(now)
			mutate(&state, &event)
			if err := engine.Evaluate(context.Background(), match, state, prediction, event); err != nil {
				t.Fatal(err)
			}
			for _, d := range store.decisions {
				if d.Status != StatusRejected {
					t.Fatalf("%s was not rejected", name)
				}
			}
		})
	}
}

func TestSettlementAndFlatStakePerformance(t *testing.T) {
	store := &memoryStore{decisions: map[string]Decision{
		"one": {ID: "s1", DedupKey: "one", MatchID: "m1", Status: StatusQualified, StartGoals: 2, AdditionalGoals: 1, Quote: odds.Quote{Over: 1.8}},
	}}
	notifier := &memoryNotifier{}
	settings := DefaultSettings()
	settings.TelegramEnabled = true
	engine := NewEngine(store, notifier, settings, nil, ModelContract{}, nil, false, true)
	if err := engine.Settle(context.Background(), domain.MatchState{
		MatchID: "m1", Status: domain.MatchStatusLive, Score: domain.ScoreState{Home: 2, Away: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if store.decisions["one"].Status != StatusWon || notifier.resolutions != 1 {
		t.Fatal("winning target did not settle immediately")
	}
	perf := ComputePerformance([]Decision{
		{Status: StatusLost, Quote: odds.Quote{Over: 2}},
		{Status: StatusLost, Quote: odds.Quote{Over: 2}},
		{Status: StatusWon, Quote: odds.Quote{Over: 2}},
	})
	if perf.ROIPct != -100.0/3 || perf.LongestLossStreak != 2 || perf.MaxDrawdownUnits != 2 {
		t.Fatalf("unexpected performance: %+v", perf)
	}
}

func TestCancelledMatchSettlesVoid(t *testing.T) {
	store := &memoryStore{decisions: map[string]Decision{
		"one": {ID: "s1", DedupKey: "one", MatchID: "m1", Status: StatusQualified, StartGoals: 2, AdditionalGoals: 1},
	}}
	engine := NewEngine(store, nil, DefaultSettings(), nil, ModelContract{}, nil, false, false)
	if err := engine.Settle(context.Background(), domain.MatchState{
		MatchID: "m1", Status: domain.MatchStatusCancelled, Score: domain.ScoreState{Home: 1, Away: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if store.decisions["one"].Status != StatusVoid {
		t.Fatalf("cancelled match settled as %s", store.decisions["one"].Status)
	}
}

func TestLockedCollectionRunsWithoutArmingAndUsesSealedGates(t *testing.T) {
	now := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	model := ModelContract{
		ModelVersion: "model-v1", ModelSHA256: "sha-v1",
		FeatureVersion: "features-v1", OneGoalQualified: true,
	}
	strategy := testStrategy()
	strategy.Definition.Enabled = true
	locked := LockedTest{
		ID: "lock-1", StrategyID: strategy.Definition.ID,
		StrategyVersion: strategy.Definition.Version, State: LockedStateCollecting,
		Contract: LockedTestContract{
			Definition: strategy.Definition, Model: model, Bookmaker: "Bet365",
			MinimumDataQuality: .85, MaximumFeedLag: 15 * time.Second,
			MaximumQuoteAge: 30 * time.Second, PostGoalCooldown: time.Minute,
			TargetOccurrences: LockedCohortTarget,
		},
		ValidationReport: strategy.Report, StartedAt: now.Add(-time.Hour),
	}
	store := &memoryStore{lockedTests: []LockedTest{locked}}
	settings := DefaultSettings()
	settings.MinimumDataQuality = .99 // Must not alter the already sealed test.
	engine := NewEngine(store, nil, settings, map[int]bool{1: true}, model,
		func(domain.MatchState, int) float64 { return .50 }, true, true)
	engine.now = func() time.Time { return now }

	match, state, prediction, event := testSignalInput(now)
	prediction.ModelVersion = model.ModelVersion
	prediction.FeatureVersion = model.FeatureVersion
	if err := engine.Evaluate(context.Background(), match, state, prediction, odds.Event{}); err != nil {
		t.Fatal(err)
	}
	ruleOnly := store.lockedOccurrences["lock-1:m1"]
	if !ruleOnly.RuleCohort || ruleOnly.SignalCohort || ruleOnly.SignalEligible {
		t.Fatalf("missing odds should collect rule only: %+v", ruleOnly)
	}

	match.ID, state.MatchID = "m2", "m2"
	if err := engine.Evaluate(context.Background(), match, state, prediction, event); err != nil {
		t.Fatal(err)
	}
	fullSignal := store.lockedOccurrences["lock-1:m2"]
	if !fullSignal.RuleCohort || !fullSignal.SignalCohort || !fullSignal.SignalEligible {
		t.Fatalf("sealed data-quality threshold was not used: failures=%v", fullSignal.GateFailures)
	}
	if fullSignal.BaselineProbability != .50 {
		t.Fatalf("baseline was not frozen into occurrence: %+v", fullSignal)
	}

	state.Score.Home++
	if err := engine.Settle(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if store.lockedOccurrences["lock-1:m2"].Status != LockedOccurrenceWon {
		t.Fatal("locked occurrence did not settle immediately on target goal")
	}
}
