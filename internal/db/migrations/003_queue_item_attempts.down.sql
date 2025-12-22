-- Migration: 003_queue_item_attempts (DOWN)
-- Description: Remove attempts column from queue_items
-- Created: 2025-12-22

-- SQLite does not support DROP COLUMN; rebuild the table without attempts.
CREATE TABLE queue_items_new (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('message', 'pause', 'conditional')),
    position INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'dispatched', 'completed', 'failed', 'skipped')),
    payload_json TEXT NOT NULL,
    error_message TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    dispatched_at TEXT,
    completed_at TEXT
);

INSERT INTO queue_items_new (
    id, agent_id, type, position, status, payload_json, error_message,
    created_at, dispatched_at, completed_at
)
SELECT
    id, agent_id, type, position, status, payload_json, error_message,
    created_at, dispatched_at, completed_at
FROM queue_items;

DROP TABLE queue_items;
ALTER TABLE queue_items_new RENAME TO queue_items;

CREATE INDEX IF NOT EXISTS idx_queue_items_agent_id ON queue_items(agent_id);
CREATE INDEX IF NOT EXISTS idx_queue_items_status ON queue_items(status);
CREATE INDEX IF NOT EXISTS idx_queue_items_position ON queue_items(agent_id, position);
