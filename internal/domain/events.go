package domain

// EventType represents the canonical type of a match event.
// All provider-specific event types must be mapped to one of these.
type EventType string

const (
	// Core match events.
	EventGoal           EventType = "GOAL"
	EventOwnGoal        EventType = "OWN_GOAL"
	EventGoalDisallowed EventType = "GOAL_DISALLOWED"
	EventPenaltyScored  EventType = "PENALTY_SCORED"
	EventPenaltyMissed  EventType = "PENALTY_MISSED"

	// Card events.
	EventYellowCard   EventType = "YELLOW_CARD"
	EventSecondYellow EventType = "SECOND_YELLOW"
	EventRedCard      EventType = "RED_CARD"

	// Substitution.
	EventSubstitution EventType = "SUBSTITUTION"

	// Shot events.
	EventShot          EventType = "SHOT"
	EventShotOnTarget  EventType = "SHOT_ON_TARGET"
	EventShotBlocked   EventType = "SHOT_BLOCKED"
	EventShotOffTarget EventType = "SHOT_OFF_TARGET"

	// Set pieces.
	EventCorner   EventType = "CORNER"
	EventFreeKick EventType = "FREE_KICK"
	EventThrowIn  EventType = "THROW_IN"
	EventGoalKick EventType = "GOAL_KICK"

	// Fouls.
	EventFoul    EventType = "FOUL"
	EventOffside EventType = "OFFSIDE"

	// Period transitions.
	EventPeriodStart    EventType = "PERIOD_START"
	EventPeriodEnd      EventType = "PERIOD_END"
	EventHalfTime       EventType = "HALF_TIME"
	EventFullTime       EventType = "FULL_TIME"
	EventExtraTimeStart EventType = "EXTRA_TIME_START"
	EventExtraTimeEnd   EventType = "EXTRA_TIME_END"

	// VAR and corrections.
	EventVARReview EventType = "VAR_REVIEW"
	EventRevision  EventType = "REVISION"

	// Other.
	EventDangerousAttack EventType = "DANGEROUS_ATTACK"
	EventInjury          EventType = "INJURY"
	EventUnknown         EventType = "UNKNOWN"
)

// IsGoalEvent returns true if the event type results in a goal being added.
func (e EventType) IsGoalEvent() bool {
	return e == EventGoal || e == EventOwnGoal || e == EventPenaltyScored
}

// IsCardEvent returns true if the event is a card.
func (e EventType) IsCardEvent() bool {
	return e == EventYellowCard || e == EventSecondYellow || e == EventRedCard
}

// IsShotEvent returns true if the event is any type of shot.
func (e EventType) IsShotEvent() bool {
	switch e {
	case EventShot, EventShotOnTarget, EventShotBlocked, EventShotOffTarget:
		return true
	default:
		return false
	}
}

// IsOnTarget returns true if the shot was on target (including goals).
func (e EventType) IsOnTarget() bool {
	return e == EventShotOnTarget || e.IsGoalEvent()
}

// IsPeriodEvent returns true if the event marks a period transition.
func (e EventType) IsPeriodEvent() bool {
	switch e {
	case EventPeriodStart, EventPeriodEnd, EventHalfTime, EventFullTime,
		EventExtraTimeStart, EventExtraTimeEnd:
		return true
	default:
		return false
	}
}

// ScoringTeamSide returns "home" or "away" for a goal event, given the
// team that produced it and the match context. For own goals, the scoring
// side is the *opposing* team.
func ScoringTeamSide(event MatchEvent, homeTeamID string) string {
	teamID := ""
	if event.TeamID != nil {
		teamID = *event.TeamID
	}

	if event.EventType == EventOwnGoal {
		// Own goal: the scoring team is the opponent.
		if teamID == homeTeamID {
			return "away"
		}
		return "home"
	}

	if teamID == homeTeamID {
		return "home"
	}
	return "away"
}

// ValidEventTypes returns all known event types for validation.
func ValidEventTypes() []EventType {
	return []EventType{
		EventGoal, EventOwnGoal, EventGoalDisallowed, EventPenaltyScored, EventPenaltyMissed,
		EventYellowCard, EventSecondYellow, EventRedCard,
		EventSubstitution,
		EventShot, EventShotOnTarget, EventShotBlocked, EventShotOffTarget,
		EventCorner, EventFreeKick, EventThrowIn, EventGoalKick,
		EventFoul, EventOffside,
		EventPeriodStart, EventPeriodEnd, EventHalfTime, EventFullTime,
		EventExtraTimeStart, EventExtraTimeEnd,
		EventVARReview, EventRevision,
		EventDangerousAttack, EventInjury, EventUnknown,
	}
}
