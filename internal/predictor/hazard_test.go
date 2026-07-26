package predictor

import (
	"testing"

	"github.com/enzotriches/golo/internal/domain"
)

func hazardPredictor(t *testing.T) *Predictor {
	t.Helper()
	p, err := NewPredictor("../../models/hazard_v1.json")
	if err != nil {
		t.Fatalf("loading hazard artifact: %v", err)
	}
	if p.hazard == nil {
		t.Fatal("artifact did not load as a hazard model")
	}
	return p
}

func predictAt(t *testing.T, p *Predictor, clock int, feats map[string]float64) domain.Probabilities {
	t.Helper()
	out, err := p.Predict(domain.MatchState{
		MatchID:      "m1",
		ClockSeconds: clock,
		Status:       domain.MatchStatusLive,
	}, feats, 1.0)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	return out.Probabilities
}

// The whole reason for the hazard form: horizons are ordered by construction.
func TestHazardHorizonsAreStrictlyOrderedWithTimeRemaining(t *testing.T) {
	p := hazardPredictor(t)

	got := predictAt(t, p, 4200, map[string]float64{})
	if !(got.GoalNext5m < got.GoalNext10m && got.GoalNext10m < got.GoalBeforeFullTime) {
		t.Fatalf("expected 5m < 10m < FT with 20+ minutes left, got %+v", got)
	}
}

// With less than five minutes left, all three horizons describe the same
// remaining time and must agree — this is the behaviour the previous model
// got wrong in the opposite direction, reporting one flat number all match.
func TestHazardHorizonsConvergeNearFullTime(t *testing.T) {
	p := hazardPredictor(t)

	got := predictAt(t, p, 5620, map[string]float64{})
	if got.GoalNext5m != got.GoalNext10m || got.GoalNext10m != got.GoalBeforeFullTime {
		t.Fatalf("expected all horizons equal deep in stoppage time, got %+v", got)
	}
}

// A quiet match at kickoff should sit near the real-world base rate: roughly
// nine matches in ten contain at least one goal.
func TestHazardFullTimeAtKickoffMatchesBaseRate(t *testing.T) {
	p := hazardPredictor(t)

	got := predictAt(t, p, 0, map[string]float64{})
	if got.GoalBeforeFullTime < 0.88 || got.GoalBeforeFullTime > 0.97 {
		t.Fatalf("GoalBeforeFullTime at kickoff = %v, want roughly the 0.88-0.97 base rate", got.GoalBeforeFullTime)
	}
}

// The full-time probability must decay as the match runs out, holding
// activity constant.
func TestHazardFullTimeDecaysWithTime(t *testing.T) {
	p := hazardPredictor(t)

	early := predictAt(t, p, 600, map[string]float64{})
	late := predictAt(t, p, 4800, map[string]float64{})
	if !(late.GoalBeforeFullTime < early.GoalBeforeFullTime) {
		t.Fatalf("FT probability did not decay: 10' = %v, 80' = %v",
			early.GoalBeforeFullTime, late.GoalBeforeFullTime)
	}
}

// Attacking pressure must raise the short horizons, or the live statistics
// feed is decorative.
func TestHazardRespondsToAttackingPressure(t *testing.T) {
	p := hazardPredictor(t)

	quiet := predictAt(t, p, 4200, map[string]float64{"activity_coverage": 1})
	busy := predictAt(t, p, 4200, map[string]float64{
		"activity_coverage":           1,
		"shots_10m_total":             4,
		"shots_on_target_10m_total":   2,
		"corners_10m_total":           2,
		"dangerous_attacks_10m_total": 20,
	})

	if busy.GoalNext10m <= quiet.GoalNext10m {
		t.Fatalf("pressure did not raise the 10m horizon: quiet %v, busy %v",
			quiet.GoalNext10m, busy.GoalNext10m)
	}
}

// A red card opens the match up regardless of which side received it.
func TestHazardRedCardRaisesIntensityEitherWay(t *testing.T) {
	p := hazardPredictor(t)

	level := predictAt(t, p, 4200, map[string]float64{})
	homeSentOff := predictAt(t, p, 4200, map[string]float64{"red_cards_diff": 1})
	awaySentOff := predictAt(t, p, 4200, map[string]float64{"red_cards_diff": -1})

	if homeSentOff.GoalNext10m <= level.GoalNext10m {
		t.Fatalf("a red card did not raise intensity: %v vs %v", homeSentOff.GoalNext10m, level.GoalNext10m)
	}
	if homeSentOff.GoalNext10m != awaySentOff.GoalNext10m {
		t.Fatalf("red card direction changed the result: %v vs %v",
			homeSentOff.GoalNext10m, awaySentOff.GoalNext10m)
	}
}

// Stoppage time is still football. The old model treated the 90th minute as
// the end and reported a flat 0.001 for everything past it.
func TestHazardStillPredictsInStoppageTime(t *testing.T) {
	p := hazardPredictor(t)

	got := predictAt(t, p, 5500, map[string]float64{})
	if got.GoalBeforeFullTime <= 0.001 {
		t.Fatalf("GoalBeforeFullTime in stoppage time = %v, want a real probability", got.GoalBeforeFullTime)
	}
}

func TestFinishedMatchHasNoRemainingChance(t *testing.T) {
	p := hazardPredictor(t)

	out, err := p.Predict(domain.MatchState{
		MatchID:      "m1",
		ClockSeconds: 5700,
		Status:       domain.MatchStatusFinished,
	}, map[string]float64{}, 1.0)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if out.Probabilities.GoalBeforeFullTime > 0.01 {
		t.Fatalf("finished match reported %v", out.Probabilities.GoalBeforeFullTime)
	}
}

// The multiplier is bounded so an extreme statistics burst cannot drive the
// intensity to an absurd value.
func TestHazardMultiplierIsBounded(t *testing.T) {
	p := hazardPredictor(t)

	got := predictAt(t, p, 4200, map[string]float64{
		"activity_coverage":           1,
		"shots_10m_total":             500,
		"shots_on_target_10m_total":   500,
		"dangerous_attacks_10m_total": 5000,
	})
	if got.GoalNext5m >= 1.0 {
		t.Fatalf("unbounded intensity produced P5m = %v", got.GoalNext5m)
	}

	stateExp := p.hazard.stateExponent(map[string]float64{"activity_coverage": 1, "shots_10m_total": 1e6})
	lambda := p.hazard.intensityAt(stateExp, 4200)
	maxLambda := p.hazard.BaseGoalsPer90 / regulationSeconds * p.hazard.MaxMultiplier
	if lambda > maxLambda*1.000001 {
		t.Fatalf("intensity %v exceeded the capped maximum %v", lambda, maxLambda)
	}
}

// Empty rolling windows mean "not observed yet", not "nothing is happening".
// Without the coverage gate the centered activity terms read the opening
// minutes of every match — and every match joined in progress — as unusually
// dull, which knocked the kickoff full-time probability from 0.92 to 0.77.
func TestActivityIsIgnoredUntilObserved(t *testing.T) {
	p := hazardPredictor(t)

	unobserved := predictAt(t, p, 0, map[string]float64{"activity_coverage": 0})
	observedQuiet := predictAt(t, p, 0, map[string]float64{"activity_coverage": 1})

	if !(unobserved.GoalBeforeFullTime > observedQuiet.GoalBeforeFullTime) {
		t.Fatalf("a genuinely quiet observed match should rate below an unobserved one: %v vs %v",
			observedQuiet.GoalBeforeFullTime, unobserved.GoalBeforeFullTime)
	}
	if unobserved.GoalBeforeFullTime < 0.88 {
		t.Fatalf("kickoff with no observation yet = %v, want the unpenalised base rate", unobserved.GoalBeforeFullTime)
	}
}

// The intensity rises through a match, so a long horizon must integrate it
// rather than extrapolate the current instant flat to the whistle.
func TestExpectedGoalsIntegratesRisingIntensity(t *testing.T) {
	p := hazardPredictor(t)
	feats := map[string]float64{"activity_coverage": 0}

	// A full match's worth of expected goals should land near the observed
	// average of roughly 2.7 per match.
	total := p.hazard.expectedGoals(feats, 0, 5640)
	if total < 2.3 || total > 3.1 {
		t.Fatalf("expected goals over a whole match = %.3f, want roughly the observed 2.7", total)
	}

	// Held flat at the kickoff intensity the same window would fall short,
	// which is precisely the bug this replaced.
	flat := p.hazard.intensityAt(p.hazard.stateExponent(feats), 0) * 5640
	if flat >= total {
		t.Fatalf("flat extrapolation %.3f should undershoot the integral %.3f", flat, total)
	}
}

// Guards the artifact against being replaced by one that was never fitted.
func TestArtifactIsTrained(t *testing.T) {
	p := hazardPredictor(t)

	if p.hazard.TrainingCount < 500 {
		t.Fatalf("artifact reports %d training matches, want a real fitted model", p.hazard.TrainingCount)
	}
	for _, name := range []string{"match_time_frac", "match_time_frac_sq", "abs_score_diff"} {
		if p.hazard.Coefficients[name] == 0 {
			t.Errorf("coefficient %s is exactly zero — the fit degenerated to intercept-only", name)
		}
	}
	// The opening of a match is measurably quieter than the hour mark; the
	// fitted time terms must reproduce that ordering.
	stateExp := p.hazard.stateExponent(map[string]float64{})
	if p.hazard.intensityAt(stateExp, 0) >= p.hazard.intensityAt(stateExp, 3600) {
		t.Error("fitted intensity does not rise from kickoff to the hour mark")
	}
}
