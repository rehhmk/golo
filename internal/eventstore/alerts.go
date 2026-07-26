package eventstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/enzotriches/golo/internal/signals"
)

func (s *SQLiteStore) SaveStrategy(strategy signals.StoredStrategy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	definition, err := json.Marshal(strategy.Definition)
	if err != nil {
		return err
	}
	report, err := json.Marshal(strategy.Report)
	if err != nil {
		return err
	}
	now := time.Now()
	if strategy.CreatedAt.IsZero() {
		strategy.CreatedAt = now
	}
	_, err = s.db.Exec(`
		INSERT INTO strategies (id, version, name, definition_json, report_json, armed, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id, version) DO UPDATE SET
			name=excluded.name, definition_json=excluded.definition_json,
			report_json=excluded.report_json, armed=excluded.armed, updated_at=excluded.updated_at;`,
		strategy.Definition.ID, strategy.Definition.Version, strategy.Definition.Name,
		string(definition), string(report), strategy.Armed, strategy.CreatedAt, now)
	return err
}

func (s *SQLiteStore) ListStrategies() ([]signals.StoredStrategy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return listStrategies(s.db, false)
}

func (s *SQLiteStore) ListArmedStrategies() ([]signals.StoredStrategy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return listStrategies(s.db, true)
}

type queryer interface {
	Query(string, ...any) (*sql.Rows, error)
}

func listStrategies(db queryer, armedOnly bool) ([]signals.StoredStrategy, error) {
	query := `SELECT definition_json, report_json, armed, created_at, updated_at FROM strategies`
	if armedOnly {
		query += ` WHERE armed=1`
	}
	query += ` ORDER BY updated_at DESC`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []signals.StoredStrategy
	for rows.Next() {
		var strategy signals.StoredStrategy
		var definition, report string
		if err := rows.Scan(&definition, &report, &strategy.Armed, &strategy.CreatedAt, &strategy.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(definition), &strategy.Definition); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(report), &strategy.Report); err != nil {
			return nil, err
		}
		out = append(out, strategy)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SetStrategyArmed(id string, version int, armed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if armed {
		var report string
		if err := tx.QueryRow(`SELECT report_json FROM strategies WHERE id=? AND version=?`, id, version).Scan(&report); err != nil {
			return err
		}
		var parsed struct {
			Qualified bool `json:"qualified"`
		}
		if err := json.Unmarshal([]byte(report), &parsed); err != nil {
			return err
		}
		if !parsed.Qualified {
			return fmt.Errorf("strategy has not passed qualification")
		}
		// Only one immutable version of a strategy may be live. Otherwise an
		// edit could send two alerts for the same market under two versions.
		if _, err := tx.Exec(`UPDATE strategies SET armed=0, updated_at=? WHERE id=?`, time.Now(), id); err != nil {
			return err
		}
	}
	result, err := tx.Exec(`UPDATE strategies SET armed=?, updated_at=? WHERE id=? AND version=?`, armed, time.Now(), id, version)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (s *SQLiteStore) SaveSignalDecision(decision signals.Decision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := json.Marshal(decision)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO signal_decisions (id, dedup_key, match_id, strategy_id, strategy_version, status, payload_json, created_at, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(dedup_key) DO UPDATE SET
			status=CASE WHEN signal_decisions.status='REJECTED' THEN excluded.status ELSE signal_decisions.status END,
			payload_json=CASE WHEN signal_decisions.status='REJECTED' THEN excluded.payload_json ELSE signal_decisions.payload_json END;`,
		decision.ID, decision.DedupKey, decision.MatchID, decision.StrategyID,
		decision.StrategyVersion, decision.Status, string(payload), decision.CreatedAt, decision.ResolvedAt)
	return err
}

func (s *SQLiteStore) GetSignalByDedupKey(key string) (signals.Decision, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return scanDecision(s.db.QueryRow(`SELECT payload_json FROM signal_decisions WHERE dedup_key=?`, key))
}

func (s *SQLiteStore) ListOpenSignalsForMatch(matchID string) ([]signals.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT payload_json FROM signal_decisions WHERE match_id=? AND status='QUALIFIED'`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDecisions(rows)
}

func (s *SQLiteStore) ListSignals(limit int) ([]signals.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(`SELECT payload_json FROM signal_decisions ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDecisions(rows)
}

func (s *SQLiteStore) UpdateSignalStatus(id string, status signals.Status, resolvedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var payload string
	if err := s.db.QueryRow(`SELECT payload_json FROM signal_decisions WHERE id=?`, id).Scan(&payload); err != nil {
		return err
	}
	var decision signals.Decision
	if err := json.Unmarshal([]byte(payload), &decision); err != nil {
		return err
	}
	decision.Status = status
	decision.ResolvedAt = &resolvedAt
	updated, err := json.Marshal(decision)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE signal_decisions SET status=?, payload_json=?, resolved_at=? WHERE id=?`,
		status, string(updated), resolvedAt, id)
	return err
}

func scanDecision(row *sql.Row) (signals.Decision, bool, error) {
	var payload string
	if err := row.Scan(&payload); err != nil {
		if err == sql.ErrNoRows {
			return signals.Decision{}, false, nil
		}
		return signals.Decision{}, false, err
	}
	var decision signals.Decision
	if err := json.Unmarshal([]byte(payload), &decision); err != nil {
		return signals.Decision{}, false, err
	}
	return decision, true, nil
}

func scanDecisions(rows *sql.Rows) ([]signals.Decision, error) {
	var out []signals.Decision
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var decision signals.Decision
		if err := json.Unmarshal([]byte(payload), &decision); err != nil {
			return nil, err
		}
		out = append(out, decision)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SaveInvitation(inv signals.Invitation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO invitations (code, expires_at, access_until, used_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		inv.Code, inv.ExpiresAt, inv.AccessUntil, inv.UsedAt, inv.CreatedAt)
	return err
}

func (s *SQLiteStore) ListInvitations() ([]signals.Invitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT code, expires_at, access_until, used_at, created_at FROM invitations ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []signals.Invitation
	for rows.Next() {
		var inv signals.Invitation
		var used sql.NullTime
		if err := rows.Scan(&inv.Code, &inv.ExpiresAt, &inv.AccessUntil, &used, &inv.CreatedAt); err != nil {
			return nil, err
		}
		if used.Valid {
			inv.UsedAt = &used.Time
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// RedeemInvitation atomically activates access and reports whether this call
// performed the redemption. Telegram may deliver the same callback more than
// once, so a retry from the same chat is a successful no-op. A different chat
// still cannot reuse the one-time code.
func (s *SQLiteStore) RedeemInvitation(code string, subscriber signals.Subscriber) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var expires, access time.Time
	var used sql.NullTime
	var usedBy sql.NullInt64
	if err := tx.QueryRow(`SELECT expires_at, access_until, used_at, used_by_chat_id FROM invitations WHERE code=?`, code).
		Scan(&expires, &access, &used, &usedBy); err != nil {
		return false, err
	}
	if used.Valid {
		if usedBy.Valid && usedBy.Int64 == subscriber.TelegramChatID {
			return false, nil
		}
		return false, fmt.Errorf("invitation is expired or already used")
	}
	if time.Now().After(expires) {
		return false, fmt.Errorf("invitation is expired or already used")
	}
	subscriber.ExpiresAt = access
	if _, err := tx.Exec(`INSERT INTO subscribers (id, telegram_chat_id, display_name, active, terms_version, adult_confirmed, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(telegram_chat_id) DO UPDATE SET display_name=excluded.display_name, active=1,
		terms_version=excluded.terms_version, adult_confirmed=excluded.adult_confirmed, expires_at=excluded.expires_at`,
		subscriber.ID, subscriber.TelegramChatID, subscriber.DisplayName, subscriber.Active,
		subscriber.TermsVersion, subscriber.AdultConfirmed, subscriber.ExpiresAt, subscriber.CreatedAt); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`UPDATE invitations SET used_at=?, used_by_chat_id=? WHERE code=?`,
		time.Now(), subscriber.TelegramChatID, code); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteStore) ListSubscribers() ([]signals.Subscriber, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, telegram_chat_id, display_name, active, terms_version, adult_confirmed, expires_at, created_at FROM subscribers ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []signals.Subscriber
	for rows.Next() {
		var sub signals.Subscriber
		if err := rows.Scan(&sub.ID, &sub.TelegramChatID, &sub.DisplayName, &sub.Active, &sub.TermsVersion, &sub.AdultConfirmed, &sub.ExpiresAt, &sub.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListActiveSubscribers() ([]signals.Subscriber, error) {
	all, err := s.ListSubscribers()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]signals.Subscriber, 0, len(all))
	for _, sub := range all {
		if sub.Active && sub.AdultConfirmed && sub.ExpiresAt.After(now) {
			out = append(out, sub)
		}
	}
	return out, nil
}

func (s *SQLiteStore) UpdateSubscriber(id string, active bool, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(`UPDATE subscribers SET active=?, expires_at=? WHERE id=?`, active, expiresAt, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SQLiteStore) SaveDelivery(delivery signals.Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO signal_deliveries
		(id, signal_id, subscriber_id, kind, status, attempts, error, message_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET status=excluded.status, attempts=excluded.attempts,
		error=excluded.error, message_id=excluded.message_id, updated_at=excluded.updated_at`,
		delivery.ID, delivery.SignalID, delivery.SubscriberID, delivery.Kind, delivery.Status,
		delivery.Attempts, delivery.Error, delivery.MessageID, delivery.CreatedAt, delivery.UpdatedAt)
	return err
}

func (s *SQLiteStore) LoadSignalSettings() (signals.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings := signals.DefaultSettings()
	var payload string
	if err := s.db.QueryRow(`SELECT value_json FROM app_settings WHERE key='signals'`).Scan(&payload); err != nil {
		if err == sql.ErrNoRows {
			return settings, nil
		}
		return settings, err
	}
	return settings, json.Unmarshal([]byte(payload), &settings)
}

func (s *SQLiteStore) SaveSignalSettings(settings signals.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO app_settings (key, value_json, updated_at) VALUES ('signals', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json, updated_at=excluded.updated_at`,
		string(payload), time.Now())
	return err
}
