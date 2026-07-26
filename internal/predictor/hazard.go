package predictor

import (
	"math"
	"sort"

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
// The intensity is the long-run competition goal rate scaled by live match
// state and activity. The artifact records whether activity coefficients were
// fitted from timestamped timelines or retained as centered priors.
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
//
// Coefficients and RedCardCoef are fitted from historical matches. Activity
// terms are fitted only when the dataset includes enough timestamped events;
// otherwise the artifact explicitly retains centered priors. They are applied
// relative to ActivityCenters so ordinary activity leaves intensity unchanged.
type HazardArtifact struct {
	ModelVersion              string             `json:"modelVersion"`
	FeatureVersion            string             `json:"featureVersion"`
	ModelType                 string             `json:"modelType"`
	BaseGoalsPer90            float64            `json:"baseGoalsPer90"`
	CompetitionBaseGoalsPer90 map[string]float64 `json:"competitionBaseGoalsPer90,omitempty"`
	Coefficients              map[string]float64 `json:"coefficients"`
	RedCardCoef               float64            `json:"redCardCoefficient"`

	ActivityCoefficients map[string]float64 `json:"activityCoefficients"`
	ActivityCenters      map[string]float64 `json:"activityCenters"`

	MinMultiplier float64          `json:"minMultiplier"`
	MaxMultiplier float64          `json:"maxMultiplier"`
	TrainedUntil  string           `json:"trainedUntil"`
	TrainingCount int              `json:"trainingMatches"`
	Notes         string           `json:"notes"`
	Validation    HazardValidation `json:"validation"`
	SHA256        string           `json:"sha256"`
}

type HazardValidation struct {
	ValidationMatches    int     `json:"validationMatches"`
	HoldoutMatches       int     `json:"holdoutMatches"`
	BaselineGoalsPer90   float64 `json:"baselineGoalsPer90"`
	OneGoalBrier         float64 `json:"oneGoalBrier"`
	OneGoalBaselineBrier float64 `json:"oneGoalBaselineBrier"`
	TwoGoalBrier         float64 `json:"twoGoalBrier"`
	TwoGoalBaselineBrier float64 `json:"twoGoalBaselineBrier"`
	OneGoalQualified     bool    `json:"oneGoalQualified"`
	TwoGoalQualified     bool    `json:"twoGoalQualified"`
}

func (v HazardValidation) matchCount() int {
	if v.ValidationMatches > 0 {
		return v.ValidationMatches
	}
	return v.HoldoutMatches
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

// timeCoefficients are the fitted terms that depend on the match clock rather
// than on the current state, and so keep changing across a prediction window.
const (
	featTimeFrac   = "match_time_frac"
	featTimeFracSq = "match_time_frac_sq"
)

// stateExponent is the part of the log-intensity that does not move with the
// clock: scoreline, cards, and observed activity. These are held at their
// present values across the window, since there is no way to know what the
// next ten minutes will bring.
func (h HazardArtifact) stateExponent(feats map[string]float64) float64 {
	exponent, _ := h.stateExplanation(feats)
	return exponent
}

// stateExplanation returns the exact additive decomposition used by
// stateExponent. It is deterministic and contains no generated prose.
func (h HazardArtifact) stateExplanation(feats map[string]float64) (float64, []domain.FeatureContribution) {
	exponent := 0.0
	contributions := make([]domain.FeatureContribution, 0, len(h.Coefficients)+len(h.ActivityCoefficients)+1)
	for name, coef := range h.Coefficients {
		if name == featTimeFrac || name == featTimeFracSq {
			continue
		}
		value := feats[name]
		contribution := coef * value
		exponent += contribution
		contributions = append(contributions, domain.FeatureContribution{
			Name: name, Value: value, Coefficient: coef, Contribution: contribution,
		})
	}

	// A dismissal opens a match up regardless of which side took it, so the
	// magnitude of the imbalance is what matters, not its sign.
	redValue := math.Abs(feats["red_cards_diff"])
	redContribution := h.RedCardCoef * redValue
	exponent += redContribution
	contributions = append(contributions, domain.FeatureContribution{
		Name: "abs_red_diff", Value: redValue, Coefficient: h.RedCardCoef, Contribution: redContribution,
	})

	// Live activity, measured against its typical level rather than absolutely,
	// and faded in as observation accumulates. Rolling windows start empty
	// whenever Golo joins a match, so without the coverage weight every match
	// would open with a large negative activity term for no reason other than
	// having just arrived.
	if coverage := feats["activity_coverage"]; coverage > 0 {
		for name, coef := range h.ActivityCoefficients {
			value := coverage * (feats[name] - h.ActivityCenters[name])
			contribution := coef * value
			exponent += contribution
			contributions = append(contributions, domain.FeatureContribution{
				Name: name, Value: value, Coefficient: coef, Contribution: contribution,
			})
		}
	}

	sort.Slice(contributions, func(i, j int) bool {
		return math.Abs(contributions[i].Contribution) > math.Abs(contributions[j].Contribution)
	})
	return exponent, contributions
}

// timeExponent is the clock-dependent part of the log-intensity at a given
// match second.
func (h HazardArtifact) timeExponent(second float64) float64 {
	frac := second / regulationSeconds
	return h.Coefficients[featTimeFrac]*frac + h.Coefficients[featTimeFracSq]*frac*frac
}

// intensityAt is the per-second goal arrival rate for both teams combined at a
// given match second.
func (h HazardArtifact) intensityAt(stateExp, second float64) float64 {
	return h.intensityAtBase(stateExp, second, h.BaseGoalsPer90)
}

func (h HazardArtifact) intensityAtBase(stateExp, second, baseGoalsPer90 float64) float64 {
	multiplier := math.Exp(stateExp + h.timeExponent(second))
	if h.MinMultiplier > 0 && multiplier < h.MinMultiplier {
		multiplier = h.MinMultiplier
	}
	if h.MaxMultiplier > 0 && multiplier > h.MaxMultiplier {
		multiplier = h.MaxMultiplier
	}
	return (baseGoalsPer90 / regulationSeconds) * multiplier
}

// integrationStepSeconds is the granularity of the intensity integral. The
// intensity curve is smooth over a 90-minute match, so half-minute steps are
// far finer than the shape requires.
const integrationStepSeconds = 30.0

// expectedGoals integrates the intensity across a window, which is what makes
// the horizons correct rather than merely ordered.
//
// The measured goal rate is not flat: it climbs from 1.93 goals/90 in the
// opening ten minutes to roughly 2.8 by the hour mark. Holding the current
// instant's intensity fixed and extrapolating it to the final whistle
// therefore understates every long horizon early in a match — at kickoff it
// put the chance of any goal at 0.865 against a real base rate near 0.93,
// because it priced ninety minutes at the cautious rate of the first ten.
func (h HazardArtifact) expectedGoals(feats map[string]float64, fromSecond, windowSeconds float64) float64 {
	return h.expectedGoalsForCompetition(feats, "", fromSecond, windowSeconds)
}

func (h HazardArtifact) expectedGoalsForCompetition(feats map[string]float64, competitionID string, fromSecond, windowSeconds float64) float64 {
	if windowSeconds <= 0 {
		return 0
	}

	stateExp := h.stateExponent(feats)
	base := h.BaseGoalsPer90
	if competitionBase := h.CompetitionBaseGoalsPer90[competitionID]; competitionBase > 0 {
		base = competitionBase
	}

	total := 0.0
	for elapsed := 0.0; elapsed < windowSeconds; elapsed += integrationStepSeconds {
		step := math.Min(integrationStepSeconds, windowSeconds-elapsed)
		// Midpoint rule: sample the intensity in the middle of each step.
		total += h.intensityAtBase(stateExp, fromSecond+elapsed+step/2, base) * step
	}
	return total
}

// goalProbability is the chance of at least one goal within the next
// windowSeconds, never looking past the end of the match.
func (h HazardArtifact) goalProbability(feats map[string]float64, fromSecond, windowSeconds, remaining float64) float64 {
	return h.goalProbabilityForCompetition(feats, "", fromSecond, windowSeconds, remaining)
}

func (h HazardArtifact) goalProbabilityForCompetition(feats map[string]float64, competitionID string, fromSecond, windowSeconds, remaining float64) float64 {
	effective := math.Min(windowSeconds, remaining)
	if effective <= 0 {
		return 0
	}
	return 1 - math.Exp(-h.expectedGoalsForCompetition(feats, competitionID, fromSecond, effective))
}

// atLeastGoalsProbability returns P(N >= goals) for a Poisson count.
func atLeastGoalsProbability(expected float64, goals int) float64 {
	if goals <= 0 {
		return 1
	}
	if expected <= 0 {
		return 0
	}
	term := math.Exp(-expected)
	cdf := term
	for k := 1; k < goals; k++ {
		term *= expected / float64(k)
		cdf += term
	}
	return math.Max(0, math.Min(1, 1-cdf))
}
