package predictor

import (
	"testing"

	"github.com/enzotriches/golo/internal/domain"
)

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
