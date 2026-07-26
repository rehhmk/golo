package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/enzotriches/golo/internal/adminauth"
	"github.com/enzotriches/golo/internal/eventstore"
	"github.com/enzotriches/golo/internal/publisher"
	"golang.org/x/crypto/bcrypt"
)

func TestAdminEndpointsRequireShortLivedBearerSession(t *testing.T) {
	store, err := eventstore.NewSQLiteStore(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	auth := adminauth.New(string(hash), "01234567890123456789012345678901", time.Hour)
	server := NewServerWithAdmin(store, publisher.NewPublisher("", ""), nil, "test", AdminDependencies{
		Auth: auth, AllowedOrigin: "https://golo.example",
		ProviderHealth: func() any { return map[string]any{"healthy": true} },
	})
	handler := server.Handler()

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/admin/provider-health", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unprotected admin endpoint returned %d", unauthorized.Code)
	}

	login := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewBufferString(`{"password":"secret"}`))
	request.Header.Set("Origin", "https://golo.example")
	handler.ServeHTTP(login, request)
	if login.Code != http.StatusOK || login.Header().Get("Access-Control-Allow-Origin") != "https://golo.example" {
		t.Fatalf("login=%d cors=%q body=%s", login.Code, login.Header().Get("Access-Control-Allow-Origin"), login.Body.String())
	}
	var session struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(login.Body.Bytes(), &session) != nil || session.Token == "" {
		t.Fatal("login did not return a token")
	}
	protected := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/provider-health", nil)
	request.Header.Set("Authorization", "Bearer "+session.Token)
	handler.ServeHTTP(protected, request)
	if protected.Code != http.StatusOK {
		t.Fatalf("authorized request returned %d: %s", protected.Code, protected.Body.String())
	}

	for _, path := range []string{"strategies", "signals", "invitations", "subscribers"} {
		response := httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodGet, "/api/admin/"+path, nil)
		request.Header.Set("Authorization", "Bearer "+session.Token)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", path, response.Code, response.Body.String())
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil || rows == nil {
			t.Fatalf("%s must return a JSON array, body=%q err=%v", path, response.Body.String(), err)
		}
	}
}
