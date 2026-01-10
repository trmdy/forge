-- Migration: 008_loop_short_id (DOWN)
-- Description: Remove short IDs from loops
-- Created: 2026-01-07

DROP INDEX IF EXISTS idx_loops_short_id;

-- SQLite does not support DROP COLUMN; rebuild the table without short_id.
CREATE TABLE loops_new (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    repo_path TEXT NOT NULL,
    base_prompt_path TEXT,
    base_prompt_msg TEXT,
    interval_seconds INTEGER NOT NULL DEFAULT 30,
    pool_id TEXT REFERENCES pools(id) ON DELETE SET NULL,
    profile_id TEXT REFERENCES profiles(id) ON DELETE SET NULL,
    state TEXT NOT NULL DEFAULT 'stopped' CHECK (state IN ('running', 'sleeping', 'waiting', 'stopped', 'error')),
    last_run_at TEXT,
    last_exit_code INTEGER,
    last_error TEXT,
    log_path TEXT,
    ledger_path TEXT,
    tags_json TEXT,
    metadata_json TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO loops_new (
    id, name, repo_path, base_prompt_path, base_prompt_msg,
    interval_seconds, pool_id, profile_id, state,
    last_run_at, last_exit_code, last_error,
    log_path, ledger_path, tags_json, metadata_json,
    created_at, updated_at
)
SELECT
    id, name, repo_path, base_prompt_path, base_prompt_msg,
    interval_seconds, pool_id, profile_id, state,
    last_run_at, last_exit_code, last_error,
    log_path, ledger_path, tags_json, metadata_json,
    created_at, updated_at
FROM loops;

DROP TABLE loops;
ALTER TABLE loops_new RENAME TO loops;

CREATE INDEX IF NOT EXISTS idx_loops_repo_path ON loops(repo_path);
CREATE INDEX IF NOT EXISTS idx_loops_state ON loops(state);
CREATE INDEX IF NOT EXISTS idx_loops_pool_id ON loops(pool_id);
CREATE INDEX IF NOT EXISTS idx_loops_profile_id ON loops(profile_id);

CREATE TRIGGER IF NOT EXISTS update_loops_timestamp
AFTER UPDATE ON loops
BEGIN
    UPDATE loops SET updated_at = datetime('now') WHERE id = NEW.id;
END;
