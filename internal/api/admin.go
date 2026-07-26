package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/enzotriches/golo/internal/scenario"
	"github.com/enzotriches/golo/internal/signals"
	"github.com/enzotriches/golo/internal/telegram"
)

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	if !s.allowLogin(ip) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return
	}
	var request struct {
		Password string `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if s.adminAuth == nil {
		http.Error(w, "admin authentication is not configured", http.StatusServiceUnavailable)
		return
	}
	token, err := s.adminAuth.Login(request.Password)
	if err != nil {
		s.recordFailedLogin(ip)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	s.clearLoginAttempts(ip)
	writeJSON(w, map[string]any{"token": token, "expiresInSeconds": 8 * 60 * 60})
}

func (s *Server) allowLogin(ip string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	cutoff := time.Now().Add(-15 * time.Minute)
	var recent []time.Time
	for _, attempt := range s.loginAttempts[ip] {
		if attempt.After(cutoff) {
			recent = append(recent, attempt)
		}
	}
	if len(recent) >= 5 {
		s.loginAttempts[ip] = recent
		return false
	}
	s.loginAttempts[ip] = recent
	return true
}

func (s *Server) recordFailedLogin(ip string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	s.loginAttempts[ip] = append(s.loginAttempts[ip], time.Now())
}

func (s *Server) clearLoginAttempts(ip string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	delete(s.loginAttempts, ip)
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if s.adminAuth == nil || !s.adminAuth.Verify(value) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/")
	switch {
	case path == "strategies/backtest" && r.Method == http.MethodPost:
		s.handleStrategyBacktest(w, r, false)
	case path == "strategies" && r.Method == http.MethodPost:
		s.handleStrategyBacktest(w, r, true)
	case path == "strategies" && r.Method == http.MethodGet:
		rows, err := s.store.ListStrategies()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, rows)
	case strings.HasPrefix(path, "strategies/") && strings.HasSuffix(path, "/arm") && r.Method == http.MethodPost:
		s.handleStrategyArm(w, r, path)
	case path == "signals" && r.Method == http.MethodGet:
		rows, err := s.store.ListSignals(300)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, rows)
	case path == "performance" && r.Method == http.MethodGet:
		rows, err := s.store.ListSignals(1000)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, signals.ComputePerformance(rows))
	case path == "provider-health" && r.Method == http.MethodGet:
		var provider any = map[string]any{"provider": "odds-api.io", "healthy": false, "message": "not configured"}
		if s.providerHealth != nil {
			provider = s.providerHealth()
		}
		var engine any = map[string]any{"message": "signal engine not configured"}
		if s.signalEngine != nil {
			engine = s.signalEngine.Health()
		}
		writeJSON(w, map[string]any{"odds": provider, "modelAndDelivery": engine})
	case path == "settings" && r.Method == http.MethodGet:
		settings, err := s.store.LoadSignalSettings()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, settings)
	case path == "settings" && (r.Method == http.MethodPut || r.Method == http.MethodPost):
		var settings signals.Settings
		if json.NewDecoder(r.Body).Decode(&settings) != nil {
			http.Error(w, "invalid settings", 400)
			return
		}
		if err := settings.Validate(); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := s.store.SaveSignalSettings(settings); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if s.signalEngine != nil {
			s.signalEngine.UpdateSettings(settings)
		}
		writeJSON(w, settings)
	case path == "invitations" && r.Method == http.MethodGet:
		rows, err := s.store.ListInvitations()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, rows)
	case path == "invitations" && r.Method == http.MethodPost:
		s.handleInvitationCreate(w, r)
	case path == "subscribers" && r.Method == http.MethodGet:
		rows, err := s.store.ListSubscribers()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, rows)
	case strings.HasPrefix(path, "subscribers/") && r.Method == http.MethodPut:
		id := strings.TrimPrefix(path, "subscribers/")
		var request struct {
			Active    bool      `json:"active"`
			ExpiresAt time.Time `json:"expiresAt"`
		}
		if id == "" || json.NewDecoder(r.Body).Decode(&request) != nil || request.ExpiresAt.IsZero() {
			http.Error(w, "invalid subscriber update", 400)
			return
		}
		if err := s.store.UpdateSubscriber(id, request.Active, request.ExpiresAt); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]any{"id": id, "active": request.Active, "expiresAt": request.ExpiresAt})
	case path == "telegram/test" && r.Method == http.MethodPost:
		s.handleTelegramTest(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleStrategyBacktest(w http.ResponseWriter, r *http.Request, persist bool) {
	if s.datasetPath == "" {
		http.Error(w, "historical dataset is not configured", http.StatusServiceUnavailable)
		return
	}
	var definition scenario.StrategyDefinition
	if json.NewDecoder(r.Body).Decode(&definition) != nil {
		http.Error(w, "invalid strategy", 400)
		return
	}
	if definition.ID == "" {
		definition.ID = randomCode(12)
	}
	if definition.Version <= 0 {
		definition.Version = 1
	}
	matches, err := scenario.LoadTimelines(s.datasetPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	report, err := scenario.BacktestDefinition(definition, matches)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	stored := signals.StoredStrategy{
		Definition: definition, Report: report, Armed: false,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if persist {
		// Editing is immutable: a changed definition becomes a new version.
		existing, err := s.store.ListStrategies()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		maxVersion := 0
		for _, row := range existing {
			if row.Definition.ID == definition.ID && row.Definition.Version > maxVersion {
				maxVersion = row.Definition.Version
			}
		}
		if maxVersion >= definition.Version {
			stored.Definition.Version = maxVersion + 1
		}
		if err := s.store.SaveStrategy(stored); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	writeJSON(w, stored)
}

func (s *Server) handleStrategyArm(w http.ResponseWriter, r *http.Request, path string) {
	id := strings.TrimSuffix(strings.TrimPrefix(path, "strategies/"), "/arm")
	var request struct {
		Version int  `json:"version"`
		Armed   bool `json:"armed"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil || request.Version <= 0 {
		http.Error(w, "invalid request", 400)
		return
	}
	if err := s.store.SetStrategyArmed(id, request.Version, request.Armed); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"id": id, "version": request.Version, "armed": request.Armed})
}

func (s *Server) handleInvitationCreate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		InviteHours int `json:"inviteHours"`
		AccessDays  int `json:"accessDays"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if request.InviteHours <= 0 {
		request.InviteHours = 24
	}
	if request.AccessDays <= 0 {
		request.AccessDays = 30
	}
	invitation := signals.Invitation{
		Code: randomCode(16), CreatedAt: time.Now(),
		ExpiresAt:   time.Now().Add(time.Duration(request.InviteHours) * time.Hour),
		AccessUntil: time.Now().Add(time.Duration(request.AccessDays) * 24 * time.Hour),
	}
	if err := s.store.SaveInvitation(invitation); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, invitation)
}

func (s *Server) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || s.telegram == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	var update telegram.Update
	if json.NewDecoder(r.Body).Decode(&update) != nil {
		http.Error(w, "invalid update", 400)
		return
	}
	if err := s.telegram.HandleWebhook(r.Context(), r.Header.Get("X-Telegram-Bot-Api-Secret-Token"), update); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleTelegramTest(w http.ResponseWriter, r *http.Request) {
	if s.telegram == nil {
		http.Error(w, "telegram is not configured", 503)
		return
	}
	var request struct {
		ChatID string `json:"chatId"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	chatID, err := strconv.ParseInt(request.ChatID, 10, 64)
	if err != nil {
		http.Error(w, "invalid chat id", 400)
		return
	}
	if err := s.telegram.SendTest(r.Context(), chatID); err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	writeJSON(w, map[string]string{"status": "sent"})
}

func randomCode(bytesCount int) string {
	raw := make([]byte, bytesCount)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
