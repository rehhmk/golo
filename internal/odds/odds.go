// Package odds defines the small provider-neutral contract used by the alert
// engine. Provider payloads never leak into signal decisions or persistence.
package odds

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

type Quote struct {
	ProviderEventID string    `json:"providerEventId"`
	Bookmaker       string    `json:"bookmaker"`
	Market          string    `json:"market"`
	Line            float64   `json:"line"`
	Over            float64   `json:"over"`
	Under           float64   `json:"under"`
	Suspended       bool      `json:"suspended"`
	Stopped         bool      `json:"stopped"`
	UpdatedAt       time.Time `json:"updatedAt"`
	ReceivedAt      time.Time `json:"receivedAt"`
	DeepLink        string    `json:"deepLink"`
	HomeScore       int       `json:"homeScore"`
	AwayScore       int       `json:"awayScore"`
}

type Event struct {
	ID          string    `json:"id"`
	Home        string    `json:"home"`
	Away        string    `json:"away"`
	Competition string    `json:"competition"`
	ScheduledAt time.Time `json:"scheduledAt"`
	HomeScore   int       `json:"homeScore"`
	AwayScore   int       `json:"awayScore"`
	Quotes      []Quote   `json:"quotes"`
}

type Provider interface {
	Name() string
	FetchLive(context.Context) ([]Event, error)
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a",
		"é", "e", "ê", "e", "í", "i", "ó", "o",
		"ô", "o", "õ", "o", "ú", "u", "ç", "c",
	)
	return nonAlnum.ReplaceAllString(replacer.Replace(value), "")
}

// MatchEvent requires both teams, kickoff proximity and the current score.
// Ambiguity is rejected rather than attaching a price to the wrong match.
func MatchEvent(match domain.Match, state domain.MatchState, events []Event) (Event, error) {
	var candidates []Event
	for _, event := range events {
		teamsMatch := normalizeName(event.Home) == normalizeName(match.HomeTeamName) &&
			normalizeName(event.Away) == normalizeName(match.AwayTeamName)
		if !teamsMatch {
			continue
		}
		internalCompetition := normalizeName(match.CompetitionName)
		externalCompetition := normalizeName(event.Competition)
		if internalCompetition != "" && externalCompetition != "" &&
			!strings.Contains(internalCompetition, externalCompetition) &&
			!strings.Contains(externalCompetition, internalCompetition) {
			continue
		}
		if !event.ScheduledAt.IsZero() && !match.ScheduledAt.IsZero() {
			delta := event.ScheduledAt.Sub(match.ScheduledAt)
			if delta < -10*time.Minute || delta > 10*time.Minute {
				continue
			}
		}
		if event.HomeScore != state.Score.Home || event.AwayScore != state.Score.Away {
			continue
		}
		candidates = append(candidates, event)
	}
	if len(candidates) != 1 {
		return Event{}, errors.New("odds event match is missing or ambiguous")
	}
	return candidates[0], nil
}

// FairOverProbability removes a two-way market's proportional overround.
func FairOverProbability(over, under float64) (float64, error) {
	if over <= 1 || under <= 1 || math.IsNaN(over) || math.IsNaN(under) {
		return 0, errors.New("invalid two-way decimal odds")
	}
	qOver, qUnder := 1/over, 1/under
	return qOver / (qOver + qUnder), nil
}

func FindTotalsQuote(event Event, bookmaker string, line float64) (Quote, bool) {
	for _, quote := range event.Quotes {
		if !strings.EqualFold(quote.Bookmaker, bookmaker) {
			continue
		}
		if strings.EqualFold(quote.Market, "totals") && math.Abs(quote.Line-line) < 0.001 {
			return quote, true
		}
	}
	return Quote{}, false
}
