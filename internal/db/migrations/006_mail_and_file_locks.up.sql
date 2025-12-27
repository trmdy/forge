-- Migration: 006_mail_and_file_locks
-- Description: Add mail threads/messages and file locks
-- Created: 2025-12-27

-- ============================================================================
-- MAIL_THREADS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS mail_threads (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    subject TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_mail_threads_workspace_id ON mail_threads(workspace_id);

-- ============================================================================
-- MAIL_MESSAGES TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS mail_messages (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL REFERENCES mail_threads(id) ON DELETE CASCADE,
    sender_agent_id TEXT REFERENCES agents(id) ON DELETE SET NULL,
    recipient_type TEXT NOT NULL CHECK (recipient_type IN ('agent', 'workspace', 'broadcast')),
    recipient_id TEXT,
    subject TEXT,
    body TEXT NOT NULL,
    importance TEXT NOT NULL DEFAULT 'normal',
    ack_required INTEGER NOT NULL DEFAULT 0,
    read_at TEXT,
    acked_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_mail_messages_thread_id ON mail_messages(thread_id);
CREATE INDEX IF NOT EXISTS idx_mail_messages_recipient ON mail_messages(recipient_type, recipient_id);
CREATE INDEX IF NOT EXISTS idx_mail_messages_unread ON mail_messages(recipient_id, read_at);

-- ============================================================================
-- FILE_LOCKS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS file_locks (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    path_pattern TEXT NOT NULL,
    exclusive INTEGER NOT NULL DEFAULT 1,
    reason TEXT,
    ttl_seconds INTEGER NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    released_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_file_locks_active
    ON file_locks(workspace_id, expires_at)
    WHERE released_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_file_locks_path
    ON file_locks(workspace_id, path_pattern);
