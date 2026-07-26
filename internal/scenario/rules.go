package scenario

import (
	"errors"
	"fmt"
	"sort"

	"github.com/enzotriches/golo/internal/domain"
)

// RuleField is deliberately constrained so a strategy created in the UI can
// be evaluated identically against historical timelines and live state.
type RuleField string

const (
	FieldMinute          RuleField = "minute"
	FieldRemainingMinute RuleField = "remaining_minutes"
	FieldGoalsSoFar      RuleField = "goals_so_far"
	FieldAbsScoreDiff    RuleField = "abs_score_diff"
	FieldRedCardsTotal   RuleField = "red_cards_total"
)

type RuleOperator string

const (
	OpEqual          RuleOperator = "eq"
	OpGreaterOrEqual RuleOperator = "gte"
	OpLessOrEqual    RuleOperator = "lte"
)

type Condition struct {
	Field    RuleField    `json:"field"`
	Operator RuleOperator `json:"operator"`
	Value    int          `json:"value"`
}

// StrategyDefinition is the persisted, auditable strategy contract.
type StrategyDefinition struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	Version          int         `json:"version"`
	Conditions       []Condition `json:"conditions"`
	CompetitionIDs   []string    `json:"competitionIds,omitempty"`
	AdditionalGoals  int         `json:"additionalGoals"`
	MinimumOdds      float64     `json:"minimumOdds"`
	MinimumSamples   int         `json:"minimumSamples"`
	MinimumHoldout   int         `json:"minimumHoldout"`
	MinimumModelEdge float64     `json:"minimumModelEdge"`
	Enabled          bool        `json:"enabled"`
}

type QualificationReport struct {
	AllMatches          Result   `json:"allMatches"`
	Holdout             Result   `json:"holdout"`
	Qualified           bool     `json:"qualified"`
	Failures            []string `json:"failures"`
	DatasetMatches      int      `json:"datasetMatches"`
	HoldoutMatches      int      `json:"holdoutMatches"`
	RequiredProbability float64  `json:"requiredProbability"`
}

func DefaultDefinition() StrategyDefinition {
	return StrategyDefinition{
		Version:          1,
		AdditionalGoals:  1,
		MinimumOdds:      1.50,
		MinimumSamples:   500,
		MinimumHoldout:   150,
		MinimumModelEdge: 0.08,
	}
}

func (d StrategyDefinition) Validate() error {
	if d.Name == "" {
		return errors.New("strategy name is required")
	}
	if d.AdditionalGoals != 1 && d.AdditionalGoals != 2 {
		return errors.New("additionalGoals must be 1 or 2")
	}
	if d.MinimumOdds < 1.01 {
		return errors.New("minimumOdds must be at least 1.01")
	}
	if d.MinimumSamples < 1 || d.MinimumHoldout < 1 {
		return errors.New("sample requirements must be positive")
	}
	if d.MinimumModelEdge < 0 || d.MinimumModelEdge > 1 {
		return errors.New("minimumModelEdge must be between 0 and 1")
	}
	if len(d.Conditions) == 0 {
		return errors.New("at least one condition is required")
	}
	for _, c := range d.Conditions {
		switch c.Field {
		case FieldMinute, FieldRemainingMinute, FieldGoalsSoFar, FieldAbsScoreDiff, FieldRedCardsTotal:
		default:
			return fmt.Errorf("unsupported rule field %q", c.Field)
		}
		switch c.Operator {
		case OpEqual, OpGreaterOrEqual, OpLessOrEqual:
		default:
			return fmt.Errorf("unsupported rule operator %q", c.Operator)
		}
		if c.Value < 0 {
			return fmt.Errorf("condition %s cannot be negative", c.Field)
		}
	}
	return nil
}

func (d StrategyDefinition) Matches(state State, competitionID string) bool {
	if len(d.CompetitionIDs) > 0 && !contains(d.CompetitionIDs, competitionID) {
		return false
	}
	for _, condition := range d.Conditions {
		if !condition.matches(state) {
			return false
		}
	}
	return true
}

func (d StrategyDefinition) MatchesLive(state domain.MatchState) bool {
	return d.Matches(State{
		Second:     state.ClockSeconds,
		ScoreHome:  state.Score.Home,
		ScoreAway:  state.Score.Away,
		RedHome:    state.RedCards.Home,
		RedAway:    state.RedCards.Away,
		Remaining:  max(0, 5640-state.ClockSeconds),
		GoalsSoFar: state.Score.Home + state.Score.Away,
	}, state.CompetitionID)
}

func (c Condition) matches(s State) bool {
	var actual int
	switch c.Field {
	case FieldMinute:
		actual = s.Second / 60
	case FieldRemainingMinute:
		actual = s.Remaining / 60
	case FieldGoalsSoFar:
		actual = s.GoalsSoFar
	case FieldAbsScoreDiff:
		actual = s.AbsScoreDiff()
	case FieldRedCardsTotal:
		actual = s.RedHome + s.RedAway
	}
	switch c.Operator {
	case OpEqual:
		return actual == c.Value
	case OpGreaterOrEqual:
		return actual >= c.Value
	case OpLessOrEqual:
		return actual <= c.Value
	default:
		return false
	}
}

// BacktestDefinition reserves the newest season in each competition as the
// chronological holdout and never uses repeated minutes as observations.
func BacktestDefinition(def StrategyDefinition, matches []MatchTimeline) (QualificationReport, error) {
	if err := def.Validate(); err != nil {
		return QualificationReport{}, err
	}
	filtered := make([]MatchTimeline, 0, len(matches))
	for _, match := range matches {
		if len(def.CompetitionIDs) == 0 || contains(def.CompetitionIDs, match.CompetitionID) {
			filtered = append(filtered, match)
		}
	}

	latest := map[string]string{}
	for _, match := range filtered {
		if match.Season > latest[match.CompetitionID] {
			latest[match.CompetitionID] = match.Season
		}
	}
	holdout := make([]MatchTimeline, 0)
	for _, match := range filtered {
		if match.Season == latest[match.CompetitionID] {
			holdout = append(holdout, match)
		}
	}

	sc := Scenario{
		Name:            def.Name,
		Description:     "strategy definition",
		AdditionalGoals: def.AdditionalGoals,
		Trigger: func(state State) bool {
			// Competition filtering happened above; the trigger is the same
			// pure AND-rule used live.
			for _, condition := range def.Conditions {
				if !condition.matches(state) {
					return false
				}
			}
			return true
		},
	}
	allResult := Evaluate(sc, filtered)
	holdoutResult := Evaluate(sc, holdout)
	required := 1 / def.MinimumOdds

	var failures []string
	if allResult.MatchCount < def.MinimumSamples {
		failures = append(failures, fmt.Sprintf("amostra total %d < %d", allResult.MatchCount, def.MinimumSamples))
	}
	if holdoutResult.MatchCount < def.MinimumHoldout {
		failures = append(failures, fmt.Sprintf("holdout %d < %d", holdoutResult.MatchCount, def.MinimumHoldout))
	}
	if holdoutResult.RateLow <= required {
		failures = append(failures, fmt.Sprintf("limite inferior %.3f <= equilíbrio %.3f", holdoutResult.RateLow, required))
	}
	sort.Strings(failures)
	return QualificationReport{
		AllMatches:          allResult,
		Holdout:             holdoutResult,
		Qualified:           len(failures) == 0,
		Failures:            failures,
		DatasetMatches:      len(filtered),
		HoldoutMatches:      len(holdout),
		RequiredProbability: required,
	}, nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
