package sportmonks

import (
	"testing"

	"github.com/enzotriches/golo/internal/domain"
)

func TestSortByPriority(t *testing.T) {
	p := New("test-key", "https://example.invalid", []string{"libertadores-id", "brasileirao-id", "mls-id", "ligamx-id"})

	matches := []domain.Match{
		{ID: "m1", CompetitionID: "unranked-league"},
		{ID: "m2", CompetitionID: "mls-id"},
		{ID: "m3", CompetitionID: "brasileirao-id"},
		{ID: "m4", CompetitionID: "another-unranked"},
		{ID: "m5", CompetitionID: "libertadores-id"},
	}

	p.sortByPriority(matches)

	want := []string{"m5", "m3", "m2", "m1", "m4"}
	for i, m := range matches {
		if m.ID != want[i] {
			t.Fatalf("position %d: got %s, want %s (full order: %v)", i, m.ID, want[i], matchIDs(matches))
		}
	}
}

func TestSortByPriorityNoConfigLeavesOrderUnchanged(t *testing.T) {
	p := New("test-key", "https://example.invalid", nil)

	matches := []domain.Match{
		{ID: "m1", CompetitionID: "a"},
		{ID: "m2", CompetitionID: "b"},
	}
	p.sortByPriority(matches)

	if matches[0].ID != "m1" || matches[1].ID != "m2" {
		t.Fatalf("expected unchanged order, got %v", matchIDs(matches))
	}
}

func matchIDs(matches []domain.Match) []string {
	ids := make([]string, len(matches))
	for i, m := range matches {
		ids[i] = m.ID
	}
	return ids
}

func TestMapEventType(t *testing.T) {
	cases := []struct {
		typeID int
		want   domain.EventType
	}{
		{14, domain.EventGoal},
		{16, domain.EventPenaltyScored},
		{18, domain.EventSubstitution},
		{19, domain.EventYellowCard},
		{20, domain.EventRedCard},
		{21, domain.EventSecondYellow},
		{10, domain.EventUnknown},  // "Penalty awarded" — not itself a goal
		{999, domain.EventUnknown}, // anything unrecognized
	}

	for _, c := range cases {
		if got := mapEventType(c.typeID); got != c.want {
			t.Errorf("mapEventType(%d) = %s, want %s", c.typeID, got, c.want)
		}
	}
}
