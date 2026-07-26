package parlayapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestFetchLiveRequiresScoreAndBuildsPairedTotal(t *testing.T) {
	now := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	responses := map[string]string{
		"/sports/soccer_brazil_campeonato/events": `[{"id":"e1","sport_title":"Brazil Serie A","commence_time":"2026-07-26T19:00:00Z","home_team":"Flamengo","away_team":"Palmeiras"}]`,
		"/sports/soccer_brazil_campeonato/odds":   `[{"id":"e1","sport_title":"Brazil Serie A","commence_time":"2026-07-26T19:00:00Z","home_team":"Flamengo","away_team":"Palmeiras","bookmakers":[{"key":"pinnacle","title":"Pinnacle","last_update":"2026-07-26T19:59:50Z","link":"https://book.test/e1","markets":[{"key":"totals","outcomes":[{"name":"Over","price":1.91,"point":2.5},{"name":"Under","price":1.99,"point":2.5}]}]}]}]`,
		"/sports/soccer_brazil_campeonato/scores": `[{"id":"e1","home_team":"Flamengo","away_team":"Palmeiras","completed":false,"scores":[{"name":"Flamengo","score":"1"},{"name":"Palmeiras","score":"1"}]}]`,
	}
	provider := New("secret", "https://fixture.invalid", "Pinnacle", []string{"soccer_brazil_campeonato"})
	provider.now = func() time.Time { return now }
	provider.httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("X-API-Key") != "secret" || request.URL.Query().Get("apiKey") != "secret" {
			t.Fatal("authentication missing")
		}
		return &http.Response{
			StatusCode: 200, Body: io.NopCloser(strings.NewReader(responses[request.URL.Path])),
			Header: make(http.Header),
		}, nil
	})}

	events, err := provider.FetchLive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || len(events[0].Quotes) != 1 {
		t.Fatalf("unexpected events: %+v", events)
	}
	quote := events[0].Quotes[0]
	if events[0].Provider != "parlay-api-shadow" || quote.Provider != "parlay-api-shadow" ||
		quote.Line != 2.5 || quote.Over != 1.91 || quote.Under != 1.99 ||
		quote.HomeScore != 1 || quote.AwayScore != 1 {
		t.Fatalf("unexpected quote: %+v", quote)
	}
}
