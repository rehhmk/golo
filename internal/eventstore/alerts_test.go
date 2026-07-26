package eventstore

import (
	"encoding/json"
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
	if err := store.SetStrategyArmed("s", 1, true, "model-sha"); err == nil {
		t.Fatal("unqualified strategy was armed")
	}
	row.Report.Qualified = true
	row.Report.ValidationQualified = true
	row.Report.ModelValidationQualified = true
	if err := store.SaveStrategy(row); err != nil {
		t.Fatal(err)
	}
	contract, _ := json.Marshal(signals.LockedTestContract{Model: signals.ModelContract{ModelSHA256: "model-sha"}})
	now := time.Now()
	if _, err := store.db.Exec(`INSERT INTO strategy_locked_tests
		(id, strategy_id, strategy_version, state, contract_json, contract_sha256,
		 validation_report_json, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"lock-1", "s", 1, signals.LockedStateRevealedPass, string(contract), "contract",
		`{"qualified":true}`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStrategyArmed("s", 1, true, "model-sha"); err != nil {
		t.Fatalf("qualified strategy did not arm: %v", err)
	}
	row.Definition.Version = 2
	if err := store.SaveStrategy(row); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO strategy_locked_tests
		(id, strategy_id, strategy_version, state, contract_json, contract_sha256,
		 validation_report_json, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"lock-2", "s", 2, signals.LockedStateRevealedPass, string(contract), "contract",
		`{"qualified":true}`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStrategyArmed("s", 2, true, "model-sha"); err != nil {
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
	redeemed, err := store.RedeemInvitation(inv.Code, sub)
	if err != nil {
		t.Fatal(err)
	}
	if !redeemed {
		t.Fatal("first redemption was reported as a retry")
	}
	redeemed, err = store.RedeemInvitation(inv.Code, sub)
	if err != nil || redeemed {
		t.Fatalf("same-chat retry should be an idempotent no-op: redeemed=%v err=%v", redeemed, err)
	}
	other := sub
	other.ID, other.TelegramChatID = "u2", 456
	if _, err := store.RedeemInvitation(inv.Code, other); err == nil {
		t.Fatal("invitation was reused by a different chat")
	}
	active, err := store.ListActiveSubscribers()
	if err != nil || len(active) != 1 {
		t.Fatalf("active subscribers=%d err=%v", len(active), err)
	}
	if !active[0].ExpiresAt.Equal(access) {
		t.Fatalf("access expiry=%v want %v", active[0].ExpiresAt, access)
	}
}

func TestMigrationDisarmsLegacyStrategyWithoutRevealedLockedTest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	definition := scenario.DefaultDefinition()
	definition.ID, definition.Name = "legacy", "Legacy armed"
	definition.Conditions = []scenario.Condition{{
		Field: scenario.FieldMinute, Operator: scenario.OpGreaterOrEqual, Value: 70,
	}}
	if err := store.SaveStrategy(signals.StoredStrategy{
		Definition: definition, Armed: true,
		Report: scenario.QualificationReport{Qualified: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	strategies, err := reopened.ListStrategies()
	if err != nil {
		t.Fatal(err)
	}
	if len(strategies) != 1 || strategies[0].Armed {
		t.Fatalf("legacy strategy remained armed after migration: %+v", strategies)
	}
}
