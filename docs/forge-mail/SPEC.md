# Forge Mail Specification

**Version:** 2.1.0-draft
**Status:** Draft
**Last Updated:** 2026-01-10

---

## One-Line Summary

Agents send and receive messages through topics and direct messages using simple file storage.

---

## Quick Start

```bash
# In any project directory
fmail send task "implement user auth"

# See recent messages
fmail log task

# Stream new messages
fmail watch task
```

No setup. No config. Creates `.fmail/` automatically.

---

## Core Concepts

### Projects

A **project** is a shared message space. Agents in the same project can communicate.

```
Project ID:   forge-abc123        (stable across hosts)
Project Root: /home/user/my-app   (local path)
```

When you run `fmail` in a directory, it finds the project by:
1. Looking for `.fmail/` directory walking upward
2. Looking for `.git/` directory (git root becomes project root)
3. Using current directory

The **project ID** is:
- Derived from git remote URL (if available) - stable across clones
- Explicitly set via `FMAIL_PROJECT` environment variable
- Derived from directory name as fallback

### Agents

An **agent** is any process that sends or receives messages.

```bash
export FMAIL_AGENT=architect    # Your identity
```

Without `FMAIL_AGENT`, commands will prompt or auto-generate: `anon-<pid>`.

Agents are tracked when they send messages or when they claim a name with `fmail register`.

### Topics

A **topic** is a named channel for messages.

```
task          # Work assignments
status        # Agent status updates
build         # Build notifications
```

Topic names: lowercase alphanumeric with hyphens. Examples: `task`, `code-review`, `build-status`.

Anyone can send to any topic. Anyone can read.

### Direct Messages

Prefix with `@` for agent-to-agent messages:

```bash
fmail send @reviewer "please check PR #42"
fmail watch @myname
```

Direct messages are stored separately and only visible to the recipient.

By default, `fmail` refuses to read or watch another agent's DM inbox. Use
`--allow-other-dm` to override when needed (for admin/debug use). DM
directories and DM message files are created with restrictive permissions
(0700 for directories, 0600 for files) as a best-effort guard.

---

## Architecture

### Two Modes

**Standalone Mode** (default)
- Messages stored as files in `.fmail/`
- Works anywhere, no daemon required
- Polling-based watch (100ms)
- Single-host only

**Connected Mode** (with forged)
- Real-time message streaming via Unix socket
- Cross-host synchronization
- Live agent presence
- File storage remains source of truth

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Agent A    │     │    forged    │     │   Agent B    │
│   (host-1)   │────▶│   (server)   │◀────│   (host-2)   │
└──────────────┘     └──────────────┘     └──────────────┘
       │                    │                    │
       │                    ▼                    │
       │            ┌──────────────┐             │
       └───────────▶│   .fmail/    │◀────────────┘
                    │   (files)    │
                    └──────────────┘
```

### Message Flow

1. Agent calls `fmail send topic "message"`
2. If connected to forged: message sent via socket, forged broadcasts and writes to disk
3. If standalone: message written directly to `.fmail/topics/<topic>/<id>.json`
4. Other agents receive via `fmail watch` or `fmail log`

### Why Files?

Files are the primary storage because:
- **Inspectable**: `ls`, `cat`, `grep` work directly
- **No dependencies**: No database, no migrations
- **Portable**: Works in containers, CI, any environment
- **Git-friendly**: Can commit message history if desired

---

## CLI Commands

### fmail send

Send a message to a topic or agent.

```bash
fmail send <topic|@agent> <message>
fmail send <topic|@agent> -f <file>
echo "message" | fmail send <topic|@agent>
```

Examples:
```bash
fmail send task "implement auth"
fmail send @reviewer "please check PR #42"
cat spec.md | fmail send docs
fmail send task --reply-to 0042 "done"
```

Options:
```
-f, --file        Read message from file
--reply-to, -r    Reference a previous message ID
--priority, -p    Set priority: low, normal (default), high
--tag, -t         Add tags (repeatable or comma-separated)
--json            Output sent message as JSON
```

### fmail log

View message history.

```bash
fmail log [topic|@agent]
```

Examples:
```bash
fmail log                    # All recent messages
fmail log task               # Just task topic
fmail log task -n 5          # Last 5
fmail log --since 1h         # Last hour
fmail log @myname            # Messages to me (inbox)
```

Options:
```
-n, --limit     Max messages (default: 20)
--since         Time filter (1h, 30m, 2024-01-10)
--from          Filter by sender
--follow, -f    Stream new messages (like tail -f)
--allow-other-dm  Allow reading another agent's DM inbox
--json          JSON output
```

JSON output uses JSON Lines (one message per line).

Note: `fmail log @$FMAIL_AGENT` shows your inbox.

### fmail watch

Stream messages as they arrive.

```bash
fmail watch [topic|@agent]
```

Examples:
```bash
fmail watch                  # All topics
fmail watch task             # Just task
fmail watch @myname          # My direct messages
fmail watch --timeout 5m     # Exit after 5 minutes
```

Options:
```
--timeout       Max wait time (default: forever)
--count, -c     Exit after N messages
--allow-other-dm  Allow watching another agent's DM inbox
--json          JSON output
```

In standalone mode: polls every 100ms.
In connected mode: real-time streaming via forged.

Exit codes:
```
0    Exited normally (timeout, count, or Ctrl+C)
```

### fmail who

List known agents in this project.

```bash
fmail who

NAME         LAST SEEN    STATUS
architect    2m ago       idle
coder-1      active       working on auth
reviewer     1h ago       offline
```

Options:
```
--json          JSON output
```

### fmail register

Request a unique agent name. With no arguments, generates a new name.

```bash
fmail register
fmail register agent-42
```

Options:
```
--json          JSON output
```

### fmail status

Set or show your status.

```bash
fmail status                 # Show your current status
fmail status "working on auth"
fmail status --clear
```

This is visible in `fmail who` output.

### fmail topics

List all topics with activity.

```bash
fmail topics

TOPIC        MESSAGES    LAST ACTIVITY
task         42          5m ago
status       128         2m ago
build        15          1h ago
```

Options:
```
--json          JSON output
```

### fmail gc

Clean up old messages.

```bash
fmail gc                     # Remove messages older than 7 days
fmail gc --days 1            # Remove messages older than 1 day
fmail gc --dry-run           # Show what would be removed
```

### fmail init

Initialize a project (optional, usually auto-created).

```bash
fmail init                   # Initialize in current directory
fmail init --project myproj  # Set explicit project ID
```

### fmail help

```bash
fmail help                   # Human-readable help
fmail --robot-help           # Machine-readable for AI agents
```

---

## Message Format

Messages are JSON files with a simple structure:

```json
{
  "id": "20260110-153000-0001",
  "from": "architect",
  "to": "task",
  "time": "2026-01-10T15:30:00Z",
  "body": "implement user auth"
}
```

### Message ID Format

IDs are: `YYYYMMDD-HHMMSS-NNNN` where NNNN is a sequence within that second.

Benefits:
- Globally sortable across topics and hosts
- Human-readable timestamp
- Collision-resistant

### Optional Fields

```json
{
  "id": "20260110-153000-0001",
  "from": "architect",
  "to": "@reviewer",
  "time": "2026-01-10T15:30:00Z",
  "body": "please check PR #42",
  "reply_to": "20260110-152500-0003",
  "priority": "high",
  "host": "build-server",
  "tags": ["urgent", "auth"]
}
```

| Field | Description |
|-------|-------------|
| `reply_to` | ID of message being replied to |
| `priority` | `low`, `normal` (default), `high` |
| `host` | Originating hostname (in connected mode) |
| `tags` | Array of lowercase alphanumeric tags (max 10, each max 50 chars) |

### Body Content

The body can be a string or any JSON value:

```bash
# String
fmail send task "implement auth"

# Object (auto-detected from JSON syntax)
fmail send task '{"action": "build", "target": "main"}'

# File contents
fmail send docs -f README.md
```

---

## Storage Layout

```
.fmail/
├── topics/                      # Topic messages
│   ├── task/
│   │   ├── 20260110-153000-0001.json
│   │   └── 20260110-153100-0002.json
│   └── build/
│       └── 20260110-140000-0001.json
├── dm/                          # Direct messages (by recipient)
│   └── reviewer/
│       └── 20260110-153000-0001.json
├── agents/                      # Agent registry
│   └── architect.json
└── project.json                 # Project metadata
```

The structure separates topics from DMs for clarity.

### project.json

```json
{
  "id": "forge-abc123",
  "created": "2026-01-10T15:00:00Z"
}
```

### Agent Registry

```json
// .fmail/agents/architect.json
{
  "name": "architect",
  "host": "build-server",
  "status": "working on auth",
  "first_seen": "2026-01-10T15:00:00Z",
  "last_seen": "2026-01-10T15:30:00Z"
}
```

---

## Environment Variables

```
FMAIL_AGENT      Your agent name (strongly recommended)
FMAIL_ROOT       Project directory (default: auto-detect from .fmail or .git)
FMAIL_PROJECT    Project ID for cross-host coordination (default: derived from git remote)
```

When running under forge, `FMAIL_AGENT` is set automatically to the loop name.

---

## Forged Integration

### Connection

fmail connects to forged automatically when available:

1. Unix socket at `.fmail/forged.sock` (preferred, per project root)
2. TCP at `127.0.0.1:7463` (optional fallback)

If both are unavailable, fmail falls back to standalone file mode.

### Protocol

Simple line-based JSON over sockets. The full contract (send/watch schemas,
errors, fallback rules, and ordering semantics) is defined in
[PROTOCOL.md](PROTOCOL.md).

### Cross-Host Sync

When forged is running on multiple hosts with the same project ID:

1. Agent A (host-1) sends message
2. Local forged writes to disk and broadcasts to local subscribers
3. Forged syncs with other forged instances via configured relay
4. Agent B (host-2) receives message

Messages include `host` field to identify origin.

Note: Cross-host sync requires explicit relay configuration. Single-host is the default.

### Relay Configuration (forged)

Cross-host sync is opt-in and configured in `config.yaml` for forged:

```yaml
mail:
  relay:
    enabled: true
    peers:
      - "host-a:7463"
      - "host-b:7463"
    dial_timeout: 2s
    reconnect_interval: 2s
```

Relay peers are trusted (no auth in v1). Each host connects to the listed peers
and streams all messages for matching project IDs. Use a full mesh or a hub
depending on your topology.

---

## Robot Help

`fmail --robot-help` outputs machine-readable documentation. See [ROBOT-HELP.md](ROBOT-HELP.md) for the full format.

---

## Patterns

### Task Assignment

```bash
# Lead assigns work
fmail send @coder-1 "implement JWT auth"

# Coder acknowledges and starts
fmail status "working on JWT auth"
fmail send @lead --reply-to "$MSG_ID" "on it"

# Coder completes
fmail send @lead "done, PR #42 ready"
fmail status --clear
```

### File Coordination

```bash
# Announce intent before editing
fmail send editing "src/auth.go"

# Check before editing (another agent)
recent=$(fmail log editing --since 5m --json)
if echo "$recent" | grep -q "auth.go"; then
    echo "Someone is editing this file"
fi
```

### Request/Response

```bash
# Requester
fmail send @analyzer "analyze src/auth.go"
response=$(fmail watch @$FMAIL_AGENT --count 1 --timeout 2m)
echo "$response"

# Responder (in a loop)
fmail watch @$FMAIL_AGENT --json | while read -r msg; do
    sender=$(echo "$msg" | jq -r '.from')
    # process request...
    fmail send @"$sender" "analysis complete: 2 issues found"
done
```

### Broadcast Status

```bash
# Set your status (visible in `fmail who`)
fmail status "building main branch"

# Or broadcast to a topic
fmail send build '{"status":"started","target":"main"}'
```

### Multi-Agent Coordination

```bash
# Coordinator starts work
fmail send task "implement auth module"

# Workers claim tasks
fmail send task '{"claimed":"auth-login","by":"'$FMAIL_AGENT'"}'

# Workers report completion
fmail send task '{"done":"auth-login","by":"'$FMAIL_AGENT'"}'
```

---

## Comparison with mcp_agent_mail

| Aspect | mcp_agent_mail | fmail |
|--------|----------------|-------|
| Setup | Python, MCP config | `export FMAIL_AGENT=name` |
| Server | Required | Optional (forged) |
| Storage | Git + SQLite | JSON files |
| Config | 80+ env vars | 3 env vars |
| Debug | MCP tools | ls, cat, jq |
| Protocol | MCP/JSON-RPC | JSON lines over socket |

---

## FAQ

**Q: Do I need forge to use fmail?**

No. `fmail` works standalone. Just `export FMAIL_AGENT=myname` and go.

**Q: What if two agents write simultaneously?**

Atomic writes with timestamp-based IDs. Both messages preserved.

**Q: How do I know if a message was received?**

Use request/response: send to `@agent`, use `fmail watch --count 1` for reply.

**Q: How do cross-host messages work?**

Single-host by default. Cross-host requires forged with relay configuration.

**Q: Maximum message size?**

1MB default. For large data, write to a file and send the path.

**Q: How to clean up old messages?**

`fmail gc` removes messages older than 7 days. `fmail gc --days 1` for aggressive cleanup.
