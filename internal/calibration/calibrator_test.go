package calibration

import (
	"testing"

	"github.com/enzotriches/golo/internal/domain"
)

func TestCalibrator_Calibrate(t *testing.T) {
	cal := NewCalibrator()
	params := domain.CalibrationParams{
		Type: "platt",
		A:    -1.0,
		B:    0.0,
	}

	raw := 0.60
	calibrated := cal.Calibrate(raw, params)

	if calibrated <= 0.0 || calibrated >= 1.0 {
		t.Errorf("calibrated probability out of bounds: %f", calibrated)
	}
	if calibrated != raw {
		// With A=-1 and B=0, 1/(1+exp(-logit)) should equal raw
		if mathAbs(calibrated-raw) > 1e-4 {
			t.Errorf("expected approx %f, got %f", raw, calibrated)
		}
	}
}

func mathAbs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
