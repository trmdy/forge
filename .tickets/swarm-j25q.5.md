---
id: swarm-j25q.5
status: closed
deps: []
links: []
created: 2025-12-27T09:34:17.765797515+01:00
type: task
priority: 1
parent: swarm-j25q
---
# Add mail and lock DB schema

Create database schema for mail threads, messages, and file locks.

## From UX_FEEDBACK_2.md - Phase 4

## DB Tables

### mail_threads
```sql
CREATE TABLE mail_threads (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    subject TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id)
);
```

### mail_messages
```sql
CREATE TABLE mail_messages (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL,
    sender_agent_id TEXT,          -- NULL if from user/system
    recipient_type TEXT NOT NULL,  -- agent, workspace, broadcast
    recipient_id TEXT,
    subject TEXT,
    body TEXT NOT NULL,
    importance TEXT DEFAULT "normal",
    ack_required BOOLEAN DEFAULT FALSE,
    read_at DATETIME,
    acked_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (thread_id) REFERENCES mail_threads(id),
    FOREIGN KEY (sender_agent_id) REFERENCES agents(id)
);
CREATE INDEX idx_mail_messages_recipient ON mail_messages(recipient_type, recipient_id);
CREATE INDEX idx_mail_messages_unread ON mail_messages(recipient_id, read_at);
```

### file_locks
```sql
CREATE TABLE file_locks (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    path_pattern TEXT NOT NULL,    -- file path or glob
    exclusive BOOLEAN DEFAULT TRUE,
    reason TEXT,
    ttl_seconds INTEGER NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    released_at DATETIME,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id),
    FOREIGN KEY (agent_id) REFERENCES agents(id)
);
CREATE INDEX idx_file_locks_active ON file_locks(workspace_id, expires_at) WHERE released_at IS NULL;
CREATE INDEX idx_file_locks_path ON file_locks(workspace_id, path_pattern);
```

## Migration
- Add as new migration file in internal/db/migrations/
- Include indexes for performance
- Add cleanup job for expired locks


