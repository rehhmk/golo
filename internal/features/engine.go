package features

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

const FeatureSchemaVersion = "v1.0.0"

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
