-- Documentary snapshot of the ALTER applied by
-- internal/eventstore/sqlite.go's migrate() to back-fill feature_version on
-- the predictions table (added after 0001_init.sql; the predictor was
-- already setting this field in memory but it was never persisted).

ALTER TABLE predictions ADD COLUMN feature_version TEXT NOT NULL DEFAULT '';
