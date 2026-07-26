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
