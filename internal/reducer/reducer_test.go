package reducer

import (
	"testing"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

func TestReducer_ReduceGoalAndRedCard(t *testing.T) {
	red := NewReducer()
	match := domain.Match{
		ID:         "m1",
		HomeTeamID: "home_team",
		AwayTeamID: "away_team",
	}
	state := domain.InitialState(match)

	now := time.Now()
	homeID := "home_team"
	awayID := "away_team"

	// Event 1: Period Start
	ev1 := domain.MatchEvent{
		EventID:     "e1",
		MatchID:     "m1",
		EventType:   domain.EventPeriodStart,
		Period:      1,
		MatchSecond: 0,
		ReceivedAt:  now,
	}
	state = red.Reduce(state, ev1)
	if state.Status != domain.MatchStatusLive {
		t.Errorf("expected match status LIVE, got %s", state.Status)
	}

	// Event 2: Home Goal
	ev2 := domain.MatchEvent{
		EventID:     "e2",
		MatchID:     "m1",
		EventType:   domain.EventGoal,
		TeamID:      &homeID,
		Period:      1,
		MatchSecond: 120,
		ReceivedAt:  now.Add(2 * time.Minute),
	}
	state = red.Reduce(state, ev2)
	if state.Score.Home != 1 || state.Score.Away != 0 {
		t.Errorf("expected score 1-0, got %d-%d", state.Score.Home, state.Score.Away)
	}

	// Event 3: Away Red Card
	ev3 := domain.MatchEvent{
		EventID:     "e3",
		MatchID:     "m1",
		EventType:   domain.EventRedCard,
		TeamID:      &awayID,
		Period:      1,
		MatchSecond: 300,
		ReceivedAt:  now.Add(5 * time.Minute),
	}
	state = red.Reduce(state, ev3)
	if state.RedCards.Away != 1 {
		t.Errorf("expected 1 away red card, got %d", state.RedCards.Away)
	}
}
