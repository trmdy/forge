-- Migration: 007_loop_runtime
-- Description: Add loop runtime tables for loop-centric Forge
-- Created: 2026-01-06

-- ==========================================================================
-- PROFILES TABLE
-- ==========================================================================
CREATE TABLE IF NOT EXISTS profiles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    harness TEXT NOT NULL,
    auth_kind TEXT,
    auth_home TEXT,
    prompt_mode TEXT NOT NULL DEFAULT 'env' CHECK (prompt_mode IN ('env', 'stdin', 'path')),
    command_template TEXT NOT NULL,
    model TEXT,
    extra_args_json TEXT,
    env_json TEXT,
    max_concurrency INTEGER NOT NULL DEFAULT 1,
    cooldown_until TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_profiles_harness ON profiles(harness);
CREATE INDEX IF NOT EXISTS idx_profiles_cooldown ON profiles(cooldown_until);

-- ==========================================================================
-- POOLS TABLE
-- ==========================================================================
CREATE TABLE IF NOT EXISTS pools (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    strategy TEXT NOT NULL DEFAULT 'round_robin',
    is_default INTEGER NOT NULL DEFAULT 0,
    metadata_json TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_pools_default ON pools(is_default);

-- ==========================================================================
-- POOL MEMBERS TABLE
-- ==========================================================================
CREATE TABLE IF NOT EXISTS pool_members (
    id TEXT PRIMARY KEY,
    pool_id TEXT NOT NULL REFERENCES pools(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    weight INTEGER NOT NULL DEFAULT 1,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(pool_id, profile_id)
);

CREATE INDEX IF NOT EXISTS idx_pool_members_pool_id ON pool_members(pool_id);
CREATE INDEX IF NOT EXISTS idx_pool_members_profile_id ON pool_members(profile_id);

-- ==========================================================================
-- LOOPS TABLE
-- ==========================================================================
CREATE TABLE IF NOT EXISTS loops (
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

CREATE INDEX IF NOT EXISTS idx_loops_repo_path ON loops(repo_path);
CREATE INDEX IF NOT EXISTS idx_loops_state ON loops(state);
CREATE INDEX IF NOT EXISTS idx_loops_pool_id ON loops(pool_id);
CREATE INDEX IF NOT EXISTS idx_loops_profile_id ON loops(profile_id);

-- ==========================================================================
-- LOOP QUEUE ITEMS TABLE
-- ==========================================================================
CREATE TABLE IF NOT EXISTS loop_queue_items (
    id TEXT PRIMARY KEY,
    loop_id TEXT NOT NULL REFERENCES loops(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN (
        'message_append',
        'next_prompt_override',
        'pause',
        'stop_graceful',
        'kill_now',
        'steer_message'
    )),
    position INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'dispatched', 'completed', 'failed', 'skipped')),
    attempts INTEGER NOT NULL DEFAULT 0,
    payload_json TEXT NOT NULL,
    error_message TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    dispatched_at TEXT,
    completed_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_loop_queue_items_loop_id ON loop_queue_items(loop_id);
CREATE INDEX IF NOT EXISTS idx_loop_queue_items_status ON loop_queue_items(status);
CREATE INDEX IF NOT EXISTS idx_loop_queue_items_position ON loop_queue_items(loop_id, position);

-- ==========================================================================
-- LOOP RUNS TABLE
-- ==========================================================================
CREATE TABLE IF NOT EXISTS loop_runs (
    id TEXT PRIMARY KEY,
    loop_id TEXT NOT NULL REFERENCES loops(id) ON DELETE CASCADE,
    profile_id TEXT REFERENCES profiles(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'error', 'killed')),
    prompt_source TEXT,
    prompt_path TEXT,
    prompt_override INTEGER NOT NULL DEFAULT 0,
    started_at TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at TEXT,
    exit_code INTEGER,
    output_tail TEXT,
    metadata_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_loop_runs_loop_id ON loop_runs(loop_id);
CREATE INDEX IF NOT EXISTS idx_loop_runs_profile_id ON loop_runs(profile_id);
CREATE INDEX IF NOT EXISTS idx_loop_runs_status ON loop_runs(status);

-- ==========================================================================
-- LOOP RUNTIME TRIGGERS
-- ==========================================================================

CREATE TRIGGER IF NOT EXISTS update_profiles_timestamp
AFTER UPDATE ON profiles
BEGIN
    UPDATE profiles SET updated_at = datetime('now') WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_pools_timestamp
AFTER UPDATE ON pools
BEGIN
    UPDATE pools SET updated_at = datetime('now') WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_loops_timestamp
AFTER UPDATE ON loops
BEGIN
    UPDATE loops SET updated_at = datetime('now') WHERE id = NEW.id;
END;
