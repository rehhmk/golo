package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/enzotriches/golo/internal/adminauth"
	"github.com/enzotriches/golo/internal/evaluation"
	"github.com/enzotriches/golo/internal/eventstore"
	"github.com/enzotriches/golo/internal/publisher"
	"github.com/enzotriches/golo/internal/signals"
	"github.com/enzotriches/golo/internal/telegram"
)

type ReplayController interface {
	SetSpeed(speed string)
	SetPaused(paused bool)
	Reset()
}

type Server struct {
	store      *eventstore.SQLiteStore
	pub        *publisher.Publisher
	replayCtrl ReplayController
	mu         sync.RWMutex
	liveCache  map[string]publisher.MatchUpdate

	// modelVersion scopes /api/metrics to the model actually running.
	modelVersion   string
	adminAuth      *adminauth.Service
	telegram       *telegram.Service
	signalEngine   *signals.Engine
	datasetPath    string
	allowedOrigin  string
	providerHealth func() any
	modelContract  signals.ModelContract
	loginMu        sync.Mutex
	loginAttempts  map[string][]time.Time
}

// NewServer builds the HTTP API. modelVersion identifies the model currently
// loaded, so /api/metrics can report that model's own accuracy rather than a
// blend of every version the database has ever seen. Empty means "evaluate
// everything", which is only appropriate when no model is loaded.
func NewServer(store *eventstore.SQLiteStore, pub *publisher.Publisher, replayCtrl ReplayController, modelVersion string) *Server {
	return NewServerWithAdmin(store, pub, replayCtrl, modelVersion, AdminDependencies{})
}

type AdminDependencies struct {
	Auth           *adminauth.Service
	Telegram       *telegram.Service
	SignalEngine   *signals.Engine
	DatasetPath    string
	AllowedOrigin  string
	ProviderHealth func() any
	ModelContract  signals.ModelContract
}

func NewServerWithAdmin(store *eventstore.SQLiteStore, pub *publisher.Publisher, replayCtrl ReplayController, modelVersion string, deps AdminDependencies) *Server {
	srv := &Server{
		store:        store,
		pub:          pub,
		replayCtrl:   replayCtrl,
		liveCache:    make(map[string]publisher.MatchUpdate),
		modelVersion: modelVersion,
		adminAuth:    deps.Auth, telegram: deps.Telegram, signalEngine: deps.SignalEngine,
		datasetPath: deps.DatasetPath, allowedOrigin: deps.AllowedOrigin,
		providerHealth: deps.ProviderHealth, modelContract: deps.ModelContract,
		loginAttempts: make(map[string][]time.Time),
	}

	// Listen to publisher updates to maintain an in-memory cache of live matches
	subCh := pub.Subscribe()
	go func() {
		for update := range subCh {
			srv.mu.Lock()
			srv.liveCache[update.State.MatchID] = update
			srv.mu.Unlock()
		}
	}()

	return srv
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/api/matches", s.handleMatches)
	mux.HandleFunc("/api/matches/", s.handleMatchDetail)
	mux.HandleFunc("/api/replay/control", s.handleReplayControl)
	mux.HandleFunc("/api/metrics", s.handleMetrics)
	mux.HandleFunc("/api/scenarios/backtest", s.handleScenarioBacktest)
	mux.HandleFunc("/api/scenarios/dataset", s.handleScenarioDataset)
	mux.HandleFunc("/api/admin/login", s.handleAdminLogin)
	mux.Handle("/api/admin/", s.requireAdmin(http.HandlerFunc(s.handleAdmin)))
	mux.HandleFunc("/api/telegram/webhook", s.handleTelegramWebhook)

	return s.corsMiddleware(mux)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := s.allowedOrigin
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"service": "golo-core",
	})
}

func (s *Server) handleMatches(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	matches := make([]publisher.MatchUpdate, 0, len(s.liveCache))
	for _, m := range s.liveCache {
		matches = append(matches, m)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(matches)
}

func (s *Server) handleMatchDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/matches/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Match ID required", http.StatusBadRequest)
		return
	}

	matchID := parts[0]

	// Handle SSE stream: GET /api/matches/:id/stream
	if len(parts) > 1 && parts[1] == "stream" {
		s.handleMatchStream(w, r, matchID)
		return
	}

	// Handle events endpoint: GET /api/matches/:id/events
	if len(parts) > 1 && parts[1] == "events" {
		events, err := s.store.GetMatchEvents(matchID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
		return
	}

	// Default: return match state & prediction history
	state, err := s.store.GetLatestMatchState(matchID)
	if err != nil {
		// Fallback to cache if present
		s.mu.RLock()
		cached, found := s.liveCache[matchID]
		s.mu.RUnlock()
		if found {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cached)
			return
		}
		http.Error(w, "Match state not found: "+err.Error(), http.StatusNotFound)
		return
	}

	preds, _ := s.store.GetMatchPredictions(matchID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"state":       state,
		"predictions": preds,
	})
}

func (s *Server) handleMatchStream(w http.ResponseWriter, r *http.Request, matchID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	subCh := s.pub.Subscribe()
	defer s.pub.Unsubscribe(subCh)

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case update, ok := <-subCh:
			if !ok {
				return
			}
			if update.State.MatchID == matchID || matchID == "all" {
				data, _ := json.Marshal(update)
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				flusher.Flush()
			}
		}
	}
}

func (s *Server) handleReplayControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Action string `json:"action"` // "play", "pause", "speed", "reset"
		Speed  string `json:"speed"`  // "1x", "10x", "max"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if s.replayCtrl != nil {
		switch req.Action {
		case "play":
			s.replayCtrl.SetPaused(false)
		case "pause":
			s.replayCtrl.SetPaused(true)
		case "speed":
			s.replayCtrl.SetSpeed(req.Speed)
		case "reset":
			s.replayCtrl.Reset()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"action": req.Action,
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	predictions, err := s.store.GetPredictionsForModel(s.modelVersion)
	if err != nil {
		http.Error(w, "Failed to load predictions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	outcomes, err := s.store.GetMatchOutcomes()
	if err != nil {
		http.Error(w, "Failed to load match outcomes: "+err.Error(), http.StatusInternalServerError)
		return
	}

	report := evaluation.Evaluate(predictions, outcomes)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.Handler())
}

var _ = context.Background
