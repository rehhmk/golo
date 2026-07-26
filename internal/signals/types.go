package signals

import (
	"fmt"
	"strings"
	"time"

	"github.com/enzotriches/golo/internal/domain"
	"github.com/enzotriches/golo/internal/odds"
	"github.com/enzotriches/golo/internal/scenario"
)

type Status string

const (
	StatusRejected  Status = "REJECTED"
	StatusQualified Status = "QUALIFIED"
	StatusWon       Status = "WON"
	StatusLost      Status = "LOST"
	StatusVoid      Status = "VOID"
)

type StoredStrategy struct {
	Definition scenario.StrategyDefinition  `json:"definition"`
	Report     scenario.QualificationReport `json:"report"`
	Armed      bool                         `json:"armed"`
	CreatedAt  time.Time                    `json:"createdAt"`
	UpdatedAt  time.Time                    `json:"updatedAt"`
}

type Decision struct {
	ID                string                       `json:"id"`
	DedupKey          string                       `json:"dedupKey"`
	StrategyID        string                       `json:"strategyId"`
	StrategyVersion   int                          `json:"strategyVersion"`
	StrategyName      string                       `json:"strategyName"`
	MatchID           string                       `json:"matchId"`
	HomeTeam          string                       `json:"homeTeam"`
	AwayTeam          string                       `json:"awayTeam"`
	Competition       string                       `json:"competition"`
	MatchSecond       int                          `json:"matchSecond"`
	StartGoals        int                          `json:"startGoals"`
	AdditionalGoals   int                          `json:"additionalGoals"`
	MarketLine        float64                      `json:"marketLine"`
	ModelProbability  float64                      `json:"modelProbability"`
	MarketProbability float64                      `json:"marketProbability"`
	Edge              float64                      `json:"edge"`
	Quote             odds.Quote                   `json:"quote"`
	Prediction        domain.Prediction            `json:"prediction"`
	State             domain.MatchState            `json:"state"`
	Qualification     scenario.QualificationReport `json:"qualification"`
	Gates             map[string]bool              `json:"gates"`
	Failures          []string                     `json:"failures"`
	Status            Status                       `json:"status"`
	CreatedAt         time.Time                    `json:"createdAt"`
	ResolvedAt        *time.Time                   `json:"resolvedAt,omitempty"`
}

type Subscriber struct {
	ID             string    `json:"id"`
	TelegramChatID int64     `json:"telegramChatId"`
	DisplayName    string    `json:"displayName"`
	Active         bool      `json:"active"`
	TermsVersion   string    `json:"termsVersion"`
	AdultConfirmed bool      `json:"adultConfirmed"`
	ExpiresAt      time.Time `json:"expiresAt"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Invitation struct {
	Code        string     `json:"code"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	AccessUntil time.Time  `json:"accessUntil"`
	UsedAt      *time.Time `json:"usedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type Delivery struct {
	ID           string    `json:"id"`
	SignalID     string    `json:"signalId"`
	SubscriberID string    `json:"subscriberId"`
	Kind         string    `json:"kind"`
	Status       string    `json:"status"`
	Attempts     int       `json:"attempts"`
	Error        string    `json:"error,omitempty"`
	MessageID    int       `json:"messageId,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Settings struct {
	AlertEngineEnabled bool          `json:"alertEngineEnabled"`
	TelegramEnabled    bool          `json:"telegramEnabled"`
	MinimumDataQuality float64       `json:"minimumDataQuality"`
	MaximumFeedLag     time.Duration `json:"maximumFeedLag"`
	MaximumQuoteAge    time.Duration `json:"maximumQuoteAge"`
	PostGoalCooldown   time.Duration `json:"postGoalCooldown"`
	Bookmaker          string        `json:"bookmaker"`
	AllowTwoGoalAlerts bool          `json:"allowTwoGoalAlerts"`
}

type Performance struct {
	Resolved          int     `json:"resolved"`
	Wins              int     `json:"wins"`
	Losses            int     `json:"losses"`
	Voids             int     `json:"voids"`
	ProfitUnits       float64 `json:"profitUnits"`
	ROIPct            float64 `json:"roiPct"`
	LongestLossStreak int     `json:"longestLossStreak"`
	MaxDrawdownUnits  float64 `json:"maxDrawdownUnits"`
}

// ComputePerformance uses a constant one-unit hypothetical stake. It measures
// the signals without turning historical outcomes into a stake recommendation.
func ComputePerformance(decisions []Decision) Performance {
	var report Performance
	equity, peak, lossRun := 0.0, 0.0, 0
	for i := len(decisions) - 1; i >= 0; i-- {
		decision := decisions[i]
		switch decision.Status {
		case StatusWon:
			report.Resolved++
			report.Wins++
			equity += decision.Quote.Over - 1
			lossRun = 0
		case StatusLost:
			report.Resolved++
			report.Losses++
			equity--
			lossRun++
			if lossRun > report.LongestLossStreak {
				report.LongestLossStreak = lossRun
			}
		case StatusVoid:
			report.Resolved++
			report.Voids++
			lossRun = 0
		default:
			continue
		}
		if equity > peak {
			peak = equity
		}
		if drawdown := peak - equity; drawdown > report.MaxDrawdownUnits {
			report.MaxDrawdownUnits = drawdown
		}
	}
	report.ProfitUnits = equity
	settledBets := report.Wins + report.Losses
	if settledBets > 0 {
		report.ROIPct = 100 * equity / float64(settledBets)
	}
	return report
}

func DefaultSettings() Settings {
	return Settings{
		MinimumDataQuality: 0.85,
		MaximumFeedLag:     15 * time.Second,
		MaximumQuoteAge:    60 * time.Second,
		PostGoalCooldown:   60 * time.Second,
		Bookmaker:          "Bet365",
	}
}

func (s Settings) Validate() error {
	if s.MinimumDataQuality < 0.85 || s.MinimumDataQuality > 1 {
		return fmt.Errorf("minimumDataQuality must be between 0.85 and 1")
	}
	if s.MaximumFeedLag <= 0 || s.MaximumFeedLag > time.Minute {
		return fmt.Errorf("maximumFeedLag must be between 1ns and 1 minute")
	}
	if s.MaximumQuoteAge <= 0 || s.MaximumQuoteAge > 2*time.Minute {
		return fmt.Errorf("maximumQuoteAge must be between 1ns and 2 minutes")
	}
	if s.PostGoalCooldown < time.Minute {
		return fmt.Errorf("postGoalCooldown must be at least 1 minute")
	}
	if strings.TrimSpace(s.Bookmaker) == "" {
		return fmt.Errorf("bookmaker is required")
	}
	return nil
}
