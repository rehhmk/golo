package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/enzotriches/golo/internal/domain"
)

// TrackRecord is a single match's own recent-call accuracy for the
// "goal in the next 10 minutes" horizon — shown on the live match card as a
// minimal "how right have we been" indicator. ResolvedCount == 0 means
// there isn't enough resolved history yet to show a meaningful percentage.
type TrackRecord struct {
	AccuracyPct   float64 `json:"accuracyPct"`
	ResolvedCount int     `json:"resolvedCount"`
}

type MatchUpdate struct {
	State       domain.MatchState `json:"state"`
	Prediction  domain.Prediction `json:"prediction"`
	TrackRecord TrackRecord       `json:"trackRecord"`
	Timestamp   time.Time         `json:"timestamp"`
}

type Publisher struct {
	mu           sync.RWMutex
	subscribers  map[chan MatchUpdate]bool
	firebaseURL  string
	firebaseAuth string
	httpClient   *http.Client
}

func NewPublisher(firebaseURL string, firebaseAuth string) *Publisher {
	return &Publisher{
		subscribers:  make(map[chan MatchUpdate]bool),
		firebaseURL:  firebaseURL,
		firebaseAuth: firebaseAuth,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (p *Publisher) Subscribe() chan MatchUpdate {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch := make(chan MatchUpdate, 100)
	p.subscribers[ch] = true
	return ch
}

func (p *Publisher) Unsubscribe(ch chan MatchUpdate) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.subscribers[ch]; ok {
		delete(p.subscribers, ch)
		close(ch)
	}
}

func (p *Publisher) Publish(ctx context.Context, state domain.MatchState, pred domain.Prediction, tr TrackRecord) error {
	update := MatchUpdate{
		State:       state,
		Prediction:  pred,
		TrackRecord: tr,
		Timestamp:   time.Now(),
	}

	// 1. Notify in-memory subscribers (SSE / WebSocket)
	p.mu.RLock()
	for ch := range p.subscribers {
		select {
		case ch <- update:
		default:
			// Non-blocking drop if channel is full
		}
	}
	p.mu.RUnlock()

	// 2. Sync to Firebase Realtime DB if configured
	if p.firebaseURL != "" {
		go p.syncToFirebase(ctx, state.MatchID, update)
	}

	return nil
}

const firebaseSyncMaxAttempts = 3

// syncToFirebase PUTs the update to the Realtime Database REST API
// (idempotent write, per blueprint §9.8), retrying transient failures with
// a short backoff. All attempts exhausted or a non-2xx response are logged
// once — this previously failed completely silently, which made a broken
// Firebase integration indistinguishable from a working one.
func (p *Publisher) syncToFirebase(ctx context.Context, matchID string, update MatchUpdate) {
	url := fmt.Sprintf("%s/matches/%s.json", p.firebaseURL, matchID)
	if p.firebaseAuth != "" {
		url += "?auth=" + p.firebaseAuth
	}

	data, err := json.Marshal(update)
	if err != nil {
		log.Printf("publisher: failed to marshal update for match %s: %v", matchID, err)
		return
	}

	var lastErr error
	for attempt := 1; attempt <= firebaseSyncMaxAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(attempt-1) * 200 * time.Millisecond):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
			} else {
				lastErr = nil
			}
		}()

		if lastErr == nil {
			return
		}
	}

	log.Printf("publisher: Firebase sync failed for match %s after %d attempts: %v", matchID, firebaseSyncMaxAttempts, lastErr)
}
