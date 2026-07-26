package theoddsapi

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

func TestFetchLiveUsesFreeDiscoveryAndRequiresIndependentScore(t *testing.T) {
	now := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	responses := map[string]string{
		"/sports/soccer_brazil_campeonato/events": `[{
			"id":"e1","sport_key":"soccer_brazil_campeonato","sport_title":"Brazil Série A",
			"commence_time":"2026-07-26T19:00:00Z","home_team":"Flamengo","away_team":"Palmeiras"
		}]`,
		"/sports/soccer_brazil_campeonato/odds/": `[{
			"id":"e1","sport_key":"soccer_brazil_campeonato","sport_title":"Brazil Série A",
			"commence_time":"2026-07-26T19:00:00Z","home_team":"Flamengo","away_team":"Palmeiras",
			"bookmakers":[{"key":"betsson","title":"Betsson","last_update":"2026-07-26T19:59:45Z",
				"link":"https://book.example/event","markets":[{"key":"totals","outcomes":[
					{"name":"Over","price":1.80,"point":2.5,"link":"https://book.example/over"},
					{"name":"Under","price":2.10,"point":2.5}
				]}]}]
		}]`,
		"/sports/soccer_brazil_campeonato/scores/": `[{
			"id":"e1","sport_key":"soccer_brazil_campeonato","sport_title":"Brazil Série A",
			"commence_time":"2026-07-26T19:00:00Z","home_team":"Flamengo","away_team":"Palmeiras",
			"completed":false,"scores":[{"name":"Flamengo","score":"1"},{"name":"Palmeiras","score":"1"}]
		}]`,
	}
	var paths []string
	provider := New("secret", "https://fixture.invalid", "Betsson", []string{"soccer_brazil_campeonato"})
	provider.now = func() time.Time { return now }
	provider.httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		paths = append(paths, request.URL.Path)
		if request.URL.Query().Get("apiKey") != "secret" {
			t.Fatal("API key missing")
		}
		body, ok := responses[request.URL.Path]
		if !ok {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}

	events, err := provider.FetchLive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 || len(events) != 1 || len(events[0].Quotes) != 1 {
		t.Fatalf("paths=%v events=%+v", paths, events)
	}
	quote := events[0].Quotes[0]
	if quote.HomeScore != 1 || quote.AwayScore != 1 || quote.Line != 2.5 ||
		quote.Over != 1.8 || quote.Under != 2.1 || quote.DeepLink != "https://book.example/over" {
		t.Fatalf("wrong quote: %+v", quote)
	}
}

func TestFetchLiveSpendsNoOddsOrScoreCallWithoutLiveTarget(t *testing.T) {
	provider := New("secret", "https://fixture.invalid", "Betsson", []string{"soccer_usa_mls"})
	provider.now = func() time.Time { return time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC) }
	calls := 0
	provider.httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(strings.NewReader(`[{
				"id":"future","commence_time":"2026-07-27T20:00:00Z"
			}]`)),
			Header: make(http.Header),
		}, nil
	})}
	events, err := provider.FetchLive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(events) != 0 {
		t.Fatalf("calls=%d events=%v", calls, events)
	}
}
