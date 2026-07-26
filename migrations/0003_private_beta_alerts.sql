-- Documentary migration for the private-beta alert engine. The executable
-- applies equivalent idempotent statements in internal/eventstore/sqlite.go.

ALTER TABLE predictions ADD COLUMN expected_goals_remaining REAL NOT NULL DEFAULT 0;
ALTER TABLE predictions ADD COLUMN prob_two_plus REAL NOT NULL DEFAULT 0;
ALTER TABLE predictions ADD COLUMN contributions_json TEXT NOT NULL DEFAULT '[]';

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
