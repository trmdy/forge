---
id: f-f16c
status: open
deps: [f-de22]
links: []
created: 2026-01-11T17:50:53Z
type: feature
priority: 3
assignee: Tormod Haugland
parent: f-3a50
---
# Richer JSON output for topics/log

## Problem

Current `fmail topics --json` and `fmail log --json` output is minimal:

```bash
$ fmail topics --json
{"name":"task","count":42}
{"name":"build","count":15}
```

Missing information needed for:
- **Cross-host debugging**: Which host did messages come from?
- **TUI summaries**: What's the latest message? Who sent it?
- **Cursor persistence**: What's the last message ID for resume?

## Solution

Enrich JSON output with additional metadata:

### fmail topics --json

```json
{
  "name": "task",
  "count": 42,
  "last_id": "20260111-153000-0042",
  "last_from": "architect",
  "last_time": "2026-01-11T15:30:00Z",
  "project_id": "forge-abc123"
}
```

### fmail log --json (header)

Add optional summary header before messages:

```bash
$ fmail log task --json --summary
{"summary": {"topic": "task", "count": 42, "first_id": "20260110-100000-0001", "last_id": "20260111-153000-0042", "project_id": "forge-abc123"}}
{"id": "20260111-152500-0041", "from": "coder", ...}
{"id": "20260111-153000-0042", "from": "architect", ...}
```

## Implementation Notes

### Topics Command (cmd/fmail/topics.go)

```go
type TopicInfo struct {
    Name      string    `json:"name"`
    Count     int       `json:"count"`
    LastID    string    `json:"last_id,omitempty"`
    LastFrom  string    `json:"last_from,omitempty"`
    LastTime  time.Time `json:"last_time,omitempty"`
    ProjectID string    `json:"project_id,omitempty"`
}

func getTopicInfo(store *mail.Store, topic string) TopicInfo {
    msgs, _ := store.ListMessages(topic, time.Time{})
    info := TopicInfo{
        Name:      topic,
        Count:     len(msgs),
        ProjectID: store.ProjectID(),
    }
    if len(msgs) > 0 {
        last := msgs[len(msgs)-1]
        info.LastID = last.ID
        info.LastFrom = last.From
        info.LastTime = last.Time
    }
    return info
}
```

### Log Command (cmd/fmail/log.go)

```go
var showSummary bool
cmd.Flags().BoolVar(&showSummary, "summary", false, "Include summary header in JSON output")

// In handler:
if jsonOutput && showSummary {
    summary := LogSummary{
        Topic:     topic,
        Count:     len(msgs),
        FirstID:   msgs[0].ID,
        LastID:    msgs[len(msgs)-1].ID,
        ProjectID: store.ProjectID(),
    }
    json.NewEncoder(os.Stdout).Encode(map[string]any{"summary": summary})
}

for _, msg := range msgs {
    json.NewEncoder(os.Stdout).Encode(msg)
}
```

### Human Output Enhancement

Also improve human-readable output:

```bash
$ fmail topics
TOPIC        MESSAGES    LAST ACTIVITY    LAST FROM
task         42          5m ago           architect
build        15          1h ago           ci-bot
status       128         2m ago           coder-1

$ fmail log task -n 3
20260111-153000-0042  architect  5m ago   implement user auth
20260111-152500-0041  coder      10m ago  on it
20260111-152000-0040  architect  15m ago  please review PR #42
```

### Store Changes

Add method to get topic metadata efficiently:

```go
func (s *Store) TopicMetadata(topic string) (*TopicMeta, error) {
    dir := s.TopicDir(topic)
    entries, err := os.ReadDir(dir)
    if err != nil {
        return nil, err
    }
    
    meta := &TopicMeta{
        Name:  topic,
        Count: len(entries),
    }
    
    if len(entries) > 0 {
        // Last entry (sorted by name = sorted by ID)
        lastFile := entries[len(entries)-1].Name()
        lastID := strings.TrimSuffix(lastFile, ".json")
        lastMsg, _ := s.ReadMessage(topic, lastID)
        meta.LastID = lastMsg.ID
        meta.LastFrom = lastMsg.From
        meta.LastTime = lastMsg.Time
    }
    
    return meta, nil
}
```

## Acceptance Criteria

- [ ] `fmail topics --json` includes last_id, last_from, last_time, project_id
- [ ] `fmail log --json --summary` emits summary header before messages
- [ ] Human output for topics shows last activity and sender
- [ ] ProjectID correctly derived from store
- [ ] Empty topics handled gracefully (omit last_* fields)

## Why This Matters

Enables:
- **TUI inbox**: Show "42 messages, last from architect 5m ago"
- **Cursor persistence**: Store last_id for reliable resume
- **Cross-host debugging**: Identify which host/project messages belong to
- **Quick status**: Glance at topics without reading messages

## Dependencies

- f-de22: Cursor concept (last_id) used for resume

## Related

- f-1fdb: Per-agent cursor tracking will use last_id
- TUI will consume this for inbox summaries
