-- Forge Database Schema
-- SQLite database for the Forge control plane
-- Version: 1.0.0

-- Enable foreign keys
PRAGMA foreign_keys = ON;

-- ============================================================================
-- NODES TABLE
-- Machines that Forge can control via SSH and tmux
-- ============================================================================
CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    ssh_target TEXT,  -- NULL for local node
    ssh_backend TEXT NOT NULL DEFAULT 'auto' CHECK (ssh_backend IN ('native', 'system', 'auto')),
    ssh_key_path TEXT,
    ssh_agent_forwarding INTEGER NOT NULL DEFAULT 0,
    ssh_proxy_jump TEXT,
    ssh_control_master TEXT,
    ssh_control_path TEXT,
    ssh_control_persist TEXT,
    ssh_timeout_seconds INTEGER,
    status TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN ('online', 'offline', 'unknown')),
    is_local INTEGER NOT NULL DEFAULT 0,
    last_seen_at TEXT,  -- ISO8601 timestamp
    metadata_json TEXT,  -- JSON blob for NodeMetadata
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes(name);
CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status);

-- ============================================================================
-- WORKSPACES TABLE
-- Managed units binding node, repo path, and tmux session
-- ============================================================================
CREATE TABLE IF NOT EXISTS workspaces (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    repo_path TEXT NOT NULL,
    tmux_session TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'error')),
    git_info_json TEXT,  -- JSON blob for GitInfo
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(node_id, repo_path),
    UNIQUE(node_id, tmux_session)
);

CREATE INDEX IF NOT EXISTS idx_workspaces_node_id ON workspaces(node_id);
CREATE INDEX IF NOT EXISTS idx_workspaces_status ON workspaces(status);
CREATE INDEX IF NOT EXISTS idx_workspaces_name ON workspaces(name);

-- ============================================================================
-- ACCOUNTS TABLE
-- Provider accounts/profiles for authentication
-- ============================================================================
CREATE TABLE IF NOT EXISTS accounts (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider IN ('anthropic', 'openai', 'google', 'custom')),
    profile_name TEXT NOT NULL,
    credential_ref TEXT NOT NULL,  -- env var, file path, or vault key
    is_active INTEGER NOT NULL DEFAULT 1,
    cooldown_until TEXT,  -- ISO8601 timestamp
    usage_stats_json TEXT,  -- JSON blob for UsageStats
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(provider, profile_name)
);

CREATE INDEX IF NOT EXISTS idx_accounts_provider ON accounts(provider);
CREATE INDEX IF NOT EXISTS idx_accounts_is_active ON accounts(is_active);
CREATE INDEX IF NOT EXISTS idx_accounts_cooldown ON accounts(cooldown_until);

-- ============================================================================
-- AGENTS TABLE
-- Running agent processes in tmux panes
-- ============================================================================
CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('opencode', 'claude-code', 'codex', 'gemini', 'generic')),
    tmux_pane TEXT NOT NULL,  -- session:window.pane format
    account_id TEXT REFERENCES accounts(id) ON DELETE SET NULL,
    state TEXT NOT NULL DEFAULT 'starting' CHECK (state IN ('working', 'idle', 'awaiting_approval', 'rate_limited', 'error', 'paused', 'starting', 'stopped')),
    state_confidence TEXT NOT NULL DEFAULT 'low' CHECK (state_confidence IN ('high', 'medium', 'low')),
    state_reason TEXT,
    state_detected_at TEXT,
    paused_until TEXT,  -- ISO8601 timestamp for auto-resume
    last_activity_at TEXT,
    metadata_json TEXT,  -- JSON blob for AgentMetadata
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(workspace_id, tmux_pane)
);

CREATE INDEX IF NOT EXISTS idx_agents_workspace_id ON agents(workspace_id);
CREATE INDEX IF NOT EXISTS idx_agents_state ON agents(state);
CREATE INDEX IF NOT EXISTS idx_agents_account_id ON agents(account_id);
CREATE INDEX IF NOT EXISTS idx_agents_type ON agents(type);

-- ============================================================================
-- QUEUE_ITEMS TABLE
-- Message queue for agents
-- ============================================================================
CREATE TABLE IF NOT EXISTS queue_items (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('message', 'pause', 'conditional')),
    position INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'dispatched', 'completed', 'failed', 'skipped')),
    attempts INTEGER NOT NULL DEFAULT 0,
    payload_json TEXT NOT NULL,  -- JSON blob for payload
    error_message TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    dispatched_at TEXT,
    completed_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_queue_items_agent_id ON queue_items(agent_id);
CREATE INDEX IF NOT EXISTS idx_queue_items_status ON queue_items(status);
CREATE INDEX IF NOT EXISTS idx_queue_items_position ON queue_items(agent_id, position);

-- ============================================================================
-- EVENTS TABLE
-- Append-only event log for observability
-- ============================================================================
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    timestamp TEXT NOT NULL DEFAULT (datetime('now')),
    type TEXT NOT NULL,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('node', 'workspace', 'agent', 'queue', 'account', 'system')),
    entity_id TEXT NOT NULL,
    payload_json TEXT,  -- JSON blob for event-specific data
    metadata_json TEXT  -- JSON blob for additional context
);

CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
CREATE INDEX IF NOT EXISTS idx_events_entity ON events(entity_type, entity_id);

-- Composite index for efficient event queries
CREATE INDEX IF NOT EXISTS idx_events_entity_timestamp ON events(entity_type, entity_id, timestamp);

-- ============================================================================
-- ALERTS TABLE
-- Active alerts requiring attention
-- ============================================================================
CREATE TABLE IF NOT EXISTS alerts (
    id TEXT PRIMARY KEY,
    workspace_id TEXT REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id TEXT REFERENCES agents(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('approval_needed', 'cooldown', 'error', 'rate_limit')),
    severity TEXT NOT NULL DEFAULT 'warning' CHECK (severity IN ('info', 'warning', 'error', 'critical')),
    message TEXT NOT NULL,
    is_resolved INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    resolved_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_alerts_workspace_id ON alerts(workspace_id);
CREATE INDEX IF NOT EXISTS idx_alerts_agent_id ON alerts(agent_id);
CREATE INDEX IF NOT EXISTS idx_alerts_is_resolved ON alerts(is_resolved);
CREATE INDEX IF NOT EXISTS idx_alerts_severity ON alerts(severity);

-- ============================================================================
-- TRANSCRIPTS TABLE
-- Agent transcript/screen history (optional, for persistence)
-- ============================================================================
CREATE TABLE IF NOT EXISTS transcripts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    content_hash TEXT NOT NULL,  -- For deduplication
    captured_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_transcripts_agent_id ON transcripts(agent_id);
CREATE INDEX IF NOT EXISTS idx_transcripts_captured_at ON transcripts(agent_id, captured_at);
CREATE INDEX IF NOT EXISTS idx_transcripts_hash ON transcripts(agent_id, content_hash);

-- ============================================================================
-- APPROVALS TABLE
-- Pending approval requests from agents
-- ============================================================================
CREATE TABLE IF NOT EXISTS approvals (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    request_type TEXT NOT NULL,  -- e.g., 'file_write', 'command_exec', 'tool_use'
    request_details_json TEXT NOT NULL,  -- JSON with request-specific info
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'denied', 'expired')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    resolved_at TEXT,
    resolved_by TEXT  -- 'user' or 'policy'
);

CREATE INDEX IF NOT EXISTS idx_approvals_agent_id ON approvals(agent_id);
CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status);

-- ============================================================================
-- LOOP RUNTIME TABLES
-- Loop-centric runtime state (profiles, pools, loops, queue, runs)
-- ============================================================================

-- PROFILES TABLE
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

-- POOLS TABLE
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

-- POOL MEMBERS TABLE
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

-- LOOPS TABLE
CREATE TABLE IF NOT EXISTS loops (
    id TEXT PRIMARY KEY,
    short_id TEXT NOT NULL,
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
CREATE UNIQUE INDEX IF NOT EXISTS idx_loops_short_id ON loops(short_id);

-- LOOP QUEUE ITEMS TABLE
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

-- LOOP RUNS TABLE
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

-- ============================================================================
-- SCHEMA VERSION TABLE
-- Track schema migrations
-- ============================================================================
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now')),
    description TEXT
);

-- Insert initial schema version
INSERT OR IGNORE INTO schema_version (version, description) 
VALUES (1, 'Initial schema with nodes, workspaces, agents, queue_items, accounts, events');

-- ============================================================================
-- TRIGGERS
-- Automatic timestamp updates
-- ============================================================================

CREATE TRIGGER IF NOT EXISTS update_nodes_timestamp 
AFTER UPDATE ON nodes
BEGIN
    UPDATE nodes SET updated_at = datetime('now') WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_workspaces_timestamp 
AFTER UPDATE ON workspaces
BEGIN
    UPDATE workspaces SET updated_at = datetime('now') WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_agents_timestamp 
AFTER UPDATE ON agents
BEGIN
    UPDATE agents SET updated_at = datetime('now') WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_accounts_timestamp 
AFTER UPDATE ON accounts
BEGIN
    UPDATE accounts SET updated_at = datetime('now') WHERE id = NEW.id;
END;

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
