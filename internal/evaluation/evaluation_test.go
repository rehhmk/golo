package evaluation

import (
	"math"
	"testing"

	"github.com/enzotriches/golo/internal/domain"
)

func predAt(second int, ft, ten float64) domain.Prediction {
	return domain.Prediction{
		MatchID:         "m1",
		AsOfMatchSecond: second,
		Probabilities:   domain.Probabilities{GoalBeforeFullTime: ft, GoalNext10m: ten},
	}
}

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

// The central fix: "will there be a goal before the final whistle?" has no
// answer while the match is still being played. Counting a goalless match in
// progress as a settled no-goal is what made the published metrics wrong.
func TestFullTimeSamplesExcludeUnfinishedMatches(t *testing.T) {
	predictions := []domain.Prediction{predAt(600, 0.9, 0.3)}

	running := map[string]MatchOutcome{
		"m1": {FinalSecond: 3000, Finished: false},
	}
	if got := BuildFullTimeSamples(predictions, running); len(got) != 0 {
		t.Fatalf("got %d samples from a match still in progress, want 0", len(got))
	}

	finished := map[string]MatchOutcome{
		"m1": {FinalSecond: 5700, Finished: true},
	}
	got := BuildFullTimeSamples(predictions, finished)
	if len(got) != 1 {
		t.Fatalf("got %d samples from a finished match, want 1", len(got))
	}
	if got[0].Observed != 0 {
		t.Fatalf("Observed = %d, want 0 — the finished match had no goal", got[0].Observed)
	}
}

func TestFullTimeSamplesLabelGoalAfterPrediction(t *testing.T) {
	outcomes := map[string]MatchOutcome{
		"m1": {GoalSeconds: []int{1200}, FinalSecond: 5700, Finished: true},
	}
	samples := BuildFullTimeSamples([]domain.Prediction{
		predAt(600, 0.9, 0.3),  // the goal at 1200 is still ahead
		predAt(1800, 0.4, 0.2), // goal already happened, none after
	}, outcomes)

	if len(samples) != 2 {
		t.Fatalf("got %d samples, want 2", len(samples))
	}
	if samples[0].Observed != 1 {
		t.Errorf("prediction before the goal: Observed = %d, want 1", samples[0].Observed)
	}
	if samples[1].Observed != 0 {
		t.Errorf("prediction after the goal: Observed = %d, want 0", samples[1].Observed)
	}
}

func TestBuildWindowSamplesOnlyIncludesResolved(t *testing.T) {
	outcome := MatchOutcome{GoalSeconds: []int{650}, FinalSecond: 800}
	samples := BuildWindowSamples([]domain.Prediction{
		predAt(100, 0.5, 0.6), // window closes at 700, match reached 800 → resolved
		predAt(500, 0.5, 0.3), // window closes at 1100, match only reached 800 → not yet
	}, outcome, 600, func(p domain.Probabilities) float64 { return p.GoalNext10m })

	if len(samples) != 1 {
		t.Fatalf("expected 1 resolved sample, got %d", len(samples))
	}
	if samples[0].Observed != 1 {
		t.Fatalf("expected the goal at 650 to resolve as observed=1 for (100,700], got %d", samples[0].Observed)
	}
}

// A window the final whistle cuts short is still resolved. Discarding those
// would throw away every prediction made in the closing minutes, which is
// exactly where goals are most likely.
func TestBuildWindowSamplesResolveWhenMatchEndsEarly(t *testing.T) {
	outcome := MatchOutcome{GoalSeconds: []int{5500}, FinalSecond: 5700, Finished: true}
	samples := BuildWindowSamples([]domain.Prediction{
		predAt(5400, 0.5, 0.4), // window would close at 6000, match ended at 5700
	}, outcome, 600, func(p domain.Probabilities) float64 { return p.GoalNext10m })

	if len(samples) != 1 {
		t.Fatalf("got %d samples, want 1", len(samples))
	}
	if samples[0].Observed != 1 {
		t.Errorf("Observed = %d, want 1 — the goal at 5500 came before the whistle", samples[0].Observed)
	}
}

func TestComputeMatchTrackRecordNoHistoryIsZeroNotMisleading(t *testing.T) {
	tr := ComputeMatchTrackRecord(nil, MatchOutcome{})
	if tr.ResolvedCount != 0 {
		t.Fatalf("expected ResolvedCount 0 for no predictions, got %d", tr.ResolvedCount)
	}
}

func TestComputeMatchTrackRecordAllCorrect(t *testing.T) {
	outcome := MatchOutcome{GoalSeconds: []int{300}, FinalSecond: 900}
	tr := ComputeMatchTrackRecord([]domain.Prediction{predAt(0, 0.5, 0.9)}, outcome)
	if tr.ResolvedCount != 1 {
		t.Fatalf("expected 1 resolved prediction, got %d", tr.ResolvedCount)
	}
	if tr.AccuracyPct != 100 {
		t.Fatalf("expected 100%% accuracy (predicted yes, goal happened), got %v", tr.AccuracyPct)
	}
}

func TestEvaluateReportsResolvedAndMatchCounts(t *testing.T) {
	predictions := []domain.Prediction{
		{MatchID: "done", AsOfMatchSecond: 600, Probabilities: domain.Probabilities{GoalBeforeFullTime: 0.8}},
		{MatchID: "done", AsOfMatchSecond: 1200, Probabilities: domain.Probabilities{GoalBeforeFullTime: 0.7}},
		{MatchID: "running", AsOfMatchSecond: 600, Probabilities: domain.Probabilities{GoalBeforeFullTime: 0.9}},
	}
	outcomes := map[string]MatchOutcome{
		"done":    {GoalSeconds: []int{2000}, FinalSecond: 5700, Finished: true},
		"running": {FinalSecond: 3000, Finished: false},
	}

	report := Evaluate(predictions, outcomes)

	if report.TotalSnapshots != 3 {
		t.Errorf("TotalSnapshots = %d, want 3", report.TotalSnapshots)
	}
	if report.ResolvedCount != 2 {
		t.Errorf("ResolvedCount = %d, want 2 — only the finished match resolves", report.ResolvedCount)
	}
	if report.MatchCount != 1 {
		t.Errorf("MatchCount = %d, want 1 — the effective sample size", report.MatchCount)
	}
}

// A dashboard gated on the prediction count can show a confident-looking zero
// for every metric while nothing at all has actually been measured.
func TestEvaluateWithNothingResolvedReportsZeroResolved(t *testing.T) {
	report := Evaluate([]domain.Prediction{
		{MatchID: "m1", AsOfMatchSecond: 600, Probabilities: domain.Probabilities{GoalBeforeFullTime: 0.8}},
	}, map[string]MatchOutcome{
		"m1": {FinalSecond: 3000, Finished: false},
	})

	if report.TotalSnapshots == 0 {
		t.Fatal("TotalSnapshots should still count the prediction")
	}
	if report.ResolvedCount != 0 {
		t.Fatalf("ResolvedCount = %d, want 0", report.ResolvedCount)
	}
	if len(report.CalibrationCurve) != 0 {
		t.Fatalf("calibration curve should be empty, got %d bins", len(report.CalibrationCurve))
	}
}

func TestBrierScoreAndLogLossKnownValues(t *testing.T) {
	samples := []Sample{
		{Predicted: 0.8, Observed: 1},
		{Predicted: 0.4, Observed: 0},
	}
	// ((0.8-1)^2 + (0.4-0)^2) / 2 = (0.04 + 0.16) / 2 = 0.10
	if got := BrierScore(samples); math.Abs(got-0.10) > 1e-9 {
		t.Errorf("BrierScore = %v, want 0.10", got)
	}
	// -(ln 0.8 + ln 0.6) / 2
	want := -(math.Log(0.8) + math.Log(0.6)) / 2
	if got := LogLoss(samples); math.Abs(got-want) > 1e-9 {
		t.Errorf("LogLoss = %v, want %v", got, want)
	}
	if got := BrierScore(nil); got != 0 {
		t.Errorf("BrierScore(nil) = %v, want 0", got)
	}
	if got := LogLoss(nil); got != 0 {
		t.Errorf("LogLoss(nil) = %v, want 0", got)
	}
}

// The clamp exists so a confident-and-wrong prediction costs a large but
// finite penalty rather than infinity.
func TestLogLossClampsCertainty(t *testing.T) {
	got := LogLoss([]Sample{{Predicted: 1.0, Observed: 0}})
	if math.IsInf(got, 0) || math.IsNaN(got) {
		t.Fatalf("LogLoss = %v, want a finite penalty", got)
	}
}

func TestECEIsZeroForPerfectCalibration(t *testing.T) {
	var samples []Sample
	// 100 samples at p=0.9 of which exactly 90 are positive.
	for i := 0; i < 100; i++ {
		observed := 0
		if i < 90 {
			observed = 1
		}
		samples = append(samples, Sample{Predicted: 0.9, Observed: observed})
	}
	ece, curve := ECEAndCurve(samples, 5)
	if ece > 1e-9 {
		t.Errorf("ECE = %v, want ~0 for a perfectly calibrated bin", ece)
	}
	if len(curve) != 1 {
		t.Fatalf("got %d bins, want 1", len(curve))
	}
	if curve[0].Count != 100 {
		t.Errorf("Count = %d, want 100", curve[0].Count)
	}
}

func TestECEDetectsOverconfidence(t *testing.T) {
	var samples []Sample
	for i := 0; i < 100; i++ {
		samples = append(samples, Sample{Predicted: 0.9, Observed: 0})
	}
	ece, _ := ECEAndCurve(samples, 5)
	if math.Abs(ece-0.9) > 1e-9 {
		t.Errorf("ECE = %v, want 0.9 — predicted 0.9, observed 0", ece)
	}
}
