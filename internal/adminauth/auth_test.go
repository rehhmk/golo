package adminauth

import (
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestLoginVerifyTamperAndExpiry(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct horse"), bcrypt.MinCost)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	service := New(string(hash), "01234567890123456789012345678901", time.Hour)
	service.now = func() time.Time { return now }

	token, err := service.Login("correct horse")
	if err != nil || !service.Verify(token) {
		t.Fatalf("valid login failed: %v", err)
	}
	if service.Verify(token + "x") {
		t.Fatal("tampered token verified")
	}
	if _, err := service.Login("wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
	service.now = func() time.Time { return now.Add(2 * time.Hour) }
	if service.Verify(token) {
		t.Fatal("expired token verified")
	}
}
