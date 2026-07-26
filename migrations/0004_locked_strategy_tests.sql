-- Documentary migration for prospective locked strategy tests. The
-- executable applies the same idempotent schema in eventstore.migrate().

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
CREATE INDEX IF NOT EXISTS idx_locked_tests_state
    ON strategy_locked_tests(state, started_at);

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
CREATE INDEX IF NOT EXISTS idx_locked_occurrence_test
    ON strategy_locked_occurrences(test_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_locked_occurrence_match
    ON strategy_locked_occurrences(match_id, status);

-- Existing versions are retained as Validation records, but an armed flag
-- from a pre-Locked-Test release must not survive this migration.
UPDATE strategies SET armed=0, updated_at=CURRENT_TIMESTAMP
WHERE armed=1 AND NOT EXISTS (
    SELECT 1 FROM strategy_locked_tests t
    WHERE t.strategy_id=strategies.id
      AND t.strategy_version=strategies.version
      AND t.state='REVEALED_PASS'
);
