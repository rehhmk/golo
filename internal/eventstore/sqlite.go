package eventstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/enzotriches/golo/internal/domain"
)

type SQLiteStore struct {
	db *sql.DB
	mu sync.Mutex
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Configure WAL mode and performance pragmas
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to set pragma (%s): %w", pragma, err)
		}
	}

	store := &SQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return store, nil
}

func (s *SQLiteStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS matches (
		id TEXT PRIMARY KEY,
		provider TEXT NOT NULL,
		provider_match_id TEXT NOT NULL,
		competition_id TEXT NOT NULL,
		home_team_id TEXT NOT NULL,
		away_team_id TEXT NOT NULL,
		status TEXT NOT NULL,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS events (
		event_id TEXT PRIMARY KEY,
		match_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		event_type TEXT NOT NULL,
		match_second INTEGER NOT NULL,
		period INTEGER NOT NULL,
		received_at TIMESTAMP NOT NULL,
		payload_json TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_events_match_sec ON events(match_id, match_second);

	CREATE TABLE IF NOT EXISTS match_states (
		match_id TEXT PRIMARY KEY,
		clock_seconds INTEGER NOT NULL,
		score_home INTEGER NOT NULL,
		score_away INTEGER NOT NULL,
		state_version INTEGER NOT NULL,
		payload_json TEXT NOT NULL,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS snapshots (
		snapshot_id TEXT PRIMARY KEY,
		match_id TEXT NOT NULL,
		match_second INTEGER NOT NULL,
		feature_version TEXT NOT NULL,
		features_json TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_snapshots_match_sec ON snapshots(match_id, match_second);

	CREATE TABLE IF NOT EXISTS predictions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		match_id TEXT NOT NULL,
		as_of_second INTEGER NOT NULL,
		prob_5m REAL NOT NULL,
		prob_10m REAL NOT NULL,
		prob_ft REAL NOT NULL,
		data_quality REAL NOT NULL,
		confidence_band TEXT NOT NULL,
		status TEXT NOT NULL,
		model_version TEXT NOT NULL,
		prediction_sequence INTEGER NOT NULL,
		feature_version TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_predictions_match ON predictions(match_id, as_of_second);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// feature_version was added after the initial schema; back-fill it on
	// databases created before this column existed. SQLite has no
	// "ADD COLUMN IF NOT EXISTS", so ignore the one expected error.
	if _, err := s.db.Exec(`ALTER TABLE predictions ADD COLUMN feature_version TEXT NOT NULL DEFAULT '';`); err != nil {
		if !isDuplicateColumnErr(err) {
			return err
		}
	}

	return nil
}

func isDuplicateColumnErr(err error) bool {
	return strings.Contains(err.Error(), "duplicate column name")
}

func (s *SQLiteStore) SaveMatch(match domain.Match) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	INSERT INTO matches (id, provider, provider_match_id, competition_id, home_team_id, away_team_id, status, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		status = excluded.status,
		updated_at = CURRENT_TIMESTAMP;
	`
	_, err := s.db.Exec(query, match.ID, match.Provider, match.ProviderMatchID, match.CompetitionID, match.HomeTeamID, match.AwayTeamID, string(match.Status))
	return err
}

func (s *SQLiteStore) SaveEvents(events []domain.MatchEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
	INSERT OR IGNORE INTO events (event_id, match_id, provider, event_type, match_second, period, received_at, payload_json)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ev := range events {
		bytes, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(ev.EventID, ev.MatchID, ev.Provider, string(ev.EventType), ev.MatchSecond, ev.Period, ev.ReceivedAt, string(bytes)); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) SaveMatchState(state domain.MatchState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	bytes, err := json.Marshal(state)
	if err != nil {
		return err
	}

	query := `
	INSERT INTO match_states (match_id, clock_seconds, score_home, score_away, state_version, payload_json, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(match_id) DO UPDATE SET
		clock_seconds = excluded.clock_seconds,
		score_home = excluded.score_home,
		score_away = excluded.score_away,
		state_version = excluded.state_version,
		payload_json = excluded.payload_json,
		updated_at = CURRENT_TIMESTAMP;
	`
	_, err = s.db.Exec(query, state.MatchID, state.ClockSeconds, state.Score.Home, state.Score.Away, state.StateVersion, string(bytes))
	return err
}

func (s *SQLiteStore) SaveSnapshot(snap domain.FeatureSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	INSERT OR IGNORE INTO snapshots (snapshot_id, match_id, match_second, feature_version, features_json, created_at)
	VALUES (?, ?, ?, ?, ?, ?);
	`
	_, err := s.db.Exec(query, snap.SnapshotID, snap.MatchID, snap.MatchSecond, snap.FeatureVersion, string(snap.Features), snap.CreatedAt)
	return err
}

func (s *SQLiteStore) SavePrediction(pred domain.Prediction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	INSERT INTO predictions (match_id, as_of_second, prob_5m, prob_10m, prob_ft, data_quality, confidence_band, status, model_version, prediction_sequence, feature_version, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	_, err := s.db.Exec(query,
		pred.MatchID,
		pred.AsOfMatchSecond,
		pred.Probabilities.GoalNext5m,
		pred.Probabilities.GoalNext10m,
		pred.Probabilities.GoalBeforeFullTime,
		pred.DataQuality,
		string(pred.ConfidenceBand),
		string(pred.Status),
		pred.ModelVersion,
		pred.PredictionSequence,
		pred.FeatureVersion,
		pred.CalculatedAt,
	)
	return err
}

func (s *SQLiteStore) GetLatestMatchState(matchID string) (domain.MatchState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `SELECT payload_json FROM match_states WHERE match_id = ?;`
	row := s.db.QueryRow(query, matchID)

	var payload string
	if err := row.Scan(&payload); err != nil {
		return domain.MatchState{}, err
	}

	var state domain.MatchState
	err := json.Unmarshal([]byte(payload), &state)
	return state, err
}

func (s *SQLiteStore) GetMatchPredictions(matchID string) ([]domain.Prediction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	SELECT match_id, as_of_second, prob_5m, prob_10m, prob_ft, data_quality, confidence_band, status, model_version, prediction_sequence, feature_version, created_at
	FROM predictions
	WHERE match_id = ?
	ORDER BY as_of_second ASC;
	`
	rows, err := s.db.Query(query, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var preds []domain.Prediction
	for rows.Next() {
		var p domain.Prediction
		var confBand, status string

		err := rows.Scan(
			&p.MatchID,
			&p.AsOfMatchSecond,
			&p.Probabilities.GoalNext5m,
			&p.Probabilities.GoalNext10m,
			&p.Probabilities.GoalBeforeFullTime,
			&p.DataQuality,
			&confBand,
			&status,
			&p.ModelVersion,
			&p.PredictionSequence,
			&p.FeatureVersion,
			&p.CalculatedAt,
		)
		if err != nil {
			return nil, err
		}
		p.ConfidenceBand = domain.ConfidenceBand(confBand)
		p.Status = domain.PredictionStatus(status)
		preds = append(preds, p)
	}

	return preds, nil
}

// GetAllPredictions returns every stored prediction across all matches,
// ordered by match and match-second, for aggregate evaluation.
func (s *SQLiteStore) GetAllPredictions() ([]domain.Prediction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	SELECT match_id, as_of_second, prob_5m, prob_10m, prob_ft, data_quality, confidence_band, status, model_version, prediction_sequence, feature_version, created_at
	FROM predictions
	ORDER BY match_id ASC, as_of_second ASC;
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var preds []domain.Prediction
	for rows.Next() {
		var p domain.Prediction
		var confBand, status string

		err := rows.Scan(
			&p.MatchID,
			&p.AsOfMatchSecond,
			&p.Probabilities.GoalNext5m,
			&p.Probabilities.GoalNext10m,
			&p.Probabilities.GoalBeforeFullTime,
			&p.DataQuality,
			&confBand,
			&status,
			&p.ModelVersion,
			&p.PredictionSequence,
			&p.FeatureVersion,
			&p.CalculatedAt,
		)
		if err != nil {
			return nil, err
		}
		p.ConfidenceBand = domain.ConfidenceBand(confBand)
		p.Status = domain.PredictionStatus(status)
		preds = append(preds, p)
	}

	return preds, nil
}

// GetMatchGoalAndEndSeconds scans every stored event once and returns, per
// match: the match-seconds at which a goal occurred (sorted ascending), and
// the highest match-second seen for that match (used as a proxy for "how far
// the match had progressed", to bound the goal-before-full-time horizon).
func (s *SQLiteStore) GetMatchGoalAndEndSeconds() (goalSeconds map[string][]int, endSeconds map[string]int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT match_id, match_second, event_type FROM events ORDER BY match_id ASC, match_second ASC;`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	goalSeconds = make(map[string][]int)
	endSeconds = make(map[string]int)

	for rows.Next() {
		var matchID, eventType string
		var matchSecond int
		if err := rows.Scan(&matchID, &matchSecond, &eventType); err != nil {
			return nil, nil, err
		}

		if matchSecond > endSeconds[matchID] {
			endSeconds[matchID] = matchSecond
		}

		et := domain.EventType(eventType)
		if et.IsGoalEvent() {
			goalSeconds[matchID] = append(goalSeconds[matchID], matchSecond)
		}
	}

	return goalSeconds, endSeconds, nil
}

// GetGoalAndEndSecondsForMatch is the single-match, indexed-lookup
// equivalent of GetMatchGoalAndEndSeconds — used on the ingestion hot path
// (once per poll tick per match) where scanning every event in the
// database would be wasteful.
func (s *SQLiteStore) GetGoalAndEndSecondsForMatch(matchID string) (goalSeconds []int, endSecond int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT match_second, event_type FROM events WHERE match_id = ? ORDER BY match_second ASC;`, matchID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var matchSecond int
		var eventType string
		if err := rows.Scan(&matchSecond, &eventType); err != nil {
			return nil, 0, err
		}

		if matchSecond > endSecond {
			endSecond = matchSecond
		}
		if domain.EventType(eventType).IsGoalEvent() {
			goalSeconds = append(goalSeconds, matchSecond)
		}
	}

	return goalSeconds, endSecond, nil
}

func (s *SQLiteStore) GetMatchEvents(matchID string) ([]domain.MatchEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `SELECT payload_json FROM events WHERE match_id = ? ORDER BY match_second ASC;`
	rows, err := s.db.Query(query, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.MatchEvent
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var ev domain.MatchEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}

	return events, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
