package calibration

import (
	"math"

	"github.com/enzotriches/golo/internal/domain"
)

// Calibrator applies probability calibration to raw model output.
type Calibrator struct{}

// NewCalibrator creates a new Calibrator.
func NewCalibrator() *Calibrator {
	return &Calibrator{}
}

// Calibrate maps raw logit or raw probability to a calibrated probability using CalibrationParams.
func (c *Calibrator) Calibrate(rawProb float64, params domain.CalibrationParams) float64 {
	// Clamp raw input probability to avoid log(0)
	rawProb = math.Max(0.0001, math.Min(0.9999, rawProb))

	var calibrated float64

	switch params.Type {
	case "platt":
		// Raw logit f(x)
		logit := math.Log(rawProb / (1.0 - rawProb))
		// Platt scaling formula: 1 / (1 + exp(A * logit + B))
		val := params.A*logit + params.B
		calibrated = 1.0 / (1.0 + math.Exp(val))

	default:
		// Identity or fallback
		calibrated = rawProb
	}

	// Clamp final calibrated probability to [0.001, 0.999]
	return math.Max(0.001, math.Min(0.999, calibrated))
}
