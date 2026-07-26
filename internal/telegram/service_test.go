package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/enzotriches/golo/internal/odds"
	"github.com/enzotriches/golo/internal/scenario"
	"github.com/enzotriches/golo/internal/signals"
)

type testStore struct {
	deliveries []signals.Delivery
	redeemed   bool
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func (s *testStore) ListActiveSubscribers() ([]signals.Subscriber, error) {
	return []signals.Subscriber{{ID: "u1", TelegramChatID: 42, Active: true}}, nil
}
func (s *testStore) ListSignals(int) ([]signals.Decision, error) { return nil, nil }
func (s *testStore) RedeemInvitation(string, signals.Subscriber) (bool, error) {
	if s.redeemed {
		return false, nil
	}
	s.redeemed = true
	return true, nil
}
func (s *testStore) SaveDelivery(d signals.Delivery) error {
	s.deliveries = append(s.deliveries, d)
	return nil
}

func TestSignalIncludesEvidenceWarningAndDeepLinkWithRetry(t *testing.T) {
	requests := 0
	var lastPayload map[string]any
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		_ = json.NewDecoder(request.Body).Decode(&lastPayload)
		if requests == 1 {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("temporary")), Header: make(http.Header)}, nil
		}
		return &http.Response{
			StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":99}}`)),
			Header: make(http.Header),
		}, nil
	})}
	store := &testStore{}
	service := New("token", "secret", store)
	service.baseURL = "https://telegram.fixture.invalid"
	service.client = client
	decision := signals.Decision{
		ID: "s1", HomeTeam: "A", AwayTeam: "B", MatchSecond: 4200, MarketLine: 2.5,
		ModelProbability: .75, MarketProbability: .60, Edge: .15,
		Quote: odds.Quote{Bookmaker: "Bet365", Over: 1.8, UpdatedAt: time.Now(), DeepLink: "https://book.example/market"},
		Qualification: scenario.QualificationReport{Holdout: scenario.Result{
			HitRate: .75, RateLow: .68, RateHigh: .80, MatchCount: 200, OddsHigh: 1.47,
		}},
	}
	if err := service.SendSignal(context.Background(), decision); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(store.deliveries) != 1 || store.deliveries[0].Status != "sent" || store.deliveries[0].Attempts != 2 {
		t.Fatalf("retry/delivery audit failed: requests=%d deliveries=%+v", requests, store.deliveries)
	}
	text, _ := lastPayload["text"].(string)
	if !strings.Contains(text, "Aposta não é investimento") || !strings.Contains(text, "IC 95%") {
		t.Fatalf("warning/evidence missing from %q", text)
	}
	markup, _ := json.Marshal(lastPayload["reply_markup"])
	if !strings.Contains(string(markup), "https://book.example/market") {
		t.Fatalf("deep link missing from %s", markup)
	}
}

func TestStartCodeAcceptsPrivateAndGroupDeepLinks(t *testing.T) {
	for input, want := range map[string]string{
		"/start abc123":             "abc123",
		"/start@AskGolo_bot abc123": "abc123",
		"/START@ASKGOLO_BOT abc123": "abc123",
		"/start":                    "",
		"/help abc123":              "",
	} {
		if got := startCode(input); got != want {
			t.Errorf("startCode(%q)=%q want %q", input, got, want)
		}
	}
}

func TestRepeatedInvitationCallbackIsAcknowledgedWithoutFalseInvalidMessage(t *testing.T) {
	var methods []string
	var texts []string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		methods = append(methods, request.URL.Path)
		var payload map[string]any
		_ = json.NewDecoder(request.Body).Decode(&payload)
		if text, ok := payload["text"].(string); ok {
			texts = append(texts, text)
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":99}}`)),
			Header:     make(http.Header),
		}, nil
	})}
	store := &testStore{}
	service := New("token", "secret", store)
	service.baseURL = "https://telegram.fixture.invalid"
	service.client = client

	var update Update
	raw := `{
		"callback_query": {
			"id": "callback-1",
			"data": "accept:invite-code",
			"from": {"id": 7, "first_name": "Enzo", "username": "enzo"},
			"message": {"message_id": 42, "chat": {"id": 123}}
		}
	}`
	if err := json.Unmarshal([]byte(raw), &update); err != nil {
		t.Fatal(err)
	}
	if err := service.HandleWebhook(context.Background(), "secret", update); err != nil {
		t.Fatal(err)
	}
	update.Callback.ID = "callback-2"
	if err := service.HandleWebhook(context.Background(), "secret", update); err != nil {
		t.Fatal(err)
	}

	if len(methods) != 5 {
		t.Fatalf("Telegram calls=%v, want answer/edit/send then answer/edit", methods)
	}
	for _, text := range texts {
		if strings.Contains(text, "inválido") {
			t.Fatalf("retry emitted false invalid message: %q", text)
		}
	}
}
