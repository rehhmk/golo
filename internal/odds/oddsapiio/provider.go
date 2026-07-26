package oddsapiio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/enzotriches/golo/internal/odds"
)

// Provider uses Odds-API.io's incremental live endpoint. The adapter is kept
// intentionally small so a commercial feed can replace it without touching
// signal logic.
type Provider struct {
	apiKey     string
	baseURL    string
	bookmaker  string
	httpClient *http.Client
	since      time.Time
}

func New(apiKey, baseURL, bookmaker string) *Provider {
	if baseURL == "" {
		baseURL = "https://api.odds-api.io/v3"
	}
	if bookmaker == "" {
		bookmaker = "Bet365"
	}
	return &Provider{
		apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), bookmaker: bookmaker,
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
}

func (p *Provider) Name() string { return "odds-api.io" }

type eventDTO struct {
	ID     any    `json:"id"`
	Home   string `json:"home"`
	Away   string `json:"away"`
	Date   string `json:"date"`
	Status string `json:"status"`
	League struct {
		Name string `json:"name"`
	} `json:"league"`
	Scores struct {
		Home int `json:"home"`
		Away int `json:"away"`
	} `json:"scores"`
	URLs       map[string]string   `json:"urls"`
	Bookmakers map[string][]market `json:"bookmakers"`
}

type market struct {
	Name      string `json:"name"`
	UpdatedAt string `json:"updatedAt"`
	Suspended bool   `json:"suspended"`
	Stopped   bool   `json:"stopped"`
	Odds      []struct {
		Max             any    `json:"max"`
		Over            any    `json:"over"`
		Under           any    `json:"under"`
		OverDirectLink  string `json:"overDirectLink"`
		UnderDirectLink string `json:"underDirectLink"`
		Suspended       bool   `json:"suspended"`
		Stopped         bool   `json:"stopped"`
	} `json:"odds"`
}

func (p *Provider) FetchLive(ctx context.Context) ([]odds.Event, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("odds-api.io: API key is not configured")
	}
	since := p.since
	if since.IsZero() || time.Since(since) > time.Minute {
		since = time.Now().Add(-time.Minute)
	}
	query := url.Values{}
	query.Set("apiKey", p.apiKey)
	query.Set("since", strconv.FormatInt(since.Unix(), 10))
	query.Set("bookmaker", p.bookmaker)
	query.Set("sport", "football")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/odds/updated?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	response, err := p.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("odds-api.io: request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("odds-api.io: status %d: %s", response.StatusCode, string(body))
	}
	var rows []eventDTO
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("odds-api.io: decode: %w", err)
	}
	now := time.Now()
	p.since = now.Add(-2 * time.Second)
	return convert(rows, p.bookmaker, now), nil
}

func convert(rows []eventDTO, bookmaker string, receivedAt time.Time) []odds.Event {
	out := make([]odds.Event, 0, len(rows))
	for _, row := range rows {
		event := odds.Event{
			Provider: "odds-api.io",
			ID:       fmt.Sprint(row.ID), Home: row.Home, Away: row.Away,
			Competition: row.League.Name, HomeScore: row.Scores.Home, AwayScore: row.Scores.Away,
		}
		event.ScheduledAt, _ = time.Parse(time.RFC3339, row.Date)
		markets, ok := row.Bookmakers[bookmaker]
		if !ok {
			for name, candidate := range row.Bookmakers {
				if strings.EqualFold(name, bookmaker) {
					markets, ok = candidate, true
					break
				}
			}
		}
		if !ok {
			continue
		}
		link := row.URLs[bookmaker]
		if link == "" {
			for name, candidate := range row.URLs {
				if strings.EqualFold(name, bookmaker) {
					link = candidate
					break
				}
			}
		}
		for _, market := range markets {
			if !strings.EqualFold(market.Name, "totals") && !strings.EqualFold(market.Name, "over/under") {
				continue
			}
			for _, selection := range market.Odds {
				line, over, under := number(selection.Max), number(selection.Over), number(selection.Under)
				if line == 0 || over == 0 || under == 0 {
					continue
				}
				updated, _ := time.Parse(time.RFC3339, market.UpdatedAt)
				selectionLink := selection.OverDirectLink
				if selectionLink == "" {
					selectionLink = link
				}
				event.Quotes = append(event.Quotes, odds.Quote{
					Provider:        "odds-api.io",
					ProviderEventID: event.ID, Bookmaker: bookmaker, Market: "totals",
					Line: line, Over: over, Under: under,
					Suspended: market.Suspended || selection.Suspended,
					Stopped:   market.Stopped || selection.Stopped,
					UpdatedAt: updated, ReceivedAt: receivedAt, DeepLink: selectionLink,
					HomeScore: event.HomeScore, AwayScore: event.AwayScore,
				})
			}
		}
		out = append(out, event)
	}
	return out
}

func number(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case string:
		parsed, _ := strconv.ParseFloat(typed, 64)
		return parsed
	default:
		return 0
	}
}
