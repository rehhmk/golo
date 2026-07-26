package theoddsapi

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

// Provider integrates The Odds API v4. It first uses the free /events
// endpoint to detect target competitions that actually have a live match,
// then spends quota only for those competitions on /odds and /scores.
type Provider struct {
	apiKey     string
	baseURL    string
	bookmaker  string
	sports     []string
	httpClient *http.Client
	now        func() time.Time
}

func New(apiKey, baseURL, bookmaker string, sports []string) *Provider {
	if baseURL == "" {
		baseURL = "https://api.the-odds-api.com/v4"
	}
	if bookmaker == "" {
		bookmaker = "Betsson"
	}
	return &Provider{
		apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"),
		bookmaker: bookmaker, sports: append([]string(nil), sports...),
		httpClient: &http.Client{Timeout: 10 * time.Second}, now: time.Now,
	}
}

func (p *Provider) Name() string { return "the-odds-api" }

type eventDTO struct {
	ID           string    `json:"id"`
	SportKey     string    `json:"sport_key"`
	SportTitle   string    `json:"sport_title"`
	CommenceTime time.Time `json:"commence_time"`
	HomeTeam     string    `json:"home_team"`
	AwayTeam     string    `json:"away_team"`
}

type oddsEventDTO struct {
	eventDTO
	Bookmakers []struct {
		Key        string    `json:"key"`
		Title      string    `json:"title"`
		LastUpdate time.Time `json:"last_update"`
		Link       string    `json:"link"`
		Markets    []struct {
			Key        string    `json:"key"`
			LastUpdate time.Time `json:"last_update"`
			Link       string    `json:"link"`
			Outcomes   []struct {
				Name  string  `json:"name"`
				Price float64 `json:"price"`
				Point float64 `json:"point"`
				Link  string  `json:"link"`
			} `json:"outcomes"`
		} `json:"markets"`
	} `json:"bookmakers"`
}

type scoreEventDTO struct {
	eventDTO
	Completed bool `json:"completed"`
	Scores    []struct {
		Name  string `json:"name"`
		Score string `json:"score"`
	} `json:"scores"`
}

func (p *Provider) FetchLive(ctx context.Context) ([]odds.Event, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("the odds api: API key is not configured")
	}
	now := p.now()
	var out []odds.Event
	for _, sport := range p.sports {
		var listed []eventDTO
		if err := p.get(ctx, "/sports/"+url.PathEscape(sport)+"/events", nil, &listed); err != nil {
			return nil, err
		}
		liveIDs := map[string]bool{}
		for _, event := range listed {
			// The provider defines an in-play event as one whose commence time
			// has passed. The four-hour bound prevents an abandoned or delayed
			// listing from consuming quota forever.
			if !event.CommenceTime.After(now) && event.CommenceTime.After(now.Add(-4*time.Hour)) {
				liveIDs[event.ID] = true
			}
		}
		if len(liveIDs) == 0 {
			continue
		}

		params := url.Values{
			"bookmakers":   {bookmakerKey(p.bookmaker)},
			"markets":      {"totals"},
			"oddsFormat":   {"decimal"},
			"dateFormat":   {"iso"},
			"includeLinks": {"true"},
		}
		var priced []oddsEventDTO
		if err := p.get(ctx, "/sports/"+url.PathEscape(sport)+"/odds/", params, &priced); err != nil {
			return nil, err
		}
		var scored []scoreEventDTO
		if err := p.get(ctx, "/sports/"+url.PathEscape(sport)+"/scores/", nil, &scored); err != nil {
			return nil, err
		}
		scores := scoreMap(scored)
		for _, event := range priced {
			if !liveIDs[event.ID] {
				continue
			}
			score, ok := scores[event.ID]
			if !ok {
				continue // never fabricate the independent quote score gate
			}
			converted := convertEvent(event, score, p.bookmaker, now)
			if len(converted.Quotes) > 0 {
				out = append(out, converted)
			}
		}
	}
	return out, nil
}

func (p *Provider) get(ctx context.Context, path string, params url.Values, target any) error {
	if params == nil {
		params = url.Values{}
	}
	params.Set("apiKey", p.apiKey)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	response, err := p.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("the odds api: request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the odds api: status %d: %s", response.StatusCode, string(body))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("the odds api: decode: %w", err)
	}
	return nil
}

type score struct{ home, away int }

func scoreMap(rows []scoreEventDTO) map[string]score {
	out := map[string]score{}
	for _, row := range rows {
		if row.Completed || len(row.Scores) == 0 {
			continue
		}
		current := score{home: -1, away: -1}
		for _, value := range row.Scores {
			parsed, err := strconv.Atoi(value.Score)
			if err != nil {
				continue
			}
			switch value.Name {
			case row.HomeTeam:
				current.home = parsed
			case row.AwayTeam:
				current.away = parsed
			}
		}
		if current.home >= 0 && current.away >= 0 {
			out[row.ID] = current
		}
	}
	return out
}

func convertEvent(row oddsEventDTO, current score, bookmaker string, receivedAt time.Time) odds.Event {
	event := odds.Event{
		ID: row.ID, Home: row.HomeTeam, Away: row.AwayTeam,
		Competition: row.SportTitle, ScheduledAt: row.CommenceTime,
		HomeScore: current.home, AwayScore: current.away,
	}
	for _, book := range row.Bookmakers {
		if !strings.EqualFold(book.Key, bookmakerKey(bookmaker)) && !strings.EqualFold(book.Title, bookmaker) {
			continue
		}
		for _, market := range book.Markets {
			if market.Key != "totals" {
				continue
			}
			type pair struct {
				over, under float64
				link        string
			}
			pairs := map[float64]pair{}
			for _, outcome := range market.Outcomes {
				currentPair := pairs[outcome.Point]
				switch strings.ToLower(outcome.Name) {
				case "over":
					currentPair.over = outcome.Price
					currentPair.link = outcome.Link
				case "under":
					currentPair.under = outcome.Price
				}
				pairs[outcome.Point] = currentPair
			}
			updated := market.LastUpdate
			if updated.IsZero() {
				updated = book.LastUpdate
			}
			for line, pair := range pairs {
				if pair.over <= 1 || pair.under <= 1 {
					continue
				}
				link := pair.link
				if link == "" {
					link = market.Link
				}
				if link == "" {
					link = book.Link
				}
				event.Quotes = append(event.Quotes, odds.Quote{
					ProviderEventID: row.ID, Bookmaker: book.Title, Market: "totals",
					Line: line, Over: pair.over, Under: pair.under,
					UpdatedAt: updated, ReceivedAt: receivedAt, DeepLink: link,
					HomeScore: current.home, AwayScore: current.away,
				})
			}
		}
	}
	return event
}

func bookmakerKey(value string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "-", "", "_", "").Replace(value))
}
