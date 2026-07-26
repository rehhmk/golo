package signals

import (
	"encoding/json"
	"time"

	"github.com/enzotriches/golo/internal/domain"
	"github.com/enzotriches/golo/internal/odds"
	"github.com/enzotriches/golo/internal/scenario"
)

const LockedCohortTarget = 150

type LockedTestState string

const (
	LockedStateDraft        LockedTestState = "DRAFT"
	LockedStateValidated    LockedTestState = "VALIDATED"
	LockedStateCollecting   LockedTestState = "COLLECTING"
	LockedStateReady        LockedTestState = "READY"
	LockedStateRevealedPass LockedTestState = "REVEALED_PASS"
	LockedStateRevealedFail LockedTestState = "REVEALED_FAIL"
)

type LockedOccurrenceStatus string

const (
	LockedOccurrenceOpen LockedOccurrenceStatus = "OPEN"
	LockedOccurrenceWon  LockedOccurrenceStatus = "WON"
	LockedOccurrenceLost LockedOccurrenceStatus = "LOST"
	LockedOccurrenceVoid LockedOccurrenceStatus = "VOID"
)

type ModelContract struct {
	ModelVersion       string  `json:"modelVersion"`
	ModelSHA256        string  `json:"modelSha256"`
	FeatureVersion     string  `json:"featureVersion"`
	OneGoalQualified   bool    `json:"oneGoalQualified"`
	TwoGoalQualified   bool    `json:"twoGoalQualified"`
	BaselineGoalsPer90 float64 `json:"baselineGoalsPer90"`
}

func (m ModelContract) Qualified(additionalGoals int) bool {
	if additionalGoals == 1 {
		return m.OneGoalQualified
	}
	if additionalGoals == 2 {
		return m.TwoGoalQualified
	}
	return false
}

type LockedTestContract struct {
	DefinitionSHA256        string                      `json:"definitionSha256"`
	PartitionManifestSHA256 string                      `json:"partitionManifestSha256"`
	Model                   ModelContract               `json:"model"`
	Definition              scenario.StrategyDefinition `json:"definition"`
	Bookmaker               string                      `json:"bookmaker"`
	MinimumDataQuality      float64                     `json:"minimumDataQuality"`
	MaximumFeedLag          time.Duration               `json:"maximumFeedLag"`
	MaximumQuoteAge         time.Duration               `json:"maximumQuoteAge"`
	PostGoalCooldown        time.Duration               `json:"postGoalCooldown"`
	AllowTwoGoalAlerts      bool                        `json:"allowTwoGoalAlerts"`
	TargetOccurrences       int                         `json:"targetOccurrences"`
	SettlementVersion       string                      `json:"settlementVersion"`
}

type LockedTest struct {
	ID               string                       `json:"id"`
	StrategyID       string                       `json:"strategyId"`
	StrategyVersion  int                          `json:"strategyVersion"`
	State            LockedTestState              `json:"state"`
	Contract         LockedTestContract           `json:"contract"`
	ContractSHA256   string                       `json:"contractSha256"`
	ValidationReport scenario.QualificationReport `json:"validationReport"`
	Report           *scenario.LockedTestReport   `json:"report,omitempty"`
	StartedAt        time.Time                    `json:"startedAt"`
	ReadyAt          *time.Time                   `json:"readyAt,omitempty"`
	RevealedAt       *time.Time                   `json:"revealedAt,omitempty"`
	CreatedAt        time.Time                    `json:"createdAt"`
	UpdatedAt        time.Time                    `json:"updatedAt"`
}

type LockedTestProgress struct {
	RuleAccepted        int `json:"ruleAccepted"`
	RuleResolved        int `json:"ruleResolved"`
	SignalAccepted      int `json:"signalAccepted"`
	SignalResolved      int `json:"signalResolved"`
	Voids               int `json:"voids"`
	RuleFeedsFresh      int `json:"ruleFeedsFresh"`
	RuleQuotesAvailable int `json:"ruleQuotesAvailable"`
	TargetOccurrences   int `json:"targetOccurrences"`
}

// LockedTestView is the only representation exposed by admin APIs. Report is
// absent until reveal, so collecting-state endpoints cannot leak outcomes.
type LockedTestView struct {
	ID              string                     `json:"id"`
	StrategyID      string                     `json:"strategyId"`
	StrategyVersion int                        `json:"strategyVersion"`
	State           LockedTestState            `json:"state"`
	Contract        LockedTestContract         `json:"contract"`
	ContractSHA256  string                     `json:"contractSha256"`
	Progress        LockedTestProgress         `json:"progress"`
	Report          *scenario.LockedTestReport `json:"report,omitempty"`
	StartedAt       time.Time                  `json:"startedAt"`
	ReadyAt         *time.Time                 `json:"readyAt,omitempty"`
	RevealedAt      *time.Time                 `json:"revealedAt,omitempty"`
}

type LockedOccurrence struct {
	ID                  string                 `json:"id"`
	TestID              string                 `json:"testId"`
	MatchID             string                 `json:"matchId"`
	RuleCohort          bool                   `json:"ruleCohort"`
	SignalCohort        bool                   `json:"signalCohort"`
	SignalEligible      bool                   `json:"signalEligible"`
	Status              LockedOccurrenceStatus `json:"status"`
	TriggerSecond       int                    `json:"triggerSecond"`
	StartGoals          int                    `json:"startGoals"`
	AdditionalGoals     int                    `json:"additionalGoals"`
	ModelProbability    float64                `json:"modelProbability"`
	BaselineProbability float64                `json:"baselineProbability"`
	MarketProbability   float64                `json:"marketProbability"`
	Quote               odds.Quote             `json:"quote"`
	StateSnapshot       domain.MatchState      `json:"stateSnapshot"`
	Prediction          domain.Prediction      `json:"prediction"`
	Gates               map[string]bool        `json:"gates"`
	GateFailures        []string               `json:"gateFailures"`
	CreatedAt           time.Time              `json:"createdAt"`
	ResolvedAt          *time.Time             `json:"resolvedAt,omitempty"`
}

// RawLockedOccurrence is used only for deterministic JSON persistence.
type RawLockedOccurrence struct {
	StateJSON      json.RawMessage
	PredictionJSON json.RawMessage
	QuoteJSON      json.RawMessage
	GatesJSON      json.RawMessage
}
