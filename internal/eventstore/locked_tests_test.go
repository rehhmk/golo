package eventstore

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/enzotriches/golo/internal/odds"
	"github.com/enzotriches/golo/internal/scenario"
	"github.com/enzotriches/golo/internal/signals"
)

func createCollectingLockedTest(t *testing.T, store *SQLiteStore) signals.LockedTest {
	t.Helper()
	definition := scenario.DefaultDefinition()
	definition.ID = "locked-strategy"
	definition.Name = "Locked strategy"
	definition.Enabled = true
	definition.Conditions = []scenario.Condition{{
		Field: scenario.FieldMinute, Operator: scenario.OpGreaterOrEqual, Value: 70,
	}}
	report := scenario.QualificationReport{
		ValidationQualified: true, ModelValidationQualified: true,
		Validation: scenario.Result{MatchCount: 200, Wins: 180, HitRate: .9, RateLow: .85},
	}
	if err := store.SaveStrategy(signals.StoredStrategy{Definition: definition, Report: report}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	locked := signals.LockedTest{
		ID: "locked-test", StrategyID: definition.ID, StrategyVersion: definition.Version,
		State: signals.LockedStateCollecting,
		Contract: signals.LockedTestContract{
			Definition: definition,
			Model: signals.ModelContract{
				ModelVersion: "model-v1", ModelSHA256: "model-sha",
				FeatureVersion: "features-v1", OneGoalQualified: true,
			},
			MinimumDataQuality: .85, MaximumFeedLag: 15 * time.Second,
			MaximumQuoteAge: 30 * time.Second, PostGoalCooldown: 60 * time.Second,
			TargetOccurrences: signals.LockedCohortTarget,
		},
		ContractSHA256: "contract-sha", ValidationReport: report,
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateLockedTest(locked); err != nil {
		t.Fatal(err)
	}
	return locked
}

func lockedOccurrence(index int) signals.LockedOccurrence {
	return signals.LockedOccurrence{
		ID: fmt.Sprintf("occ-%03d", index), MatchID: fmt.Sprintf("match-%03d", index),
		SignalEligible: true, Status: signals.LockedOccurrenceOpen,
		TriggerSecond: 4200, AdditionalGoals: 1,
		ModelProbability: .95, BaselineProbability: .50,
		MarketProbability: .55, Quote: odds.Quote{Over: 1.80},
		CreatedAt: time.Date(2026, 7, 26, 13, index%60, 0, 0, time.UTC),
	}
}

func TestLockedTestRedactionCohortLimitsVoidReplacementAndReveal(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "locked.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	locked := createCollectingLockedTest(t, store)

	for i := 0; i < signals.LockedCohortTarget; i++ {
		occurrence := lockedOccurrence(i)
		occurrence.TestID = locked.ID
		admitted, err := store.AdmitLockedOccurrence(occurrence)
		if err != nil || !admitted {
			t.Fatalf("admit %d: admitted=%v err=%v", i, admitted, err)
		}
	}
	duplicate := lockedOccurrence(0)
	duplicate.ID = "different-id"
	duplicate.TestID = locked.ID
	if admitted, err := store.AdmitLockedOccurrence(duplicate); err != nil || admitted {
		t.Fatalf("duplicate trigger admitted=%v err=%v", admitted, err)
	}
	overflow := lockedOccurrence(150)
	overflow.TestID = locked.ID
	if admitted, err := store.AdmitLockedOccurrence(overflow); err != nil || admitted {
		t.Fatalf("full cohorts admitted overflow=%v err=%v", admitted, err)
	}

	resolvedAt := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	if err := store.UpdateLockedOccurrenceStatus("occ-000", signals.LockedOccurrenceVoid, resolvedAt); err != nil {
		t.Fatal(err)
	}
	replacement := lockedOccurrence(150)
	replacement.TestID = locked.ID
	if admitted, err := store.AdmitLockedOccurrence(replacement); err != nil || !admitted {
		t.Fatalf("void did not free cohort slot: admitted=%v err=%v", admitted, err)
	}
	for i := 1; i <= signals.LockedCohortTarget; i++ {
		if err := store.UpdateLockedOccurrenceStatus(
			fmt.Sprintf("occ-%03d", i), signals.LockedOccurrenceWon, resolvedAt,
		); err != nil {
			t.Fatalf("resolve %d: %v", i, err)
		}
	}

	view, err := store.GetLockedTestView(locked.StrategyID, locked.StrategyVersion)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != signals.LockedStateReady || view.Report != nil {
		t.Fatalf("ready view leaked report or state wrong: %+v", view)
	}
	if view.Progress.RuleResolved != 150 || view.Progress.SignalResolved != 150 || view.Progress.Voids != 1 {
		t.Fatalf("unexpected redacted progress: %+v", view.Progress)
	}

	revealed, err := store.RevealLockedTest(locked.StrategyID, locked.StrategyVersion, resolvedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if revealed.State != signals.LockedStateRevealedPass || revealed.Report == nil || !revealed.Report.Qualified {
		t.Fatalf("expected revealed pass: %+v", revealed)
	}
	if revealed.Progress.Voids != 1 {
		t.Fatalf("reveal lost void count: %+v", revealed.Progress)
	}
	firstReveal := revealed.Report.RevealedAt
	repeated, err := store.RevealLockedTest(locked.StrategyID, locked.StrategyVersion, resolvedAt.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Report == nil || !repeated.Report.RevealedAt.Equal(firstReveal) {
		t.Fatal("irreversible reveal report changed on retry")
	}
	if err := store.SetStrategyArmed(locked.StrategyID, locked.StrategyVersion, true, "wrong-model"); err == nil {
		t.Fatal("model hash mismatch did not block arming")
	}
	if err := store.SetStrategyArmed(locked.StrategyID, locked.StrategyVersion, true, "model-sha"); err != nil {
		t.Fatalf("revealed passing strategy did not arm: %v", err)
	}
}

func TestLockedTestCannotRevealBeforeReady(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "not-ready.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	locked := createCollectingLockedTest(t, store)
	if _, err := store.RevealLockedTest(locked.StrategyID, locked.StrategyVersion, time.Now()); err == nil {
		t.Fatal("collecting test was revealed")
	}
}
