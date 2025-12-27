-- Migration: 006_mail_and_file_locks (DOWN)
-- Description: Remove mail threads/messages and file locks
-- Created: 2025-12-27

DROP INDEX IF EXISTS idx_file_locks_active;
DROP INDEX IF EXISTS idx_file_locks_path;
DROP TABLE IF EXISTS file_locks;

DROP INDEX IF EXISTS idx_mail_messages_unread;
DROP INDEX IF EXISTS idx_mail_messages_recipient;
DROP INDEX IF EXISTS idx_mail_messages_thread_id;
DROP TABLE IF EXISTS mail_messages;

DROP INDEX IF EXISTS idx_mail_threads_workspace_id;
DROP TABLE IF EXISTS mail_threads;
