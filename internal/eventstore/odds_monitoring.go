package eventstore

import (
	"database/sql"
	"encoding/json"

	"github.com/enzotriches/golo/internal/odds"
)

func (s *SQLiteStore) SaveProviderObservation(observation odds.ProviderObservation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO odds_provider_observations
		(provider, observed_at, accepted_events, accepted_quotes, payload_json)
		VALUES (?, ?, ?, ?, ?)`,
		observation.Provider, observation.ObservedAt, observation.AcceptedEvents,
		observation.AcceptedQuotes, string(payload))
	return err
}

func (s *SQLiteStore) SaveProviderComparison(comparison odds.ProviderComparison) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := json.Marshal(comparison)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO odds_provider_comparisons
		(observed_at, primary_provider, shadow_provider, matched_events, comparable_quotes, payload_json)
		VALUES (?, ?, ?, ?, ?, ?)`,
		comparison.ObservedAt, comparison.PrimaryProvider, comparison.ShadowProvider,
		comparison.MatchedEvents, comparison.ComparableQuotes, string(payload))
	return err
}

func (s *SQLiteStore) ListProviderObservations(limit int) ([]odds.ProviderObservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(`SELECT payload_json FROM odds_provider_observations ORDER BY observed_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProviderObservations(rows)
}

func (s *SQLiteStore) ListProviderComparisons(limit int) ([]odds.ProviderComparison, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(`SELECT payload_json FROM odds_provider_comparisons ORDER BY observed_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]odds.ProviderComparison, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var comparison odds.ProviderComparison
		if err := json.Unmarshal([]byte(payload), &comparison); err != nil {
			return nil, err
		}
		out = append(out, comparison)
	}
	return out, rows.Err()
}

func scanProviderObservations(rows *sql.Rows) ([]odds.ProviderObservation, error) {
	out := make([]odds.ProviderObservation, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var observation odds.ProviderObservation
		if err := json.Unmarshal([]byte(payload), &observation); err != nil {
			return nil, err
		}
		out = append(out, observation)
	}
	return out, rows.Err()
}
