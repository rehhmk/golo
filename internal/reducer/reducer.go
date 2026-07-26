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
}

// NewReducer initializes a new Reducer instance.
func NewReducer() *Reducer {
	return &Reducer{
		events: make(map[string][]domain.MatchEvent),
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
}

var _ = time.Now
