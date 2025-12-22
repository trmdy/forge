-- Migration: 003_queue_item_attempts (UP)
-- Description: Add attempts column to queue_items
-- Created: 2025-12-22

ALTER TABLE queue_items
ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
