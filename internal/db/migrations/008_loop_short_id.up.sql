-- Migration: 008_loop_short_id
-- Description: Add short IDs to loops
-- Created: 2026-01-07

ALTER TABLE loops ADD COLUMN short_id TEXT;

UPDATE loops
SET short_id = lower(substr(replace(id, '-', ''), 1, 8))
WHERE short_id IS NULL OR short_id = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_loops_short_id ON loops(short_id);
