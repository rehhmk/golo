package odds

import (
	"math"
	"testing"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

func TestFairOverProbabilityRemovesOverround(t *testing.T) {
	got, err := FairOverProbability(1.80, 2.00)
	if err != nil {
		t.Fatal(err)
	}
	want := (1 / 1.80) / (1/1.80 + 1/2.00)
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("got %.8f want %.8f", got, want)
	}
}

func TestMatchEventRejectsAmbiguityAndScoreMismatch(t *testing.T) {
	kickoff := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	match := domain.Match{HomeTeamName: "São Paulo", AwayTeamName: "Grêmio", ScheduledAt: kickoff}
	state := domain.MatchState{Score: domain.ScoreState{Home: 1, Away: 0}}
	candidate := Event{ID: "a", Home: "Sao Paulo", Away: "Gremio", ScheduledAt: kickoff, HomeScore: 1}

	if got, err := MatchEvent(match, state, []Event{candidate}); err != nil || got.ID != "a" {
		t.Fatalf("expected unique normalized match, got %+v, %v", got, err)
	}
	wrongScore := candidate
	wrongScore.HomeScore = 0
	if _, err := MatchEvent(match, state, []Event{wrongScore}); err == nil {
		t.Fatal("score mismatch was accepted")
	}
	if _, err := MatchEvent(match, state, []Event{candidate, candidate}); err == nil {
		t.Fatal("ambiguous event mapping was accepted")
	}
}
