package predictor

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/enzotriches/golo/internal/domain"
)

func TestPredictorHashesExactArtifactBytes(t *testing.T) {
	raw := []byte(`{
		"modelVersion":"hazard-test","featureVersion":"features-test",
		"modelType":"poisson_hazard","baseGoalsPer90":2.5,
		"coefficients":{},"activityCoefficients":{},"activityCenters":{},
		"sha256":"embedded-value"
	}`)
	path := filepath.Join(t.TempDir(), "model.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	predictor, err := NewPredictor(path)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%x", sha256.Sum256(raw))
	if predictor.SHA256() != want {
		t.Fatalf("artifact hash=%s want exact-file hash=%s", predictor.SHA256(), want)
	}
}

func TestPredictor_Predict(t *testing.T) {
	artifact := MultiHorizonArtifact{
		ModelVersion:   "test_v1",
		FeatureVersion: "v1.0.0",
		Horizons: map[string]HorizonModel{
			"5m": {
				HorizonSeconds: 300,
				Intercept:      -2.0,
				Coefficients:   map[string]float64{"shots_5m_total": 0.5},
				Calibration:    domain.CalibrationParams{Type: "platt", A: -1.0, B: 0.0},
			},
			"10m": {
				HorizonSeconds: 600,
				Intercept:      -1.5,
				Coefficients:   map[string]float64{"shots_10m_total": 0.4},
				Calibration:    domain.CalibrationParams{Type: "platt", A: -1.0, B: 0.0},
			},
			"full_time": {
				HorizonSeconds: 5400,
				Intercept:      -0.5,
				Coefficients:   map[string]float64{"score_diff": -0.2},
				Calibration:    domain.CalibrationParams{Type: "platt", A: -1.0, B: 0.0},
			},
		},
	}

	predEngine := NewPredictorFromArtifact(artifact)

	state := domain.MatchState{
		MatchID:      "m1",
		ClockSeconds: 1200,
		Status:       domain.MatchStatusLive,
	}

	feats := map[string]float64{
		"shots_5m_total":  2,
		"shots_10m_total": 3,
		"score_diff":      0,
	}

	pred, err := predEngine.Predict(state, feats, 0.95)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pred.Probabilities.GoalNext5m <= 0 || pred.Probabilities.GoalNext10m <= 0 {
		t.Errorf("expected positive probabilities, got %+v", pred.Probabilities)
	}
	if pred.Probabilities.GoalNext5m > pred.Probabilities.GoalNext10m {
		t.Errorf("5m prob should not exceed 10m prob: 5m=%f 10m=%f", pred.Probabilities.GoalNext5m, pred.Probabilities.GoalNext10m)
	}
	if pred.ConfidenceBand != domain.ConfidenceHigh {
		t.Errorf("expected HIGH confidence, got %s", pred.ConfidenceBand)
	}
}

// Nested horizons must satisfy p5m <= p10m <= pFT, and the constraint must be
// applied by capping the shorter horizons rather than inflating the full-time
// one — pFT is the North-Star metric and the only time-aware horizon.
func TestPredictHorizonsAreMonotonic(t *testing.T) {
	pred := NewPredictorFromArtifact(MultiHorizonArtifact{
		ModelVersion: "test",
		Horizons: map[string]HorizonModel{
			// A 10m model that would otherwise far exceed the full-time one.
			"5m":        {Intercept: 0.0, Calibration: domain.CalibrationParams{Type: "platt", A: -1}},
			"10m":       {Intercept: 2.0, Calibration: domain.CalibrationParams{Type: "platt", A: -1}},
			"full_time": {Intercept: -4.0, Calibration: domain.CalibrationParams{Type: "platt", A: -1}},
		},
	})

	state := domain.MatchState{MatchID: "m1", ClockSeconds: 5200, Status: domain.MatchStatusLive}
	out, err := pred.Predict(state, map[string]float64{}, 1.0)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}

	p := out.Probabilities
	if !(p.GoalNext5m <= p.GoalNext10m && p.GoalNext10m <= p.GoalBeforeFullTime) {
		t.Fatalf("horizons not monotonic: 5m=%v 10m=%v FT=%v", p.GoalNext5m, p.GoalNext10m, p.GoalBeforeFullTime)
	}

	// The full-time horizon must not have been dragged upward to satisfy it.
	if p.GoalBeforeFullTime > 0.05 {
		t.Fatalf("GoalBeforeFullTime = %v, want the time-aware model's own low value, not one inflated by the shorter horizons", p.GoalBeforeFullTime)
	}
}
