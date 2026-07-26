package features

import (
	"testing"

	"github.com/enzotriches/golo/internal/domain"
)

func TestFeatureEngine_ExtractFeatures(t *testing.T) {
	fe := NewFeatureEngine()
	state := domain.MatchState{
		MatchID:      "m1",
		ClockSeconds: 1800, // 30th min
		Period:       1,
		Score:        domain.ScoreState{Home: 1, Away: 0},
		RedCards:     domain.CardState{Home: 0, Away: 1},
		Windows: map[int]*domain.WindowStats{
			300: {
				WindowSeconds: 300,
				Home:          domain.TeamStats{Shots: 3, ShotsOnTarget: 2, XG: 0.45},
				Away:          domain.TeamStats{Shots: 1, ShotsOnTarget: 0, XG: 0.08},
			},
		},
	}

	feats, snap, err := fe.ExtractFeatures(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if feats["match_second"] != 1800 {
		t.Errorf("expected match_second 1800, got %f", feats["match_second"])
	}
	if feats["shots_5m_total"] != 4 {
		t.Errorf("expected shots_5m_total 4, got %f", feats["shots_5m_total"])
	}
	if feats["red_cards_diff"] != -1 {
		t.Errorf("expected red_cards_diff -1, got %f", feats["red_cards_diff"])
	}
	if snap.MatchID != "m1" {
		t.Errorf("expected snapshot MatchID m1, got %s", snap.MatchID)
	}
}

// A match with no shot history must not be pushed to a degenerate probability
// by the "time since last shot" sentinel alone. Before the cap this reported
// 5400, which at the 5m horizon's coefficient contributed -10.8 to the logit
// and pinned every live prediction to the 0.001 floor.
func TestDeltaFeaturesAreCappedAtLongestWindow(t *testing.T) {
	fe := NewFeatureEngine()

	feats, _, err := fe.ExtractFeatures(domain.MatchState{
		MatchID:      "m1",
		ClockSeconds: 4200,
		Windows:      map[int]*domain.WindowStats{},
	})
	if err != nil {
		t.Fatalf("ExtractFeatures: %v", err)
	}

	for _, name := range []string{
		"last_goal_delta_sec",
		"last_shot_delta_sec",
		"last_shot_on_target_delta_sec",
		"last_card_delta_sec",
		"last_sub_delta_sec",
	} {
		if got := feats[name]; got != maxDeltaSeconds {
			t.Errorf("%s = %v, want %v when nothing has been observed", name, got, maxDeltaSeconds)
		}
	}
}

func TestDeltaFeatureCapsAnOldEvent(t *testing.T) {
	fe := NewFeatureEngine()

	shotAt := 100
	feats, _, err := fe.ExtractFeatures(domain.MatchState{
		MatchID:        "m1",
		ClockSeconds:   4200, // over an hour since that shot
		LastShotSecond: &shotAt,
		Windows:        map[int]*domain.WindowStats{},
	})
	if err != nil {
		t.Fatalf("ExtractFeatures: %v", err)
	}

	if got := feats["last_shot_delta_sec"]; got != maxDeltaSeconds {
		t.Fatalf("last_shot_delta_sec = %v, want it capped at %v", got, maxDeltaSeconds)
	}
}

func TestDeltaFeaturePassesThroughRecentEvent(t *testing.T) {
	fe := NewFeatureEngine()

	shotAt := 4140
	feats, _, err := fe.ExtractFeatures(domain.MatchState{
		MatchID:        "m1",
		ClockSeconds:   4200,
		LastShotSecond: &shotAt,
		Windows:        map[int]*domain.WindowStats{},
	})
	if err != nil {
		t.Fatalf("ExtractFeatures: %v", err)
	}

	if got := feats["last_shot_delta_sec"]; got != 60 {
		t.Fatalf("last_shot_delta_sec = %v, want 60", got)
	}
}
