-- Additive rollout for provider shadow observations and the isolated
-- experimental beta lane. The executable applies equivalent idempotent
-- statements in internal/eventstore/sqlite.go.

CREATE TABLE IF NOT EXISTS odds_provider_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL,
    observed_at TIMESTAMP NOT NULL,
    accepted_events INTEGER NOT NULL,
    accepted_quotes INTEGER NOT NULL,
    payload_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_odds_observation_provider_time
    ON odds_provider_observations(provider, observed_at DESC);

CREATE TABLE IF NOT EXISTS odds_provider_comparisons (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    observed_at TIMESTAMP NOT NULL,
    primary_provider TEXT NOT NULL,
    shadow_provider TEXT NOT NULL,
    matched_events INTEGER NOT NULL,
    comparable_quotes INTEGER NOT NULL,
    payload_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_odds_comparison_time
    ON odds_provider_comparisons(observed_at DESC);

ALTER TABLE strategies ADD COLUMN experimental_armed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE signal_decisions ADD COLUMN lane TEXT NOT NULL DEFAULT 'QUALIFIED_PRODUCTION';
CREATE INDEX IF NOT EXISTS idx_signal_lane_created
    ON signal_decisions(lane, created_at DESC);
