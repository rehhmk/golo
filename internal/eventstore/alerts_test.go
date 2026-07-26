package eventstore

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/enzotriches/golo/internal/scenario"
	"github.com/enzotriches/golo/internal/signals"
)

func TestStrategyArmingRequiresQualification(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "alerts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	def := scenario.DefaultDefinition()
	def.ID, def.Name, def.Version = "s", "test", 1
	def.Conditions = []scenario.Condition{{Field: scenario.FieldMinute, Operator: scenario.OpGreaterOrEqual, Value: 70}}
	row := signals.StoredStrategy{Definition: def, Report: scenario.QualificationReport{Qualified: false}}
	if err := store.SaveStrategy(row); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStrategyArmed("s", 1, true); err == nil {
		t.Fatal("unqualified strategy was armed")
	}
	row.Report.Qualified = true
	if err := store.SaveStrategy(row); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStrategyArmed("s", 1, true); err != nil {
		t.Fatalf("qualified strategy did not arm: %v", err)
	}
	row.Definition.Version = 2
	if err := store.SaveStrategy(row); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStrategyArmed("s", 2, true); err != nil {
		t.Fatal(err)
	}
	armed, err := store.ListArmedStrategies()
	if err != nil || len(armed) != 1 || armed[0].Definition.Version != 2 {
		t.Fatalf("expected only newest armed version, got %+v err=%v", armed, err)
	}
}

func TestInvitationIsOneTimeAndSetsAccessExpiry(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "invite.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	access := time.Now().Add(30 * 24 * time.Hour).Round(time.Second)
	inv := signals.Invitation{Code: "ONLYONCE", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), AccessUntil: access}
	if err := store.SaveInvitation(inv); err != nil {
		t.Fatal(err)
	}
	sub := signals.Subscriber{
		ID: "u1", TelegramChatID: 123, DisplayName: "Tester", Active: true,
		AdultConfirmed: true, TermsVersion: "v1", CreatedAt: time.Now(),
	}
	if err := store.RedeemInvitation(inv.Code, sub); err != nil {
		t.Fatal(err)
	}
	if err := store.RedeemInvitation(inv.Code, sub); err == nil {
		t.Fatal("invitation was redeemed twice")
	}
	active, err := store.ListActiveSubscribers()
	if err != nil || len(active) != 1 {
		t.Fatalf("active subscribers=%d err=%v", len(active), err)
	}
	if !active[0].ExpiresAt.Equal(access) {
		t.Fatalf("access expiry=%v want %v", active[0].ExpiresAt, access)
	}
}
