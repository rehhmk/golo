package eventstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/enzotriches/golo/internal/domain"
	"github.com/enzotriches/golo/internal/evaluation"
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
		expected_goals_remaining REAL NOT NULL DEFAULT 0,
		prob_two_plus REAL NOT NULL DEFAULT 0,
		contributions_json TEXT NOT NULL DEFAULT '[]',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_predictions_match ON predictions(match_id, as_of_second);

	CREATE TABLE IF NOT EXISTS strategies (
		id TEXT NOT NULL,
		version INTEGER NOT NULL,
		name TEXT NOT NULL,
		definition_json TEXT NOT NULL,
		report_json TEXT NOT NULL,
		armed INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		PRIMARY KEY (id, version)
	);
	CREATE INDEX IF NOT EXISTS idx_strategies_armed ON strategies(armed, updated_at);

	CREATE TABLE IF NOT EXISTS strategy_locked_tests (
		id TEXT PRIMARY KEY,
		strategy_id TEXT NOT NULL,
		strategy_version INTEGER NOT NULL,
		state TEXT NOT NULL,
		contract_json TEXT NOT NULL,
		contract_sha256 TEXT NOT NULL,
		validation_report_json TEXT NOT NULL,
		report_json TEXT,
		started_at TIMESTAMP NOT NULL,
		ready_at TIMESTAMP,
		revealed_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		UNIQUE(strategy_id, strategy_version),
		FOREIGN KEY(strategy_id, strategy_version) REFERENCES strategies(id, version)
	);
	CREATE INDEX IF NOT EXISTS idx_locked_tests_state ON strategy_locked_tests(state, started_at);

	CREATE TABLE IF NOT EXISTS strategy_locked_occurrences (
		id TEXT PRIMARY KEY,
		test_id TEXT NOT NULL,
		match_id TEXT NOT NULL,
		rule_cohort INTEGER NOT NULL,
		signal_cohort INTEGER NOT NULL,
		signal_eligible INTEGER NOT NULL,
		status TEXT NOT NULL,
		payload_json TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		resolved_at TIMESTAMP,
		UNIQUE(test_id, match_id),
		FOREIGN KEY(test_id) REFERENCES strategy_locked_tests(id)
	);
	CREATE INDEX IF NOT EXISTS idx_locked_occurrence_test ON strategy_locked_occurrences(test_id, status, created_at);
	CREATE INDEX IF NOT EXISTS idx_locked_occurrence_match ON strategy_locked_occurrences(match_id, status);

	-- A pre-Locked-Test strategy may have been armed by an older release.
	-- Keep rollout fail-closed while preserving versions and reports.
	UPDATE strategies SET armed=0, updated_at=CURRENT_TIMESTAMP
	WHERE armed=1 AND NOT EXISTS (
		SELECT 1 FROM strategy_locked_tests t
		WHERE t.strategy_id=strategies.id
		  AND t.strategy_version=strategies.version
		  AND t.state='REVEALED_PASS'
	);

	CREATE TABLE IF NOT EXISTS signal_decisions (
		id TEXT PRIMARY KEY,
		dedup_key TEXT NOT NULL UNIQUE,
		match_id TEXT NOT NULL,
		strategy_id TEXT NOT NULL,
		strategy_version INTEGER NOT NULL,
		status TEXT NOT NULL,
		payload_json TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		resolved_at TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_signal_match_status ON signal_decisions(match_id, status);
	CREATE INDEX IF NOT EXISTS idx_signal_created ON signal_decisions(created_at DESC);

	CREATE TABLE IF NOT EXISTS subscribers (
		id TEXT PRIMARY KEY,
		telegram_chat_id INTEGER NOT NULL UNIQUE,
		display_name TEXT NOT NULL,
		active INTEGER NOT NULL DEFAULT 1,
		terms_version TEXT NOT NULL,
		adult_confirmed INTEGER NOT NULL DEFAULT 0,
		expires_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS invitations (
		code TEXT PRIMARY KEY,
		expires_at TIMESTAMP NOT NULL,
		access_until TIMESTAMP NOT NULL,
		used_at TIMESTAMP,
		used_by_chat_id INTEGER,
		created_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS signal_deliveries (
		id TEXT PRIMARY KEY,
		signal_id TEXT NOT NULL,
		subscriber_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		status TEXT NOT NULL,
		attempts INTEGER NOT NULL,
		error TEXT NOT NULL DEFAULT '',
		message_id INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_delivery_signal ON signal_deliveries(signal_id);

	CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value_json TEXT NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
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
	for _, statement := range []string{
		`ALTER TABLE predictions ADD COLUMN expected_goals_remaining REAL NOT NULL DEFAULT 0;`,
		`ALTER TABLE predictions ADD COLUMN prob_two_plus REAL NOT NULL DEFAULT 0;`,
		`ALTER TABLE predictions ADD COLUMN contributions_json TEXT NOT NULL DEFAULT '[]';`,
		`ALTER TABLE invitations ADD COLUMN used_by_chat_id INTEGER;`,
	} {
		if _, err := s.db.Exec(statement); err != nil && !isDuplicateColumnErr(err) {
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

	contributions, err := json.Marshal(pred.Contributions)
	if err != nil {
		return err
	}
	query := `
	INSERT INTO predictions (match_id, as_of_second, prob_5m, prob_10m, prob_ft, data_quality, confidence_band, status, model_version, prediction_sequence, feature_version, expected_goals_remaining, prob_two_plus, contributions_json, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	_, err = s.db.Exec(query,
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
		pred.ExpectedGoalsRemaining,
		pred.Probabilities.TwoOrMoreBeforeFullTime,
		string(contributions),
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
	SELECT match_id, as_of_second, prob_5m, prob_10m, prob_ft, data_quality, confidence_band, status, model_version, prediction_sequence, feature_version, expected_goals_remaining, prob_two_plus, contributions_json, created_at
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
		var confBand, status, contributions string

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
			&p.ExpectedGoalsRemaining,
			&p.Probabilities.TwoOrMoreBeforeFullTime,
			&contributions,
			&p.CalculatedAt,
		)
		if err != nil {
			return nil, err
		}
		p.ConfidenceBand = domain.ConfidenceBand(confBand)
		p.Status = domain.PredictionStatus(status)
		if err := json.Unmarshal([]byte(contributions), &p.Contributions); err != nil {
			return nil, err
		}
		preds = append(preds, p)
	}

	return preds, nil
}

// GetAllPredictions returns every stored prediction across all matches,
// ordered by match and match-second, for aggregate evaluation.
func (s *SQLiteStore) GetAllPredictions() ([]domain.Prediction, error) {
	return s.getPredictions("")
}

// GetPredictionsForModel returns only the predictions a given model version
// produced. Accuracy reported across mixed versions describes no model that
// exists: the store accumulates every generation ever run, so a since-replaced
// model keeps dragging the published figures around long after its last
// prediction. Passing an empty version returns everything.
func (s *SQLiteStore) GetPredictionsForModel(modelVersion string) ([]domain.Prediction, error) {
	return s.getPredictions(modelVersion)
}

func (s *SQLiteStore) getPredictions(modelVersion string) ([]domain.Prediction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	SELECT match_id, as_of_second, prob_5m, prob_10m, prob_ft, data_quality, confidence_band, status, model_version, prediction_sequence, feature_version, expected_goals_remaining, prob_two_plus, contributions_json, created_at
	FROM predictions
	ORDER BY match_id ASC, as_of_second ASC;
	`
	args := []interface{}{}
	if modelVersion != "" {
		query = `
	SELECT match_id, as_of_second, prob_5m, prob_10m, prob_ft, data_quality, confidence_band, status, model_version, prediction_sequence, feature_version, expected_goals_remaining, prob_two_plus, contributions_json, created_at
	FROM predictions
	WHERE model_version = ?
	ORDER BY match_id ASC, as_of_second ASC;
	`
		args = append(args, modelVersion)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var preds []domain.Prediction
	for rows.Next() {
		var p domain.Prediction
		var confBand, status, contributions string

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
			&p.ExpectedGoalsRemaining,
			&p.Probabilities.TwoOrMoreBeforeFullTime,
			&contributions,
			&p.CalculatedAt,
		)
		if err != nil {
			return nil, err
		}
		p.ConfidenceBand = domain.ConfidenceBand(confBand)
		p.Status = domain.PredictionStatus(status)
		if err := json.Unmarshal([]byte(contributions), &p.Contributions); err != nil {
			return nil, err
		}
		preds = append(preds, p)
	}

	return preds, nil
}

// matchProgress is the per-match lifecycle data the evaluator needs, read
// from the two places that actually track it.
type matchProgress struct {
	status    string
	clock     int
	updatedAt string
}

// abandonedAfter is how long a match may go without an update before a match
// still marked live is treated as over.
//
// A finished match normally drops out of the provider's in-play feed, and the
// final status is only recorded if a poll catches it in the brief window
// where the feed still lists it as full-time. Miss that window — a restart, a
// rate-limit backoff — and the match stays LIVE forever, permanently
// unresolvable. A match that stopped updating hours ago in the 90th minute is
// over; refusing to say so costs real evaluation data.
const abandonedAfter = 3 * time.Hour

// minFinishedSecond is the match clock past which a stale match is considered
// complete rather than merely interrupted.
const minFinishedSecond = 85 * 60

// GetMatchOutcomes returns, per match, when goals happened, how far the match
// got, and whether it is over.
//
// The end of a match is taken from match_states.clock_seconds, not from the
// last stored event. The provider's event feed carries only goals, cards and
// substitutions and never emits a full-time marker, so the final event is
// typically minutes short of the whistle — in production, matches were ending
// 8 to 14 minutes after their last event. Using the event as the boundary
// made every metric blind to the closing stretch of every match, which is
// precisely where scoring is most likely.
func (s *SQLiteStore) GetMatchOutcomes() (map[string]evaluation.MatchOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	progress, err := s.matchProgress()
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`SELECT match_id, match_second, event_type FROM events ORDER BY match_id ASC, match_second ASC;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	goals := make(map[string][]int)
	lastEvent := make(map[string]int)

	for rows.Next() {
		var matchID, eventType string
		var matchSecond int
		if err := rows.Scan(&matchID, &matchSecond, &eventType); err != nil {
			return nil, err
		}
		if matchSecond > lastEvent[matchID] {
			lastEvent[matchID] = matchSecond
		}
		if domain.EventType(eventType).IsGoalEvent() {
			goals[matchID] = append(goals[matchID], matchSecond)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	outcomes := make(map[string]evaluation.MatchOutcome, len(progress))
	for matchID, prog := range progress {
		outcomes[matchID] = buildOutcome(goals[matchID], lastEvent[matchID], prog)
	}
	// A match with events but no row in matches still deserves its goals.
	for matchID, goalSeconds := range goals {
		if _, ok := outcomes[matchID]; !ok {
			outcomes[matchID] = evaluation.MatchOutcome{
				GoalSeconds: goalSeconds,
				FinalSecond: lastEvent[matchID],
			}
		}
	}

	return outcomes, nil
}

// GetOutcomeForMatch is the single-match, indexed-lookup equivalent of
// GetMatchOutcomes, for the ingestion hot path where scanning every event in
// the database once per poll tick would be wasteful.
func (s *SQLiteStore) GetOutcomeForMatch(matchID string) (evaluation.MatchOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT match_second, event_type FROM events WHERE match_id = ? ORDER BY match_second ASC;`, matchID)
	if err != nil {
		return evaluation.MatchOutcome{}, err
	}
	defer rows.Close()

	var goalSeconds []int
	lastEvent := 0
	for rows.Next() {
		var matchSecond int
		var eventType string
		if err := rows.Scan(&matchSecond, &eventType); err != nil {
			return evaluation.MatchOutcome{}, err
		}
		if matchSecond > lastEvent {
			lastEvent = matchSecond
		}
		if domain.EventType(eventType).IsGoalEvent() {
			goalSeconds = append(goalSeconds, matchSecond)
		}
	}
	if err := rows.Err(); err != nil {
		return evaluation.MatchOutcome{}, err
	}

	var prog matchProgress
	row := s.db.QueryRow(`
	SELECT m.status, COALESCE(ms.clock_seconds, 0), m.updated_at
	FROM matches m LEFT JOIN match_states ms ON ms.match_id = m.id
	WHERE m.id = ?;`, matchID)
	if err := row.Scan(&prog.status, &prog.clock, &prog.updatedAt); err != nil && err != sql.ErrNoRows {
		return evaluation.MatchOutcome{}, err
	}

	return buildOutcome(goalSeconds, lastEvent, prog), nil
}

func (s *SQLiteStore) matchProgress() (map[string]matchProgress, error) {
	rows, err := s.db.Query(`
	SELECT m.id, m.status, COALESCE(ms.clock_seconds, 0), m.updated_at
	FROM matches m LEFT JOIN match_states ms ON ms.match_id = m.id;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]matchProgress)
	for rows.Next() {
		var id string
		var prog matchProgress
		if err := rows.Scan(&id, &prog.status, &prog.clock, &prog.updatedAt); err != nil {
			return nil, err
		}
		out[id] = prog
	}
	return out, rows.Err()
}

// buildOutcome combines the event timeline with the match lifecycle.
func buildOutcome(goalSeconds []int, lastEventSecond int, prog matchProgress) evaluation.MatchOutcome {
	final := prog.clock
	if lastEventSecond > final {
		// An event beyond the recorded clock means the clock is behind; trust
		// whichever reached further, never less than what we have seen.
		final = lastEventSecond
	}

	finished := domain.MatchStatus(prog.status) == domain.MatchStatusFinished
	if !finished && final >= minFinishedSecond && staleFor(prog.updatedAt) > abandonedAfter {
		finished = true
	}

	return evaluation.MatchOutcome{
		GoalSeconds: goalSeconds,
		FinalSecond: final,
		Finished:    finished,
	}
}

// staleFor reports how long ago a match row was last written. An unparseable
// or missing timestamp yields zero, which keeps the match unresolved rather
// than guessing it is over.
func staleFor(updatedAt string) time.Duration {
	if updatedAt == "" {
		return 0
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339Nano, time.RFC3339} {
		if ts, err := time.Parse(layout, updatedAt); err == nil {
			return time.Since(ts)
		}
	}
	return 0
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
