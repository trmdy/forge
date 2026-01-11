---
id: f-1fdb
status: open
deps: []
links: []
created: 2026-01-11T17:51:18Z
type: feature
priority: 3
assignee: Tormod Haugland
parent: f-3a50
---
# Per-agent cursor tracking in agents/<name>.json

## Problem

Agents have no built-in way to track "what have I already read?" across sessions. Current workarounds:

1. **Timestamp-based**: `fmail watch --since 1h` - imprecise, can miss or duplicate
2. **External state**: Agent maintains own cursor file - duplicates what fmail could do
3. **Process messages twice**: Just accept occasional duplicates

The agent registry already exists at `.fmail/agents/<name>.json` with:
```json
{
  "name": "architect",
  "host": "build-server",
  "status": "working on auth",
  "first_seen": "2026-01-10T15:00:00Z",
  "last_seen": "2026-01-10T15:30:00Z"
}
```

This is the natural place to store read cursors.

## Solution

Add `cursors` field to agent registry for tracking last-read position per topic/DM:

```json
{
  "name": "architect",
  "host": "build-server",
  "status": "working on auth",
  "first_seen": "2026-01-10T15:00:00Z",
  "last_seen": "2026-01-10T15:30:00Z",
  "cursors": {
    "task": "20260111-153000-0042",
    "build": "20260111-140000-0015",
    "@architect": "20260111-152000-0008"
  }
}
```

Add CLI commands to manage cursors:

```bash
# Mark topic as read up to latest
fmail mark-read task

# Mark up to specific ID
fmail mark-read task --at 20260111-153000-0042

# Show unread counts
fmail unread
# task: 5 unread (last read: 20260111-153000-0042)
# build: 0 unread
# @architect: 2 unread

# Watch only unread (auto-updates cursor)
fmail watch task --unread
```

## Implementation Notes

### Agent Registry Changes (internal/mail/agent.go)

```go
type Agent struct {
    Name      string            `json:"name"`
    Host      string            `json:"host,omitempty"`
    Status    string            `json:"status,omitempty"`
    FirstSeen time.Time         `json:"first_seen"`
    LastSeen  time.Time         `json:"last_seen"`
    Cursors   map[string]string `json:"cursors,omitempty"`  // NEW: topic/dm -> last_read_id
}

func (a *Agent) GetCursor(target string) string {
    if a.Cursors == nil {
        return ""
    }
    return a.Cursors[target]
}

func (a *Agent) SetCursor(target, msgID string) {
    if a.Cursors == nil {
        a.Cursors = make(map[string]string)
    }
    a.Cursors[target] = msgID
}
```

### Mark-Read Command (cmd/fmail/markread.go)

```go
var markReadCmd = &cli.Command{
    Name:  "mark-read",
    Usage: "Mark messages as read up to latest or specific ID",
    Flags: []cli.Flag{
        &cli.StringFlag{Name: "at", Usage: "Mark read up to this message ID"},
    },
    Action: func(c *cli.Context) error {
        target := c.Args().First()  // topic or @agent
        store := getStore(c)
        agent := store.GetAgent(os.Getenv("FMAIL_AGENT"))
        
        var cursorID string
        if c.String("at") != "" {
            cursorID = c.String("at")
        } else {
            // Get latest message ID
            msgs, _ := store.ListMessages(target, time.Time{})
            if len(msgs) > 0 {
                cursorID = msgs[len(msgs)-1].ID
            }
        }
        
        agent.SetCursor(target, cursorID)
        store.SaveAgent(agent)
        return nil
    },
}
```

### Unread Command (cmd/fmail/unread.go)

```go
var unreadCmd = &cli.Command{
    Name:  "unread",
    Usage: "Show unread message counts",
    Action: func(c *cli.Context) error {
        store := getStore(c)
        agent := store.GetAgent(os.Getenv("FMAIL_AGENT"))
        
        topics := store.ListTopics()
        for _, topic := range topics {
            cursor := agent.GetCursor(topic)
            msgs, _ := store.ListMessagesAfter(topic, cursor)
            if len(msgs) > 0 || c.Bool("all") {
                fmt.Printf("%s: %d unread", topic, len(msgs))
                if cursor != "" {
                    fmt.Printf(" (last read: %s)", cursor)
                }
                fmt.Println()
            }
        }
        return nil
    },
}
```

### Watch --unread Flag (cmd/fmail/watch.go)

```go
var unreadOnly bool
cmd.Flags().BoolVar(&unreadOnly, "unread", false, "Start from last read position, auto-update cursor")

// In handler:
if unreadOnly {
    agent := store.GetAgent(agentName)
    afterID = agent.GetCursor(topic)
    
    // Auto-update cursor as messages arrive
    defer func() {
        if lastSeenID != "" {
            agent.SetCursor(topic, lastSeenID)
            store.SaveAgent(agent)
        }
    }()
}
```

## Design Decisions

### Why client-side cursors?

- **Stateless protocol**: Watch remains stateless, cursor is client concern
- **Agent-specific**: Each agent tracks their own read state
- **Offline-friendly**: Cursors persist in filesystem, work without forged
- **Simple**: No protocol changes, just file updates

### Why in agent registry?

- **Already exists**: Agent file is natural home for agent-specific state
- **One file per agent**: No proliferation of cursor files
- **Discoverable**: `cat .fmail/agents/myname.json` shows everything

### Auto-update vs explicit

- `fmail watch --unread`: Auto-updates cursor as messages processed
- `fmail mark-read`: Explicit control for batch processing

## Acceptance Criteria

- [ ] Agent registry includes `cursors` map
- [ ] `fmail mark-read <topic>` sets cursor to latest
- [ ] `fmail mark-read <topic> --at <id>` sets specific cursor
- [ ] `fmail unread` shows unread counts per topic
- [ ] `fmail watch --unread` starts from cursor, auto-updates
- [ ] Cursors persist across sessions
- [ ] Works for both topics and DM inboxes

## Why This Matters

Enables:
- **Session continuity**: Resume exactly where you left off
- **Unread indicators**: TUI can show badge counts
- **Batch processing**: Mark read after processing batch
- **Multiple agents**: Each agent tracks independently

## Related

- f-de22: Cursor resume (`--after`) provides the underlying mechanism
- TUI inbox will use this for unread badges
