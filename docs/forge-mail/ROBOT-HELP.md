# Robot Help Specification

The `fmail --robot-help` command outputs machine-readable documentation for AI agents.

---

## Purpose

AI agents need to quickly understand how to use fmail. Robot help provides:

1. Structured command reference
2. Copy-paste examples
3. Common patterns
4. Environment requirements

---

## Output Format

Single JSON object on stdout:

```json
{
  "name": "fmail",
  "version": "2.1.0",
  "description": "Agent-to-agent messaging via .fmail/ files",

  "setup": "export FMAIL_AGENT=<your-name>",

  "commands": {
    "send": {
      "usage": "fmail send <topic|@agent> <message>",
      "flags": ["-f FILE", "--reply-to ID", "--priority low|normal|high"],
      "examples": [
        "fmail send task 'implement auth'",
        "fmail send @reviewer 'check PR #42'"
      ]
    },
    "log": {
      "usage": "fmail log [topic|@agent] [-n N] [--since TIME]",
      "flags": ["-n LIMIT", "--since TIME", "--from AGENT", "--json", "-f/--follow"],
      "examples": [
        "fmail log task -n 5",
        "fmail log @$FMAIL_AGENT --since 1h"
      ]
    },
    "watch": {
      "usage": "fmail watch [topic|@agent] [--timeout T] [--count N]",
      "flags": ["--timeout DURATION", "--count N", "--json"],
      "examples": [
        "fmail watch task",
        "fmail watch @$FMAIL_AGENT --count 1 --timeout 2m"
      ]
    },
    "who": {
      "usage": "fmail who [--json]",
      "description": "List agents in project"
    },
    "status": {
      "usage": "fmail status [message] [--clear]",
      "examples": [
        "fmail status 'working on auth'",
        "fmail status --clear"
      ]
    },
    "topics": {
      "usage": "fmail topics [--json]",
      "description": "List topics with activity"
    },
    "gc": {
      "usage": "fmail gc [--days N] [--dry-run]"
    }
  },

  "patterns": {
    "request_response": [
      "fmail send @worker 'analyze src/auth.go'",
      "response=$(fmail watch @$FMAIL_AGENT --count 1 --timeout 2m)"
    ],
    "broadcast": "fmail send status 'starting work'",
    "coordinate": [
      "fmail send editing 'src/auth.go'",
      "fmail log editing --since 5m --json | grep -q 'auth.go'"
    ]
  },

  "env": {
    "FMAIL_AGENT": "Your agent name (strongly recommended)",
    "FMAIL_ROOT": "Project directory (auto-detected)",
    "FMAIL_PROJECT": "Project ID for cross-host sync"
  },

  "message_format": {
    "id": "YYYYMMDD-HHMMSS-NNNN",
    "from": "sender agent name",
    "to": "topic or @agent",
    "time": "ISO 8601 timestamp",
    "body": "string or JSON object"
  },

  "storage": ".fmail/topics/<topic>/<id>.json and .fmail/dm/<agent>/<id>.json"
}
```

---

## Design Principles

1. **Self-contained** - All info in one JSON blob
2. **Copy-paste ready** - Examples work directly
3. **Compact** - Minimal but complete
4. **Stable** - Future versions add fields, never remove

---

## Usage by Agents

```bash
# Learn about fmail
fmail --robot-help | jq '.commands.send'

# Get examples
fmail --robot-help | jq '.patterns'
```

---

## Version History

- **2.1.0** - Simplified format, added status command
- **2.0.0** - Initial structured format
