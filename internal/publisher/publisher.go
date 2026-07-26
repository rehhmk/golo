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

	tokenMu      sync.Mutex
	cachedToken  string
	tokenExpires time.Time
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
	baseURL := fmt.Sprintf("%s/matches/%s.json", p.firebaseURL, matchID)

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

		reqURL, err := p.authenticatedURL(ctx, baseURL)
		if err != nil {
			lastErr = err
			continue
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(data))
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

// authenticatedURL appends an OAuth2 access token to the Realtime Database
// REST URL. If FIREBASE_AUTH was configured explicitly, that value is used
// as-is. Otherwise it fetches one from the GCE metadata server (the VM's own
// service account, granted roles/firebasedatabase.admin at deploy time) —
// legacy Firebase "database secrets" no longer work for RTDB instances
// created after Google's mid-2024 deprecation, so a real OAuth2 token is
// required, not just an opaque string.
func (p *Publisher) authenticatedURL(ctx context.Context, baseURL string) (string, error) {
	if p.firebaseAuth != "" {
		return baseURL + "?access_token=" + p.firebaseAuth, nil
	}

	token, err := p.metadataAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("no FIREBASE_AUTH set and metadata token fetch failed: %w", err)
	}
	return baseURL + "?access_token=" + token, nil
}

const metadataTokenURL = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"

type metadataTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// metadataAccessToken fetches (and caches) an OAuth2 access token for the
// GCE instance's attached service account. Only works when actually running
// on a GCE VM — the metadata server isn't reachable elsewhere, which is
// expected (local/dev runs should set FIREBASE_AUTH or leave Firebase sync
// disabled entirely by leaving FIREBASE_DATABASE_URL blank).
func (p *Publisher) metadataAccessToken(ctx context.Context) (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()

	if p.cachedToken != "" && time.Now().Before(p.tokenExpires) {
		return p.cachedToken, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataTokenURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata server returned status %d", resp.StatusCode)
	}

	var tok metadataTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("metadata server returned empty access token")
	}

	// Refresh a bit early to avoid racing the real expiry.
	p.cachedToken = tok.AccessToken
	p.tokenExpires = time.Now().Add(time.Duration(tok.ExpiresIn-60) * time.Second)

	return p.cachedToken, nil
}
