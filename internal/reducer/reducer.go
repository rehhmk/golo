package reducer

import (
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

// Supported rolling window sizes in seconds: 60s (1m), 180s (3m), 300s (5m), 600s (10m).
var SupportedWindows = []int{60, 180, 300, 600}

// Reducer processes canonical MatchEvents and produces an updated MatchState.
type Reducer struct {
	// Ring buffers or event histories per match for calculating rolling window stats
	events map[string][]domain.MatchEvent

	// prevStats holds the last cumulative statistics seen per match, so
	// ApplySnapshot can recover what happened between two polls by diffing.
	prevStats map[string]statBaseline
}

// statBaseline is the previous poll's cumulative statistics for one match.
type statBaseline struct {
	home domain.TeamStatTotals
	away domain.TeamStatTotals
}

// NewReducer initializes a new Reducer instance.
func NewReducer() *Reducer {
	return &Reducer{
		events:    make(map[string][]domain.MatchEvent),
		prevStats: make(map[string]statBaseline),
	}
}

// Reduce applies a MatchEvent to a MatchState and returns the updated MatchState deterministically.
func (r *Reducer) Reduce(state domain.MatchState, event domain.MatchEvent) domain.MatchState {
	newState := state
	newState.StateVersion++
	newState.ReceivedAt = event.ReceivedAt
	newState.Provider = event.Provider
	newState.ProviderUpdatedAt = event.ProviderTime

	if !event.ProviderTime.IsZero() && !event.ReceivedAt.IsZero() {
		lag := event.ReceivedAt.Sub(event.ProviderTime).Milliseconds()
		if lag > 0 {
			newState.FeedLagMs = lag
		}
	}

	if event.MatchSecond > newState.ClockSeconds {
		newState.ClockSeconds = event.MatchSecond
	}
	if event.Period > newState.Period {
		newState.Period = event.Period
	}

	r.events[event.MatchID] = append(r.events[event.MatchID], event)

	side := domain.ScoringTeamSide(event, newState.HomeTeamID)

	switch event.EventType {
	case domain.EventGoal, domain.EventPenaltyScored:
		if side == "home" {
			newState.Score.Home++
		} else {
			newState.Score.Away++
		}
		sec := event.MatchSecond
		newState.LastGoalSecond = &sec

	case domain.EventOwnGoal:
		if side == "home" {
			newState.Score.Home++
		} else {
			newState.Score.Away++
		}
		sec := event.MatchSecond
		newState.LastGoalSecond = &sec

	case domain.EventGoalDisallowed:
		if side == "home" && newState.Score.Home > 0 {
			newState.Score.Home--
		} else if side == "away" && newState.Score.Away > 0 {
			newState.Score.Away--
		}

	case domain.EventYellowCard:
		if side == "home" {
			newState.YellowCards.Home++
		} else {
			newState.YellowCards.Away++
		}
		sec := event.MatchSecond
		newState.LastCardSecond = &sec

	case domain.EventSecondYellow, domain.EventRedCard:
		if side == "home" {
			newState.RedCards.Home++
		} else {
			newState.RedCards.Away++
		}
		sec := event.MatchSecond
		newState.LastCardSecond = &sec

	case domain.EventSubstitution:
		if side == "home" {
			newState.Substitutions.Home++
		} else {
			newState.Substitutions.Away++
		}
		sec := event.MatchSecond
		newState.LastSubstitutionSec = &sec

	case domain.EventShot, domain.EventShotOnTarget, domain.EventShotBlocked, domain.EventShotOffTarget:
		sec := event.MatchSecond
		newState.LastShotSecond = &sec
		if event.EventType.IsOnTarget() {
			newState.LastShotOnTargetSec = &sec
		}

	case domain.EventPeriodStart:
		if newState.Status == domain.MatchStatusScheduled {
			newState.Status = domain.MatchStatusLive
		}
	case domain.EventHalfTime:
		newState.Status = domain.MatchStatusHalfTime
	case domain.EventFullTime:
		newState.Status = domain.MatchStatusFinished
	}

	newState.Windows = r.calculateRollingWindows(event.MatchID, newState.ClockSeconds, newState.HomeTeamID)

	return newState
}

// ApplySnapshot folds a provider's authoritative point-in-time view into the
// match state. It is the counterpart to Reduce: Reduce handles data that
// arrives as discrete events, ApplySnapshot handles data that arrives as a
// current value or a running total.
//
// Three things happen here that events alone cannot deliver:
//
//  1. The clock advances. Without this, ClockSeconds is pinned to the last
//     goal or card and every time-dependent probability goes stale.
//  2. Status and score come from the provider directly, so a match that
//     kicks off, breaks for half time or finishes is reflected even when the
//     feed publishes no explicit period events — and so the score stays right
//     even for goal variants the event-type mapping doesn't recognize.
//  3. Cumulative statistics are diffed against the previous snapshot and the
//     difference is replayed as synthetic events at the current clock, which
//     is what gives the rolling windows (and therefore the 5m/10m horizons)
//     any shot, corner or attack signal at all on feeds that report these as
//     totals rather than events.
//
// Synthetic events are deliberately kept in the in-memory rolling-window
// history only. They are inferences, not observations, so they are never
// returned to the caller and never reach the canonical event log.
func (r *Reducer) ApplySnapshot(state domain.MatchState, snap domain.LiveSnapshot) domain.MatchState {
	newState := state
	newState.StateVersion++

	if !snap.ReceivedAt.IsZero() {
		newState.ReceivedAt = snap.ReceivedAt
	}
	if !snap.ProviderTime.IsZero() {
		newState.ProviderUpdatedAt = snap.ProviderTime
	}
	if !snap.ProviderTime.IsZero() && !snap.ReceivedAt.IsZero() {
		if lag := snap.ReceivedAt.Sub(snap.ProviderTime).Milliseconds(); lag > 0 {
			newState.FeedLagMs = lag
		}
	}

	// The clock is monotonic. A provider that briefly reports a lower minute
	// (a mid-poll correction, or a period boundary racing the timer) must not
	// rewind match time, which would corrupt every rolling window at once.
	if snap.ClockSeconds > newState.ClockSeconds {
		newState.ClockSeconds = snap.ClockSeconds
	}
	// Record where observation began, so consumers can tell an empty rolling
	// window caused by a quiet match from one caused by having just arrived.
	if newState.ObservedFromSecond < 0 {
		newState.ObservedFromSecond = snap.ClockSeconds
	}
	if snap.Period > newState.Period {
		newState.Period = snap.Period
	}
	if snap.Status != "" {
		newState.Status = snap.Status
	}
	// Only overwrite the reduced score when the provider actually reports one,
	// otherwise an event-driven feed's tally would be zeroed on every poll.
	if snap.HasScore {
		newState.Score = snap.Score
	}

	if snap.HasStats {
		r.applyStatDeltas(&newState, snap)
	}

	newState.Windows = r.calculateRollingWindows(newState.MatchID, newState.ClockSeconds, newState.HomeTeamID)

	return newState
}

// applyStatDeltas turns the increase in each cumulative statistic since the
// last snapshot into synthetic events timestamped at the current clock.
//
// The first snapshot for a match only establishes a baseline and synthesizes
// nothing: a match joined at the 70th minute has already accumulated a dozen
// shots, and replaying them all at second 4200 would slam every rolling
// window with an attacking surge that never happened.
func (r *Reducer) applyStatDeltas(state *domain.MatchState, snap domain.LiveSnapshot) {
	baseline, seen := r.prevStats[snap.MatchID]
	r.prevStats[snap.MatchID] = statBaseline{home: snap.Home, away: snap.Away}
	if !seen {
		return
	}

	second := state.ClockSeconds
	homeID, awayID := state.HomeTeamID, state.AwayTeamID

	emit := func(teamID string, evType domain.EventType, count int) {
		for i := 0; i < count; i++ {
			team := teamID
			r.events[snap.MatchID] = append(r.events[snap.MatchID], domain.MatchEvent{
				MatchID:     snap.MatchID,
				Provider:    state.Provider,
				EventType:   evType,
				TeamID:      &team,
				MatchSecond: second,
				Period:      state.Period,
			})
		}
		if count > 0 && (evType == domain.EventShotOnTarget || evType == domain.EventShotOffTarget || evType == domain.EventShotBlocked) {
			sec := second
			state.LastShotSecond = &sec
			if evType == domain.EventShotOnTarget {
				state.LastShotOnTargetSec = &sec
			}
		}
	}

	for _, side := range []struct {
		teamID string
		prev   domain.TeamStatTotals
		cur    domain.TeamStatTotals
	}{
		{homeID, baseline.home, snap.Home},
		{awayID, baseline.away, snap.Away},
	} {
		emit(side.teamID, domain.EventShotOnTarget, delta(side.prev.ShotsOnTarget, side.cur.ShotsOnTarget))
		emit(side.teamID, domain.EventShotOffTarget, delta(side.prev.ShotsOffTarget, side.cur.ShotsOffTarget))
		emit(side.teamID, domain.EventShotBlocked, delta(side.prev.ShotsBlocked, side.cur.ShotsBlocked))
		emit(side.teamID, domain.EventCorner, delta(side.prev.Corners, side.cur.Corners))
		emit(side.teamID, domain.EventFoul, delta(side.prev.Fouls, side.cur.Fouls))
		emit(side.teamID, domain.EventDangerousAttack, delta(side.prev.DangerousAttacks, side.cur.DangerousAttacks))
	}
}

// delta returns how much a cumulative counter grew, ignoring any decrease.
// Providers do revise statistics downward (a shot reclassified, a stat
// corrected); treating that as negative activity would be meaningless, so a
// decrease simply contributes nothing and the new value becomes the baseline.
func delta(prev, cur int) int {
	if cur <= prev {
		return 0
	}
	return cur - prev
}

func (r *Reducer) calculateRollingWindows(matchID string, currentSecond int, homeTeamID string) map[int]*domain.WindowStats {
	windows := make(map[int]*domain.WindowStats)
	history := r.events[matchID]

	for _, wSec := range SupportedWindows {
		ws := &domain.WindowStats{
			WindowSeconds: wSec,
			Home:          domain.TeamStats{},
			Away:          domain.TeamStats{},
		}
		cutoffSecond := currentSecond - wSec

		for _, ev := range history {
			if ev.MatchSecond < cutoffSecond || ev.MatchSecond > currentSecond {
				continue
			}

			isHome := false
			if ev.TeamID != nil && *ev.TeamID == homeTeamID {
				isHome = true
			}

			if ev.EventType.IsShotEvent() {
				if isHome {
					ws.Home.Shots++
				} else {
					ws.Away.Shots++
				}
			}
			if ev.EventType.IsOnTarget() {
				if isHome {
					ws.Home.ShotsOnTarget++
				} else {
					ws.Away.ShotsOnTarget++
				}
			}
			if ev.EventType == domain.EventShotBlocked {
				if isHome {
					ws.Home.ShotsBlocked++
				} else {
					ws.Away.ShotsBlocked++
				}
			}
			if ev.XG != nil {
				if isHome {
					ws.Home.XG += *ev.XG
				} else {
					ws.Away.XG += *ev.XG
				}
			}
			if ev.EventType == domain.EventCorner {
				if isHome {
					ws.Home.Corners++
				} else {
					ws.Away.Corners++
				}
			}
			if ev.EventType == domain.EventFoul {
				if isHome {
					ws.Home.Fouls++
				} else {
					ws.Away.Fouls++
				}
			}
			if ev.EventType == domain.EventDangerousAttack {
				if isHome {
					ws.Home.DangerousAtks++
				} else {
					ws.Away.DangerousAtks++
				}
			}
		}

		windows[wSec] = ws
	}

	return windows
}

func (r *Reducer) Reset(matchID string) {
	delete(r.events, matchID)
	delete(r.prevStats, matchID)
}

var _ = time.Now
