package oddsapiio

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestProviderConvertsRecordedTotalsResponse(t *testing.T) {
	fixture := `[{
		"id": 123, "home": "Flamengo", "away": "Palmeiras",
		"date": "2026-07-26T20:00:00Z", "status": "live",
		"league": {"name": "Serie A"},
		"scores": {"home": 1, "away": 1},
		"urls": {"Bet365": "https://book.example/event/123"},
		"bookmakers": {"Bet365": [{
			"name": "Over/Under", "updatedAt": "2026-07-26T21:10:00Z",
			"odds": [{"max": 2.5, "over": "1.80", "under": "2.10", "overDirectLink": "https://book.example/over"}]
		}]}
	}]`
	provider := New("secret", "https://fixture.invalid", "Bet365")
	provider.httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/odds/updated" || request.URL.Query().Get("apiKey") != "secret" {
			t.Fatalf("unexpected request %s", request.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(fixture)),
			Header:     make(http.Header),
		}, nil
	})}
	events, err := provider.FetchLive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || len(events[0].Quotes) != 1 {
		t.Fatalf("unexpected conversion: %+v", events)
	}
	quote := events[0].Quotes[0]
	if quote.Line != 2.5 || quote.Over != 1.8 || quote.Under != 2.1 || quote.DeepLink != "https://book.example/over" {
		t.Fatalf("wrong quote: %+v", quote)
	}
	if quote.HomeScore != 1 || quote.AwayScore != 1 || quote.UpdatedAt.IsZero() {
		t.Fatalf("score/timestamp missing: %+v", quote)
	}
}
