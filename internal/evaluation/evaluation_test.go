package evaluation

import (
	"testing"

	"github.com/enzotriches/golo/internal/domain"
)

func TestHitRate(t *testing.T) {
	samples := []Sample{
		{Predicted: 0.7, Observed: 1}, // correct (predicted yes, happened)
		{Predicted: 0.2, Observed: 0}, // correct (predicted no, didn't happen)
		{Predicted: 0.8, Observed: 0}, // wrong
		{Predicted: 0.1, Observed: 1}, // wrong
	}
	if got := HitRate(samples); got != 50 {
		t.Fatalf("HitRate = %v, want 50", got)
	}
	if got := HitRate(nil); got != 0 {
		t.Fatalf("HitRate(nil) = %v, want 0", got)
	}
}

func TestBuildWindowSamplesOnlyIncludesResolved(t *testing.T) {
	predictions := []domain.Prediction{
		{AsOfMatchSecond: 100, Probabilities: domain.Probabilities{GoalNext10m: 0.6}}, // resolved: end=800 >= 100+600
		{AsOfMatchSecond: 500, Probabilities: domain.Probabilities{GoalNext10m: 0.3}}, // not resolved: end=800 < 500+600
	}
	goalSeconds := []int{650} // a goal at second 650
	endSecond := 800

	samples := BuildWindowSamples(predictions, goalSeconds, endSecond, 600, func(p domain.Probabilities) float64 {
		return p.GoalNext10m
	})

	if len(samples) != 1 {
		t.Fatalf("expected 1 resolved sample, got %d", len(samples))
	}
	if samples[0].Observed != 1 {
		t.Fatalf("expected goal at second 650 to resolve as observed=1 for window (100,700], got %d", samples[0].Observed)
	}
}

func TestComputeMatchTrackRecordNoHistoryIsZeroNotMisleading(t *testing.T) {
	tr := ComputeMatchTrackRecord(nil, nil, 0)
	if tr.ResolvedCount != 0 {
		t.Fatalf("expected ResolvedCount 0 for no predictions, got %d", tr.ResolvedCount)
	}
}

func TestComputeMatchTrackRecordAllCorrect(t *testing.T) {
	predictions := []domain.Prediction{
		{AsOfMatchSecond: 0, Probabilities: domain.Probabilities{GoalNext10m: 0.9}},
	}
	goalSeconds := []int{300} // goal at 5:00, inside (0, 600]
	endSecond := 900

	tr := ComputeMatchTrackRecord(predictions, goalSeconds, endSecond)
	if tr.ResolvedCount != 1 {
		t.Fatalf("expected 1 resolved prediction, got %d", tr.ResolvedCount)
	}
	if tr.AccuracyPct != 100 {
		t.Fatalf("expected 100%% accuracy (predicted yes, goal happened), got %v", tr.AccuracyPct)
	}
}
