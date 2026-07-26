// Package domain contains the core types and business logic for Golo.
// It has zero external dependencies — no database, no API, no framework.
package domain

import (
	"encoding/json"
	"time"
)

// MatchEvent represents a single canonical event in a football match.
// Every event from any provider is normalized into this structure.
// See blueprint §7.4 for the original specification.
type MatchEvent struct {
	EventID         string          `json:"eventId"`
	MatchID         string          `json:"matchId"`
	Provider        string          `json:"provider"`
	ProviderEventID string          `json:"providerEventId"`
	EventType       EventType       `json:"eventType"`
	TeamID          *string         `json:"teamId,omitempty"`
	PlayerID        *string         `json:"playerId,omitempty"`
	MatchSecond     int             `json:"matchSecond"`
	Period          int             `json:"period"`
	ProviderTime    time.Time       `json:"providerTime"`
	ReceivedAt      time.Time       `json:"receivedAt"`
	ProcessedAt     time.Time       `json:"processedAt"`
	XG              *float64        `json:"xg,omitempty"`
	PositionX       *float64        `json:"positionX,omitempty"`
	PositionY       *float64        `json:"positionY,omitempty"`
	Attributes      json.RawMessage `json:"attributes,omitempty"`
	SchemaVersion   int             `json:"schemaVersion"`
}

// MatchStatus represents the lifecycle state of a match.
type MatchStatus string

const (
	MatchStatusScheduled MatchStatus = "SCHEDULED"
	MatchStatusLive      MatchStatus = "LIVE"
	MatchStatusHalfTime  MatchStatus = "HALF_TIME"
	MatchStatusPaused    MatchStatus = "PAUSED"
	MatchStatusFinished  MatchStatus = "FINISHED"
	MatchStatusCancelled MatchStatus = "CANCELLED"
	MatchStatusStale     MatchStatus = "STALE"
	MatchStatusError     MatchStatus = "ERROR"
)

// ScoreState holds the current score for both teams.
type ScoreState struct {
	Home int `json:"home"`
	Away int `json:"away"`
}

// CardState holds card counts for both teams.
type CardState struct {
	Home int `json:"home"`
	Away int `json:"away"`
}

// TeamStats holds rolling statistics for one team within a time window.
type TeamStats struct {
	Shots         int     `json:"shots"`
	ShotsOnTarget int     `json:"shotsOnTarget"`
	ShotsBlocked  int     `json:"shotsBlocked"`
	XG            float64 `json:"xg"`
	Corners       int     `json:"corners"`
	Fouls         int     `json:"fouls"`
	DangerousAtks int     `json:"dangerousAttacks"`
}

// WindowStats holds rolling statistics for a specific time window for both teams.
type WindowStats struct {
	WindowSeconds int       `json:"windowSeconds"`
	Home          TeamStats `json:"home"`
	Away          TeamStats `json:"away"`
}

// MatchState represents the full deterministic state of a match at a point in time.
// It is produced by the reducer and consumed by the feature engine and publisher.
// See blueprint §10.1.
type MatchState struct {
	MatchID       string      `json:"matchId"`
	Status        MatchStatus `json:"status"`
	Period        int         `json:"period"`
	ClockSeconds  int         `json:"clockSeconds"`
	Score         ScoreState  `json:"score"`
	RedCards      CardState   `json:"redCards"`
	YellowCards   CardState   `json:"yellowCards"`
	Substitutions CardState   `json:"substitutions"` // reusing CardState for home/away count

	// Rolling window stats for different time windows.
	Windows map[int]*WindowStats `json:"windows,omitempty"`

	// Provider metadata.
	Provider          string    `json:"provider"`
	ProviderUpdatedAt time.Time `json:"providerUpdatedAt"`
	ReceivedAt        time.Time `json:"receivedAt"`
	FeedLagMs         int64     `json:"feedLagMs"`

	// Versioning for deterministic replay comparison.
	StateVersion int `json:"stateVersion"`

	// ObservedFromSecond is the match clock at which Golo started watching,
	// or -1 before the first observation. Rolling windows are empty until
	// enough time has passed since then, and an empty window means "not
	// observed yet", not "nothing happened" — consumers weighing activity
	// need to tell those apart or they read the opening minutes of every
	// match, and every match joined in progress, as unusually quiet.
	ObservedFromSecond int `json:"observedFromSecond"`

	// Timestamps of significant recent events (for temporal features).
	LastGoalSecond      *int `json:"lastGoalSecond,omitempty"`
	LastShotSecond      *int `json:"lastShotSecond,omitempty"`
	LastShotOnTargetSec *int `json:"lastShotOnTargetSecond,omitempty"`
	LastCardSecond      *int `json:"lastCardSecond,omitempty"`
	LastSubstitutionSec *int `json:"lastSubstitutionSecond,omitempty"`

	// Home and away team identifiers.
	HomeTeamID string `json:"homeTeamId"`
	AwayTeamID string `json:"awayTeamId"`

	// Human-readable names for display. Providers that identify teams and
	// competitions by opaque numeric IDs supply these separately; when a
	// provider has no separate name the ID doubles as the label, so consumers
	// should fall back to the ID rather than render an empty string.
	HomeTeamName    string `json:"homeTeamName,omitempty"`
	AwayTeamName    string `json:"awayTeamName,omitempty"`
	CompetitionName string `json:"competitionName,omitempty"`

	// Competition context.
	CompetitionID string `json:"competitionId"`
	SeasonID      string `json:"seasonId,omitempty"`
}

// TeamStatTotals holds cumulative match statistics for one team, as reported
// by a provider that publishes running totals rather than discrete events.
type TeamStatTotals struct {
	ShotsOnTarget    int     `json:"shotsOnTarget"`
	ShotsOffTarget   int     `json:"shotsOffTarget"`
	ShotsBlocked     int     `json:"shotsBlocked"`
	Corners          int     `json:"corners"`
	Fouls            int     `json:"fouls"`
	DangerousAttacks int     `json:"dangerousAttacks"`
	Possession       float64 `json:"possession"`
}

// LiveSnapshot is a provider's authoritative point-in-time view of a match:
// the data a feed publishes as a current value or a running total rather than
// as a discrete event — the match clock above all, plus status, score and
// aggregate statistics.
//
// It exists because an event feed alone cannot drive a live probability. A
// match can go ten minutes without emitting a single event, yet every horizon
// Golo predicts ("goal in the next 5 minutes", "goal before full time")
// depends on the clock having moved. Reducing only events leaves ClockSeconds
// frozen at the last goal or card, so the published probability silently goes
// stale. Snapshots let the reducer advance state on every poll tick.
//
// The HasScore and HasStats flags distinguish "this provider does not report
// this data" from "the reported value is genuinely zero". Without them a feed
// that omits the score would silently reset a 2-0 match to 0-0 on every poll.
type LiveSnapshot struct {
	MatchID      string      `json:"matchId"`
	Status       MatchStatus `json:"status"`
	Period       int         `json:"period"`
	ClockSeconds int         `json:"clockSeconds"`

	HasScore bool       `json:"hasScore"`
	Score    ScoreState `json:"score"`

	HasStats bool           `json:"hasStats"`
	Home     TeamStatTotals `json:"home"`
	Away     TeamStatTotals `json:"away"`

	ProviderTime time.Time `json:"providerTime"`
	ReceivedAt   time.Time `json:"receivedAt"`
}

// PredictionStatus indicates the operational confidence of a prediction.
type PredictionStatus string

const (
	PredictionStatusOK          PredictionStatus = "OK"
	PredictionStatusStale       PredictionStatus = "STALE"
	PredictionStatusDegraded    PredictionStatus = "DEGRADED"
	PredictionStatusUnavailable PredictionStatus = "UNAVAILABLE"
)

// ConfidenceBand represents the quality-derived confidence level.
type ConfidenceBand string

const (
	ConfidenceHigh   ConfidenceBand = "HIGH"
	ConfidenceMedium ConfidenceBand = "MEDIUM"
	ConfidenceLow    ConfidenceBand = "LOW"
)

// Probabilities holds the predicted probabilities for all horizons.
type Probabilities struct {
	GoalNext5m              float64 `json:"goalNext5m"`
	GoalNext10m             float64 `json:"goalNext10m"`
	GoalBeforeFullTime      float64 `json:"goalBeforeFullTime"`
	TwoOrMoreBeforeFullTime float64 `json:"twoOrMoreGoalsBeforeFullTime"`
	HomeGoalBeforeFullTime  float64 `json:"homeGoalBeforeFullTime,omitempty"`
	AwayGoalBeforeFullTime  float64 `json:"awayGoalBeforeFullTime,omitempty"`
}

// FeatureContribution is one auditable term in the hazard model's
// log-intensity. Contribution always equals Value*Coefficient.
type FeatureContribution struct {
	Name         string  `json:"name"`
	Value        float64 `json:"value"`
	Coefficient  float64 `json:"coefficient"`
	Contribution float64 `json:"contribution"`
}

// Prediction represents the model output for a match at a given instant.
// See blueprint §10.2.
type Prediction struct {
	MatchID                string                `json:"matchId"`
	AsOfMatchSecond        int                   `json:"asOfMatchSecond"`
	CalculatedAt           time.Time             `json:"calculatedAt"`
	Probabilities          Probabilities         `json:"probabilities"`
	DataQuality            float64               `json:"dataQuality"`
	ConfidenceBand         ConfidenceBand        `json:"confidenceBand"`
	Status                 PredictionStatus      `json:"status"`
	ModelVersion           string                `json:"modelVersion"`
	CalibratorVersion      string                `json:"calibratorVersion"`
	FeatureVersion         string                `json:"featureVersion"`
	PredictionSequence     int                   `json:"predictionSequence"`
	ExpectedGoalsRemaining float64               `json:"expectedGoalsRemaining"`
	Contributions          []FeatureContribution `json:"contributions,omitempty"`
}

// ProviderHealth captures the operational status of a data provider.
type ProviderHealth struct {
	Provider      string    `json:"provider"`
	IsHealthy     bool      `json:"isHealthy"`
	LastSuccessAt time.Time `json:"lastSuccessAt,omitempty"`
	LastErrorAt   time.Time `json:"lastErrorAt,omitempty"`
	ErrorCount    int       `json:"errorCount"`
	AvgLatencyMs  int64     `json:"avgLatencyMs"`
	Message       string    `json:"message,omitempty"`
}

// Match represents a scheduled or active football match.
type Match struct {
	ID              string      `json:"id"`
	Provider        string      `json:"provider"`
	ProviderMatchID string      `json:"providerMatchId"`
	CompetitionID   string      `json:"competitionId"`
	SeasonID        string      `json:"seasonId,omitempty"`
	HomeTeamID      string      `json:"homeTeamId"`
	AwayTeamID      string      `json:"awayTeamId"`
	HomeTeamName    string      `json:"homeTeamName,omitempty"`
	AwayTeamName    string      `json:"awayTeamName,omitempty"`
	CompetitionName string      `json:"competitionName,omitempty"`
	ScheduledAt     time.Time   `json:"scheduledAt"`
	Status          MatchStatus `json:"status"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
}

// FeatureSnapshot represents a point-in-time feature vector for model input.
type FeatureSnapshot struct {
	SnapshotID     string          `json:"snapshotId"`
	MatchID        string          `json:"matchId"`
	MatchSecond    int             `json:"matchSecond"`
	CutoffTime     time.Time       `json:"cutoffTime"`
	Features       json.RawMessage `json:"features"`
	FeatureVersion string          `json:"featureVersion"`
	CreatedAt      time.Time       `json:"createdAt"`
}

// ModelArtifact represents a trained model exported as JSON.
// See blueprint §12.5.
type ModelArtifact struct {
	ModelVersion   string             `json:"modelVersion"`
	FeatureVersion string             `json:"featureVersion"`
	HorizonSeconds int                `json:"horizonSeconds"`
	Intercept      float64            `json:"intercept"`
	Coefficients   map[string]float64 `json:"coefficients"`
	Normalization  map[string]float64 `json:"normalization,omitempty"`
	Calibration    CalibrationParams  `json:"calibration"`
	TrainedUntil   string             `json:"trainedUntil"`
	SHA256         string             `json:"sha256"`
}

// CalibrationParams holds parameters for probability calibration.
type CalibrationParams struct {
	Type string  `json:"type"` // "platt" or "isotonic"
	A    float64 `json:"a"`
	B    float64 `json:"b"`
}

// InitialState returns a fresh MatchState for a match that hasn't started yet.
func InitialState(match Match) MatchState {
	return MatchState{
		MatchID:       match.ID,
		Status:        MatchStatusScheduled,
		Period:        0,
		ClockSeconds:  0,
		Score:         ScoreState{Home: 0, Away: 0},
		RedCards:      CardState{Home: 0, Away: 0},
		YellowCards:   CardState{Home: 0, Away: 0},
		Substitutions: CardState{Home: 0, Away: 0},
		Windows:       make(map[int]*WindowStats),
		HomeTeamID:    match.HomeTeamID,
		AwayTeamID:    match.AwayTeamID,
		CompetitionID: match.CompetitionID,
		SeasonID:      match.SeasonID,
		StateVersion:  0,

		// Nothing observed yet; the first snapshot sets this.
		ObservedFromSecond: -1,

		HomeTeamName:    match.HomeTeamName,
		AwayTeamName:    match.AwayTeamName,
		CompetitionName: match.CompetitionName,
	}
}
