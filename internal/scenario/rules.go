package scenario

import (
	"errors"
	"fmt"
	"sort"
	"time"

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
	ID                string      `json:"id"`
	Name              string      `json:"name"`
	Version           int         `json:"version"`
	Conditions        []Condition `json:"conditions"`
	CompetitionIDs    []string    `json:"competitionIds,omitempty"`
	AdditionalGoals   int         `json:"additionalGoals"`
	MinimumOdds       float64     `json:"minimumOdds"`
	MinimumSamples    int         `json:"minimumSamples"`
	MinimumValidation int         `json:"minimumValidation"`
	// MinimumHoldout reads legacy persisted/request JSON. New writes and UI
	// use MinimumValidation.
	MinimumHoldout   int     `json:"minimumHoldout,omitempty"`
	MinimumModelEdge float64 `json:"minimumModelEdge"`
	Enabled          bool    `json:"enabled"`
}

type QualificationReport struct {
	Training   Result `json:"training"`
	Validation Result `json:"validation"`
	AllMatches Result `json:"allMatches"`
	// Holdout remains populated as a read-compatible alias for Validation.
	Holdout                  Result            `json:"holdout,omitempty"`
	Partition                PartitionManifest `json:"partition"`
	ValidationQualified      bool              `json:"validationQualified"`
	ModelValidationQualified bool              `json:"modelValidationQualified"`
	Qualified                bool              `json:"qualified"`
	ValidationFailures       []string          `json:"validationFailures"`
	Failures                 []string          `json:"failures"`
	DatasetMatches           int               `json:"datasetMatches"`
	ValidationMatches        int               `json:"validationMatches"`
	HoldoutMatches           int               `json:"holdoutMatches,omitempty"`
	RequiredProbability      float64           `json:"requiredProbability"`
	LockedTest               *LockedTestReport `json:"lockedTest,omitempty"`
}

type LockedTestReport struct {
	Rule             Result    `json:"rule"`
	Signal           Result    `json:"signal"`
	ModelBrier       float64   `json:"modelBrier"`
	BaselineBrier    float64   `json:"baselineBrier"`
	MarketBrier      float64   `json:"marketBrier"`
	CalibrationError float64   `json:"calibrationError"`
	ProfitUnits      float64   `json:"profitUnits"`
	ROIPct           float64   `json:"roiPct"`
	MaxDrawdownUnits float64   `json:"maxDrawdownUnits"`
	QuoteCoverage    float64   `json:"quoteCoverage"`
	Qualified        bool      `json:"qualified"`
	Failures         []string  `json:"failures"`
	RevealedAt       time.Time `json:"revealedAt"`
}

func DefaultDefinition() StrategyDefinition {
	return StrategyDefinition{
		Version:           1,
		AdditionalGoals:   1,
		MinimumOdds:       1.50,
		MinimumSamples:    500,
		MinimumValidation: 150,
		MinimumModelEdge:  0.08,
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
	if d.MinimumSamples < 1 || d.validationMinimum() < 1 {
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

func (d StrategyDefinition) validationMinimum() int {
	if d.MinimumValidation > 0 {
		return d.MinimumValidation
	}
	return d.MinimumHoldout
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

// BacktestDefinition exposes the already-inspected newest season as
// Validation. It never calls that data a final test: production qualification
// requires a separately sealed prospective Locked Test.
func BacktestDefinition(def StrategyDefinition, matches []MatchTimeline) (QualificationReport, error) {
	if err := def.Validate(); err != nil {
		return QualificationReport{}, err
	}
	trainingRows, validationRows, manifest := PartitionTimelines(matches)
	trainingRows = filterCompetitions(trainingRows, def.CompetitionIDs)
	validationRows = filterCompetitions(validationRows, def.CompetitionIDs)
	filtered := append(append(make([]MatchTimeline, 0, len(trainingRows)+len(validationRows)), trainingRows...), validationRows...)

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
	trainingResult := Evaluate(sc, trainingRows)
	validationResult := Evaluate(sc, validationRows)
	allResult := Evaluate(sc, filtered)
	required := 1 / def.MinimumOdds

	validationFailures := make([]string, 0)
	if allResult.MatchCount < def.MinimumSamples {
		validationFailures = append(validationFailures, fmt.Sprintf("amostra total %d < %d", allResult.MatchCount, def.MinimumSamples))
	}
	if validationResult.MatchCount < def.validationMinimum() {
		validationFailures = append(validationFailures, fmt.Sprintf("validação %d < %d", validationResult.MatchCount, def.validationMinimum()))
	}
	if validationResult.RateLow <= required {
		validationFailures = append(validationFailures, fmt.Sprintf("limite inferior de validação %.3f <= equilíbrio %.3f", validationResult.RateLow, required))
	}
	if len(manifest.ExcludedCompetitionIDs) > 0 && len(trainingRows)+len(validationRows) == 0 {
		validationFailures = append(validationFailures, "nenhuma competição possui duas temporadas elegíveis")
	}
	sort.Strings(validationFailures)
	failures := append([]string(nil), validationFailures...)
	failures = append(failures, "teste bloqueado ainda não revelado")
	return QualificationReport{
		Training: trainingResult, Validation: validationResult,
		AllMatches: allResult, Holdout: validationResult, Partition: manifest,
		ValidationQualified: len(validationFailures) == 0,
		Qualified:           false, ValidationFailures: validationFailures, Failures: failures,
		DatasetMatches: len(filtered), ValidationMatches: len(validationRows),
		HoldoutMatches: len(validationRows), RequiredProbability: required,
	}, nil
}

func filterCompetitions(matches []MatchTimeline, competitionIDs []string) []MatchTimeline {
	if len(competitionIDs) == 0 {
		return matches
	}
	filtered := make([]MatchTimeline, 0, len(matches))
	for _, match := range matches {
		if contains(competitionIDs, match.CompetitionID) {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
