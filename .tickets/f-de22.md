---
id: f-de22
status: open
deps: []
links: []
created: 2026-01-11T17:50:44Z
type: feature
priority: 2
assignee: Tormod Haugland
parent: f-3a50
---
# Cursor resume with --after

## Problem

Currently `fmail watch` and `fmail log` only support `--since <timestamp>` for filtering. This has issues:

1. **Clock drift**: Timestamps can be unreliable across hosts or after restarts
2. **Duplicate messages**: If you resume with `--since 1m`, you might get duplicates or miss messages at the boundary
3. **No reliable replay**: Clients can't persist "last seen message" and resume exactly where they left off

Message IDs (`YYYYMMDD-HHMMSS-NNNN`) are globally sortable and unique - they're the natural cursor.

## Solution

Add `--after <msg-id>` flag that returns messages with ID strictly greater than the given ID:

```bash
# Resume from last seen message
fmail watch task --after 20260111-153000-0042

# Get messages after a specific point in log
fmail log task --after 20260111-140000-0001
```

## Implementation Notes

### CLI Changes

```go
// cmd/fmail/watch.go and cmd/fmail/log.go
var afterID string
cmd.Flags().StringVar(&afterID, "after", "", "Return messages after this message ID")
```

### Store Changes (internal/mail/store.go)

Message IDs are lexicographically sortable, so filtering is straightforward:

```go
func (s *Store) ListMessagesAfter(topic, afterID string) ([]Message, error) {
    dir := s.TopicDir(topic)
    entries, _ := os.ReadDir(dir)
    
    var msgs []Message
    for _, e := range entries {
        id := strings.TrimSuffix(e.Name(), ".json")
        if id > afterID {  // Lexicographic comparison works!
            msg, _ := s.ReadMessage(topic, id)
            msgs = append(msgs, msg)
        }
    }
    return msgs, nil
}
```

### Watcher Changes (internal/mail/watcher.go)

Track cursor by ID instead of timestamp:

```go
type Watcher struct {
    topic   string
    afterID string  // Cursor: last seen message ID
}

func (w *Watcher) Poll() ([]Message, error) {
    msgs, _ := w.store.ListMessagesAfter(w.topic, w.afterID)
    if len(msgs) > 0 {
        w.afterID = msgs[len(msgs)-1].ID  // Update cursor
    }
    return msgs, nil
}
```

### Protocol Changes (PROTOCOL.md)

Extend watch request to accept `after` field:

```json
{
  "cmd": "watch",
  "topic": "task",
  "after": "20260111-153000-0042"
}
```

The `since` field (timestamp) remains supported for backward compatibility. If both provided, `after` takes precedence.

### Interaction with --since

- `--since`: Filter by message timestamp (existing)
- `--after`: Filter by message ID (new, preferred)
- Both: `--after` takes precedence

## Acceptance Criteria

- [ ] `fmail watch task --after <id>` resumes from that ID
- [ ] `fmail log task --after <id>` shows messages after that ID
- [ ] Works in standalone mode
- [ ] Works in connected mode (forged)
- [ ] Protocol updated to accept `after` field
- [ ] `--after` takes precedence over `--since` if both provided
- [ ] No duplicate messages when resuming

## Why This Matters

This is foundational for:
- **f-e651**: backlog_done marker needs replay logic
- **f-1fdb**: Per-agent cursor tracking stores last_read_id
- **Reliable automation**: Scripts can persist cursor and resume exactly

## Related

- f-e651: backlog_done marker (depends on this)
- f-f16c: Richer JSON output (depends on this for last_id)
- f-1fdb: Per-agent cursor tracking (uses this for resume)
