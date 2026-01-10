# Forge Mail

Simple agent-to-agent messaging for AI agents.

## Quick Start

```bash
# Set your identity
export FMAIL_AGENT=myagent

# Send a message
fmail send task "implement user authentication"

# View messages
fmail log task

# Watch for new messages
fmail watch task
```

No setup required. Creates `.fmail/` automatically.

## Documents

| Document | Description |
|----------|-------------|
| [SPEC.md](SPEC.md) | Full specification - commands, format, architecture |
| [DESIGN.md](DESIGN.md) | Design decisions and trade-offs |
| [PROTOCOL.md](PROTOCOL.md) | Forged transport protocol (JSON lines) |
| [ROBOT-HELP.md](ROBOT-HELP.md) | Machine-readable help format for AI agents |

## Core Concepts

- **Topics**: Named channels (`task`, `status`, `build`)
- **Direct Messages**: `@agent` syntax for point-to-point
- **Files**: Messages stored in `.fmail/` as JSON
- **Zero Config**: Works immediately, no daemon required

## Commands

```
fmail send <topic|@agent> <message>   Send a message
fmail log [topic]                     View message history
fmail watch [topic]                   Stream new messages
fmail who                             List agents
fmail status [message]                Set your status
fmail topics                          List topics
fmail gc                              Clean up old messages
```

## Why Forge Mail?

| mcp_agent_mail | fmail |
|----------------|-------|
| Python + MCP required | No dependencies |
| 80+ env vars | 3 env vars |
| Git + SQLite storage | Plain JSON files |
| Complex setup | Works immediately |

## Patterns

### Request/Response
```bash
fmail send @analyzer "analyze src/auth.go"
response=$(fmail watch @$FMAIL_AGENT --count 1 --timeout 2m)
```

### Task Assignment
```bash
fmail send @coder "implement JWT auth"
fmail status "working on JWT"
# ... work ...
fmail send @lead "done, PR #42"
```

### File Coordination
```bash
fmail send editing "src/auth.go"
# Others check before editing:
fmail log editing --since 5m
```

## Status

**Version 2.1.0-draft** - Under development
