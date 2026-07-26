package predictor

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/enzotriches/golo/internal/calibration"
	"github.com/enzotriches/golo/internal/domain"
)

type HorizonModel struct {
	HorizonSeconds int                      `json:"horizonSeconds"`
	Intercept      float64                  `json:"intercept"`
	Coefficients   map[string]float64       `json:"coefficients"`
	Calibration    domain.CalibrationParams `json:"calibration"`
}

type MultiHorizonArtifact struct {
	ModelVersion   string                  `json:"modelVersion"`
	FeatureVersion string                  `json:"featureVersion"`
	TrainedUntil   string                  `json:"trainedUntil"`
	SHA256         string                  `json:"sha256"`
	Horizons       map[string]HorizonModel `json:"horizons"`
}

type Predictor struct {
	artifact   MultiHorizonArtifact
	calibrator *calibration.Calibrator
	sequence   int
}

func NewPredictor(artifactPath string) (*Predictor, error) {
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read model artifact: %w", err)
	}

	var artifact MultiHorizonArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, fmt.Errorf("failed to parse model artifact JSON: %w", err)
	}

	return &Predictor{
		artifact:   artifact,
		calibrator: calibration.NewCalibrator(),
		sequence:   0,
	}, nil
}

func NewPredictorFromArtifact(artifact MultiHorizonArtifact) *Predictor {
	return &Predictor{
		artifact:   artifact,
		calibrator: calibration.NewCalibrator(),
		sequence:   0,
	}
}

func (p *Predictor) Predict(state domain.MatchState, feats map[string]float64, qualityScore float64) (domain.Prediction, error) {
	p.sequence++

	if state.Status == domain.MatchStatusFinished || state.ClockSeconds >= 5400 {
		return domain.Prediction{
			MatchID:         state.MatchID,
			AsOfMatchSecond: state.ClockSeconds,
			CalculatedAt:    time.Now(),
			Probabilities: domain.Probabilities{
				GoalNext5m:         0.001,
				GoalNext10m:        0.001,
				GoalBeforeFullTime: 0.001,
			},
			DataQuality:        qualityScore,
			ConfidenceBand:     domain.ConfidenceHigh,
			Status:             domain.PredictionStatusOK,
			ModelVersion:       p.artifact.ModelVersion,
			CalibratorVersion:  "platt_v1",
			FeatureVersion:     p.artifact.FeatureVersion,
			PredictionSequence: p.sequence,
		}, nil
	}

	p5m := p.evaluateHorizon("5m", feats)
	p10m := p.evaluateHorizon("10m", feats)
	pFT := p.evaluateHorizon("full_time", feats)

	// Enforce monotonicity: p5m <= p10m <= pFT.
	//
	// "A goal before full time" contains "a goal in the next 10 minutes",
	// which contains "a goal in the next 5 minutes", so the inequality is a
	// logical necessity rather than a modelling preference. It is enforced by
	// capping the shorter horizons against the longer one, never by raising
	// the longer one to meet them: only the full-time model is time-aware
	// (it carries a match_second term), so near the final whistle it is the
	// only horizon that knows there are three minutes left rather than ten.
	// Raising pFT to meet an unconstrained p10m did the opposite — at the
	// 87th minute of a real match it inflated the published full-time
	// probability from 1.6% to 34.8%, and that figure is the North-Star
	// horizon every calibration metric is scored against.
	p10m = math.Min(p10m, pFT)
	p5m = math.Min(p5m, p10m)

	// Confidence band derived from qualityScore
	confBand := domain.ConfidenceHigh
	if qualityScore < 0.5 {
		confBand = domain.ConfidenceLow
	} else if qualityScore < 0.8 {
		confBand = domain.ConfidenceMedium
	}

	predStatus := domain.PredictionStatusOK
	if qualityScore < 0.4 {
		predStatus = domain.PredictionStatusDegraded
	}

	return domain.Prediction{
		MatchID:         state.MatchID,
		AsOfMatchSecond: state.ClockSeconds,
		CalculatedAt:    time.Now(),
		Probabilities: domain.Probabilities{
			GoalNext5m:         roundProb(p5m),
			GoalNext10m:        roundProb(p10m),
			GoalBeforeFullTime: roundProb(pFT),
		},
		DataQuality:        qualityScore,
		ConfidenceBand:     confBand,
		Status:             predStatus,
		ModelVersion:       p.artifact.ModelVersion,
		CalibratorVersion:  "platt_v1",
		FeatureVersion:     p.artifact.FeatureVersion,
		PredictionSequence: p.sequence,
	}, nil
}

func (p *Predictor) evaluateHorizon(horizonKey string, feats map[string]float64) float64 {
	hModel, exists := p.artifact.Horizons[horizonKey]
	if !exists {
		return 0.10
	}

	logit := hModel.Intercept
	for name, coef := range hModel.Coefficients {
		if val, ok := feats[name]; ok {
			logit += coef * val
		}
	}

	// Sigmoid function
	rawProb := 1.0 / (1.0 + math.Exp(-logit))

	// Apply Platt calibration
	return p.calibrator.Calibrate(rawProb, hModel.Calibration)
}

func roundProb(p float64) float64 {
	return math.Round(p*1000) / 1000
}
