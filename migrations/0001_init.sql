-- Documentary snapshot of the schema created inline by
-- internal/eventstore/sqlite.go's migrate(). This file is not executed by
-- the Go binary (which still applies its schema via CREATE TABLE IF NOT
-- EXISTS at startup) — it exists so the schema's history is versioned and
-- reviewable, per context/Golo_Blueprint_MVP.md §11.

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
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_predictions_match ON predictions(match_id, as_of_second);
