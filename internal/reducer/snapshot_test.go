package reducer

import (
	"testing"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

func newTestState() domain.MatchState {
	return domain.InitialState(domain.Match{
		ID:         "m1",
		HomeTeamID: "home_team",
		AwayTeamID: "away_team",
	})
}

func snapshotAt(clock int, home, away domain.TeamStatTotals) domain.LiveSnapshot {
	return domain.LiveSnapshot{
		MatchID:      "m1",
		Status:       domain.MatchStatusLive,
		Period:       2,
		ClockSeconds: clock,
		HasScore:     true,
		Score:        domain.ScoreState{Home: 1},
		HasStats:     true,
		Home:         home,
		Away:         away,
		ProviderTime: time.Now(),
		ReceivedAt:   time.Now(),
	}
}

// The whole point of snapshots: the clock must advance even when the feed
// reports no new events, or every time-dependent horizon goes stale.
func TestApplySnapshotAdvancesClockWithoutEvents(t *testing.T) {
	red := NewReducer()
	state := newTestState()

	state = red.ApplySnapshot(state, snapshotAt(4200, domain.TeamStatTotals{}, domain.TeamStatTotals{}))
	if state.ClockSeconds != 4200 {
		t.Fatalf("ClockSeconds = %d, want 4200", state.ClockSeconds)
	}
	if state.Status != domain.MatchStatusLive {
		t.Fatalf("Status = %s, want LIVE", state.Status)
	}
	if state.Period != 2 {
		t.Fatalf("Period = %d, want 2", state.Period)
	}

	state = red.ApplySnapshot(state, snapshotAt(4260, domain.TeamStatTotals{}, domain.TeamStatTotals{}))
	if state.ClockSeconds != 4260 {
		t.Fatalf("ClockSeconds = %d, want 4260 after second snapshot", state.ClockSeconds)
	}
}

func TestApplySnapshotClockNeverRewinds(t *testing.T) {
	red := NewReducer()
	state := newTestState()

	state = red.ApplySnapshot(state, snapshotAt(4200, domain.TeamStatTotals{}, domain.TeamStatTotals{}))
	state = red.ApplySnapshot(state, snapshotAt(3000, domain.TeamStatTotals{}, domain.TeamStatTotals{}))

	if state.ClockSeconds != 4200 {
		t.Fatalf("ClockSeconds = %d, want it held at 4200 after a backwards snapshot", state.ClockSeconds)
	}
}

// Joining a match already in progress must not replay its whole accumulated
// stat history at the current second — that would fabricate an attacking
// surge that never happened.
func TestFirstSnapshotOnlyEstablishesBaseline(t *testing.T) {
	red := NewReducer()
	state := newTestState()

	state = red.ApplySnapshot(state, snapshotAt(4200,
		domain.TeamStatTotals{ShotsOnTarget: 5, ShotsOffTarget: 6, Corners: 4},
		domain.TeamStatTotals{ShotsOnTarget: 3, Corners: 2},
	))

	w := state.Windows[600]
	if w == nil {
		t.Fatal("expected a 10m window")
	}
	if total := w.Home.Shots + w.Away.Shots; total != 0 {
		t.Fatalf("10m window shots = %d, want 0 on the first snapshot", total)
	}
	if state.LastShotSecond != nil {
		t.Fatalf("LastShotSecond = %v, want nil on the first snapshot", *state.LastShotSecond)
	}
}

func TestSnapshotStatDeltasFeedRollingWindows(t *testing.T) {
	red := NewReducer()
	state := newTestState()

	// Baseline at 70:00.
	state = red.ApplySnapshot(state, snapshotAt(4200,
		domain.TeamStatTotals{ShotsOnTarget: 5, ShotsOffTarget: 6, ShotsBlocked: 1, Corners: 4},
		domain.TeamStatTotals{ShotsOnTarget: 3},
	))

	// At 71:00 the home side has had 2 more on target, 1 off, 1 corner;
	// the away side 1 more on target.
	state = red.ApplySnapshot(state, snapshotAt(4260,
		domain.TeamStatTotals{ShotsOnTarget: 7, ShotsOffTarget: 7, ShotsBlocked: 1, Corners: 5},
		domain.TeamStatTotals{ShotsOnTarget: 4},
	))

	w := state.Windows[600]
	if w == nil {
		t.Fatal("expected a 10m window")
	}
	if w.Home.Shots != 3 {
		t.Fatalf("home shots in 10m window = %d, want 3", w.Home.Shots)
	}
	if w.Home.ShotsOnTarget != 2 {
		t.Fatalf("home shots on target = %d, want 2", w.Home.ShotsOnTarget)
	}
	if w.Home.Corners != 1 {
		t.Fatalf("home corners = %d, want 1", w.Home.Corners)
	}
	if w.Away.ShotsOnTarget != 1 {
		t.Fatalf("away shots on target = %d, want 1", w.Away.ShotsOnTarget)
	}
	if state.LastShotOnTargetSec == nil || *state.LastShotOnTargetSec != 4260 {
		t.Fatalf("LastShotOnTargetSec = %v, want 4260", state.LastShotOnTargetSec)
	}
}

// Providers do revise statistics downward. That must not be read as negative
// activity, and the corrected value must become the new baseline.
func TestSnapshotStatDecreaseIsIgnoredButRebaselines(t *testing.T) {
	red := NewReducer()
	state := newTestState()

	state = red.ApplySnapshot(state, snapshotAt(4200, domain.TeamStatTotals{ShotsOnTarget: 5}, domain.TeamStatTotals{}))
	state = red.ApplySnapshot(state, snapshotAt(4260, domain.TeamStatTotals{ShotsOnTarget: 4}, domain.TeamStatTotals{}))

	if w := state.Windows[600]; w.Home.Shots != 0 {
		t.Fatalf("home shots = %d, want 0 after a downward revision", w.Home.Shots)
	}

	// Back up to 5 is one shot relative to the corrected baseline of 4.
	state = red.ApplySnapshot(state, snapshotAt(4320, domain.TeamStatTotals{ShotsOnTarget: 5}, domain.TeamStatTotals{}))
	if w := state.Windows[600]; w.Home.Shots != 1 {
		t.Fatalf("home shots = %d, want 1 relative to the corrected baseline", w.Home.Shots)
	}
}

// An event-driven provider reports no score; its reduced tally must survive.
func TestApplySnapshotWithoutScoreKeepsReducedScore(t *testing.T) {
	red := NewReducer()
	state := newTestState()
	state.Score = domain.ScoreState{Home: 2, Away: 1}

	state = red.ApplySnapshot(state, domain.LiveSnapshot{
		MatchID:      "m1",
		Status:       domain.MatchStatusLive,
		ClockSeconds: 3000,
	})

	if state.Score.Home != 2 || state.Score.Away != 1 {
		t.Fatalf("Score = %+v, want it preserved at {2 1}", state.Score)
	}
}

// The provider's score is authoritative where it exists, so it wins over the
// event-derived tally — which is how own goals stay counted despite having no
// recognised event type.
func TestApplySnapshotWithScoreOverridesReducedScore(t *testing.T) {
	red := NewReducer()
	state := newTestState()
	state.Score = domain.ScoreState{Home: 1}

	state = red.ApplySnapshot(state, snapshotAt(3000, domain.TeamStatTotals{}, domain.TeamStatTotals{}))
	if state.Score.Home != 1 || state.Score.Away != 0 {
		t.Fatalf("Score = %+v, want {1 0} from the snapshot", state.Score)
	}

	snap := snapshotAt(3060, domain.TeamStatTotals{}, domain.TeamStatTotals{})
	snap.Score = domain.ScoreState{Home: 1, Away: 2}
	state = red.ApplySnapshot(state, snap)
	if state.Score.Away != 2 {
		t.Fatalf("Score.Away = %d, want 2 from the authoritative snapshot", state.Score.Away)
	}
}
