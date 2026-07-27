package features

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

const FeatureSchemaVersion = "v1.1.0"

// regulationSeconds is 90 minutes, the denominator for normalised match time.
const regulationSeconds = 5400.0

// FeatureEngine builds point-in-time feature vectors from MatchState.
type FeatureEngine struct{}

// NewFeatureEngine creates a new FeatureEngine.
func NewFeatureEngine() *FeatureEngine {
	return &FeatureEngine{}
}

// ExtractFeatures converts a MatchState into a map of named numerical features and a FeatureSnapshot.
func (fe *FeatureEngine) ExtractFeatures(state domain.MatchState) (map[string]float64, domain.FeatureSnapshot, error) {
	feats := make(map[string]float64)

	feats["match_second"] = float64(state.ClockSeconds)
	feats["period"] = float64(state.Period)
	feats["score_home"] = float64(state.Score.Home)
	feats["score_away"] = float64(state.Score.Away)
	feats["score_diff"] = float64(state.Score.Home - state.Score.Away)

	feats["red_cards_home"] = float64(state.RedCards.Home)
	feats["red_cards_away"] = float64(state.RedCards.Away)
	feats["red_cards_diff"] = float64(state.RedCards.Home - state.RedCards.Away)

	// Terms the trained hazard model consumes. Time enters as a fraction of
	// regulation plus its square, because the measured goal intensity climbs
	// steeply out of a cautious opening and then flattens — a shape a linear
	// term cannot follow. The scoreline and card terms use magnitudes rather
	// than signed differences: a two-goal gap raises the goal rate whichever
	// side is ahead (2.37 goals/90 when level against 2.77 at two apart).
	timeFrac := float64(state.ClockSeconds) / regulationSeconds
	feats["match_time_frac"] = timeFrac
	feats["match_time_frac_sq"] = timeFrac * timeFrac
	feats["abs_score_diff"] = math.Abs(float64(state.Score.Home - state.Score.Away))
	feats["goals_so_far"] = float64(state.Score.Home + state.Score.Away)
	feats["abs_red_diff"] = math.Abs(float64(state.RedCards.Home - state.RedCards.Away))

	// How much of the widest rolling window Golo has actually watched, from 0
	// (just arrived, windows necessarily empty) to 1 (a full 10 minutes
	// observed). Anything reading the activity windows must scale by this, or
	// the empty windows at kickoff — and at the moment Golo picks up a match
	// already in progress — look like an unusually dull passage of play.
	feats["activity_coverage"] = observedFraction(state)

	feats["yellow_cards_home"] = float64(state.YellowCards.Home)
	feats["yellow_cards_away"] = float64(state.YellowCards.Away)

	// Time deltas (seconds since the last event of each kind, capped — see maxDeltaSeconds)
	feats["last_goal_delta_sec"] = fe.deltaSec(state.ClockSeconds, state.LastGoalSecond)
	feats["last_shot_delta_sec"] = fe.deltaSec(state.ClockSeconds, state.LastShotSecond)
	feats["last_shot_on_target_delta_sec"] = fe.deltaSec(state.ClockSeconds, state.LastShotOnTargetSec)
	feats["last_card_delta_sec"] = fe.deltaSec(state.ClockSeconds, state.LastCardSecond)
	feats["last_sub_delta_sec"] = fe.deltaSec(state.ClockSeconds, state.LastSubstitutionSec)

	// Rolling window statistics
	for _, windowSec := range []int{60, 180, 300, 600} {
		wKey := fmt.Sprintf("%ds", windowSec)
		if windowSec == 60 {
			wKey = "1m"
		} else if windowSec == 180 {
			wKey = "3m"
		} else if windowSec == 300 {
			wKey = "5m"
		} else if windowSec == 600 {
			wKey = "10m"
		}

		ws, exists := state.Windows[windowSec]
		if !exists || ws == nil {
			feats["shots_"+wKey+"_home"] = 0
			feats["shots_"+wKey+"_away"] = 0
			feats["shots_"+wKey+"_total"] = 0
			feats["shots_on_target_"+wKey+"_total"] = 0
			feats["xg_"+wKey+"_home"] = 0
			feats["xg_"+wKey+"_away"] = 0
			feats["xg_"+wKey+"_total"] = 0
			feats["corners_"+wKey+"_total"] = 0
			continue
		}

		feats["shots_"+wKey+"_home"] = float64(ws.Home.Shots)
		feats["shots_"+wKey+"_away"] = float64(ws.Away.Shots)
		feats["shots_"+wKey+"_total"] = float64(ws.Home.Shots + ws.Away.Shots)
		feats["shots_"+wKey+"_diff"] = float64(ws.Home.Shots - ws.Away.Shots)

		feats["shots_on_target_"+wKey+"_home"] = float64(ws.Home.ShotsOnTarget)
		feats["shots_on_target_"+wKey+"_away"] = float64(ws.Away.ShotsOnTarget)
		feats["shots_on_target_"+wKey+"_total"] = float64(ws.Home.ShotsOnTarget + ws.Away.ShotsOnTarget)

		// Shots that missed, counted separately from those on target so the two
		// are disjoint. Overlapping counters (all shots plus on-target shots)
		// correlate at 0.61 and the fit splits one effect across both with
		// opposite signs, which had the model rating a shot on target as
		// *less* dangerous than one that missed.
		onTarget := ws.Home.ShotsOnTarget + ws.Away.ShotsOnTarget
		allShots := ws.Home.Shots + ws.Away.Shots
		offTarget := allShots - onTarget
		if offTarget < 0 {
			offTarget = 0
		}
		feats["shots_off_target_"+wKey+"_total"] = float64(offTarget)

		feats["xg_"+wKey+"_home"] = ws.Home.XG
		feats["xg_"+wKey+"_away"] = ws.Away.XG
		feats["xg_"+wKey+"_total"] = ws.Home.XG + ws.Away.XG
		feats["xg_"+wKey+"_diff"] = ws.Home.XG - ws.Away.XG

		feats["corners_"+wKey+"_home"] = float64(ws.Home.Corners)
		feats["corners_"+wKey+"_away"] = float64(ws.Away.Corners)
		feats["corners_"+wKey+"_total"] = float64(ws.Home.Corners + ws.Away.Corners)

		feats["dangerous_attacks_"+wKey+"_total"] = float64(ws.Home.DangerousAtks + ws.Away.DangerousAtks)
	}

	// Momentum feature: Compare 3m xG intensity vs 10m average xG intensity
	xg3m := feats["xg_3m_total"]
	xg10mAvg := feats["xg_10m_total"] / 3.3333
	feats["xg_momentum_surge"] = xg3m - xg10mAvg

	bytes, err := json.Marshal(feats)
	if err != nil {
		return nil, domain.FeatureSnapshot{}, err
	}

	snapshot := domain.FeatureSnapshot{
		SnapshotID:     fmt.Sprintf("snap_%s_%d", state.MatchID, state.ClockSeconds),
		MatchID:        state.MatchID,
		MatchSecond:    state.ClockSeconds,
		CutoffTime:     state.ProviderUpdatedAt,
		Features:       bytes,
		FeatureVersion: FeatureSchemaVersion,
		CreatedAt:      time.Now(),
	}

	return feats, snapshot, nil
}

// maxDeltaSeconds caps every "seconds since the last X" feature at the longest
// rolling window the model reasons over.
//
// The cap is not cosmetic. These features enter the model linearly, so an
// uncapped value keeps pushing the logit long after the signal has saturated:
// a match with no shot on record used to report 5400 seconds, which at the 5m
// horizon's -0.002 coefficient alone contributes -10.8 and drives the
// probability to the 0.001 floor no matter what else is happening on the
// pitch. Ten minutes without a shot and eighty minutes without one mean the
// same thing to a model whose widest window is ten minutes, so the feature
// saturates there.
const maxDeltaSeconds = 600.0

// observedFraction reports how much of the widest rolling window is backed by
// real observation.
func observedFraction(state domain.MatchState) float64 {
	if state.ObservedFromSecond < 0 {
		return 0
	}
	observed := float64(state.ClockSeconds - state.ObservedFromSecond)
	if observed <= 0 {
		return 0
	}
	if observed >= maxDeltaSeconds {
		return 1
	}
	return observed / maxDeltaSeconds
}

func (fe *FeatureEngine) deltaSec(current int, last *int) float64 {
	if last == nil {
		// Nothing of this kind observed yet — either it genuinely hasn't
		// happened, or Golo joined the match already in progress. Both are
		// "not recently", which is what the cap represents.
		return maxDeltaSeconds
	}
	diff := current - *last
	if diff < 0 {
		return 0
	}
	if float64(diff) > maxDeltaSeconds {
		return maxDeltaSeconds
	}
	return float64(diff)
}
