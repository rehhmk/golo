package adminauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	passwordHash []byte
	sessionKey   []byte
	ttl          time.Duration
	now          func() time.Time
}

type claims struct {
	Subject string `json:"sub"`
	Expires int64  `json:"exp"`
	Nonce   string `json:"nonce"`
}

func New(passwordHash, sessionSecret string, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 8 * time.Hour
	}
	return &Service{
		passwordHash: []byte(passwordHash),
		sessionKey:   []byte(sessionSecret),
		ttl:          ttl,
		now:          time.Now,
	}
}

func (s *Service) Configured() bool {
	return len(s.passwordHash) > 0 && len(s.sessionKey) >= 32
}

func (s *Service) Login(password string) (string, error) {
	if !s.Configured() {
		return "", errors.New("admin authentication is not configured")
	}
	if err := bcrypt.CompareHashAndPassword(s.passwordHash, []byte(password)); err != nil {
		return "", errors.New("invalid credentials")
	}
	var nonce [12]byte
	_, _ = rand.Read(nonce[:])
	payload, _ := json.Marshal(claims{
		Subject: "admin", Expires: s.now().Add(s.ttl).Unix(), Nonce: hex.EncodeToString(nonce[:]),
	})
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + s.sign(encoded), nil
}

func (s *Service) Verify(token string) bool {
	if !s.Configured() {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(s.sign(parts[0]))) {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var parsed claims
	if json.Unmarshal(payload, &parsed) != nil {
		return false
	}
	return parsed.Subject == "admin" && parsed.Expires > s.now().Unix()
}

func (s *Service) sign(value string) string {
	mac := hmac.New(sha256.New, s.sessionKey)
	_, _ = mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
