package predictor

import (
	"math"

	"github.com/enzotriches/golo/internal/domain"
)

// The Poisson hazard baseline (blueprint §12, ADR "no deep learning in MVP").
//
// Goals arrive as a roughly Poisson process, so the probability of at least
// one goal over a window of w seconds at intensity lambda is
//
//	P = 1 - exp(-lambda * w)
//
// Two properties matter more than raw accuracy here, and neither survives a
// per-horizon logistic model:
//
//   - Nesting is automatic. "Goal before full time" covers a longer window
//     than "goal in the next 10 minutes", which covers "the next 5", so the
//     probabilities are correctly ordered by construction rather than by a
//     clamp applied afterwards.
//   - Horizons converge as the match runs out. With four minutes left, "the
//     next 10 minutes" and "before full time" describe the same four minutes
//     and must report the same number — which falls out of capping each
//     window at the time actually remaining.
//
// The intensity is the long-run league goal rate scaled by live match
// activity. The scaling coefficients are a prior, not a fit: see the model
// artifact's own notes. Calibration against real outcomes is what
// ml/src/train_baseline.py is for, and until that runs these numbers are
// directionally sensible rather than trustworthy in absolute terms.
const (
	// regulationSeconds is 90 minutes of normal time.
	regulationSeconds = 5400.0

	// stoppageAllowanceSeconds approximates added time across both halves.
	// Without it a match sitting at 90:00+ would report zero remaining time
	// and therefore zero chance of a goal, while still being played.
	stoppageAllowanceSeconds = 240.0

	// minRemainingSeconds keeps a match that is deep into stoppage time from
	// collapsing to exactly zero while the provider still reports it live.
	minRemainingSeconds = 30.0
)

// HazardArtifact is a Poisson-hazard model artifact.
type HazardArtifact struct {
	ModelVersion   string             `json:"modelVersion"`
	FeatureVersion string             `json:"featureVersion"`
	ModelType      string             `json:"modelType"`
	BaseGoalsPer90 float64            `json:"baseGoalsPer90"`
	Coefficients   map[string]float64 `json:"coefficients"`
	RedCardCoef    float64            `json:"redCardCoefficient"`
	MinMultiplier  float64            `json:"minMultiplier"`
	MaxMultiplier  float64            `json:"maxMultiplier"`
	TrainedUntil   string             `json:"trainedUntil"`
	Notes          string             `json:"notes"`
}

// remainingSeconds estimates how much play is left, which is what every
// horizon has to be capped against.
func remainingSeconds(state domain.MatchState) float64 {
	remaining := regulationSeconds + stoppageAllowanceSeconds - float64(state.ClockSeconds)
	if remaining < minRemainingSeconds {
		return minRemainingSeconds
	}
	return remaining
}

// intensity returns the per-second goal arrival rate for both teams combined.
func (h HazardArtifact) intensity(feats map[string]float64) float64 {
	base := h.BaseGoalsPer90 / regulationSeconds

	exponent := 0.0
	for name, coef := range h.Coefficients {
		exponent += coef * feats[name]
	}

	// A dismissal opens a match up regardless of which side took it, so the
	// magnitude of the imbalance is what matters, not its sign.
	exponent += h.RedCardCoef * math.Abs(feats["red_cards_diff"])

	multiplier := math.Exp(exponent)
	if h.MinMultiplier > 0 && multiplier < h.MinMultiplier {
		multiplier = h.MinMultiplier
	}
	if h.MaxMultiplier > 0 && multiplier > h.MaxMultiplier {
		multiplier = h.MaxMultiplier
	}

	return base * multiplier
}

// goalProbability is the chance of at least one goal within the next
// windowSeconds, never looking past the end of the match.
func goalProbability(lambda, windowSeconds, remaining float64) float64 {
	effective := math.Min(windowSeconds, remaining)
	if effective <= 0 {
		return 0
	}
	return 1 - math.Exp(-lambda*effective)
}
