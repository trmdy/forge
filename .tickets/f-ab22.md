---
id: f-ab22
status: open
deps: []
links: []
created: 2026-01-11T17:51:06Z
type: feature
priority: 3
assignee: Tormod Haugland
parent: f-4c08
---
# Priority mapping: fmail to forge queue

## Problem

fmail messages have a `priority` field (low/normal/high), but this has no effect when messages reach Forge loops:

```bash
fmail send @my-loop --priority high "URGENT: fix production bug"
# Message sits in queue behind 10 other items
```

The priority information is lost at the fmail→queue boundary.

## Solution

Map fmail priority to queue position:

| fmail priority | Queue behavior |
|----------------|----------------|
| `high` | Prepend to front of queue |
| `normal` | Append to end (default) |
| `low` | Append to end, after normal |

### Behavior

```bash
# Queue state: [item1, item2, item3]

fmail send @loop --priority high "urgent"
# Queue state: [urgent, item1, item2, item3]

fmail send @loop "normal task"
# Queue state: [urgent, item1, item2, item3, normal_task]

fmail send @loop --priority low "whenever"
# Queue state: [urgent, item1, item2, item3, normal_task, whenever]
```

## Implementation Notes

### Queue Item Priority (internal/queue/item.go)

```go
type Priority int

const (
    PriorityLow    Priority = -1
    PriorityNormal Priority = 0
    PriorityHigh   Priority = 1
)

type QueueItem struct {
    ID        string
    Type      ItemType
    Content   string
    Priority  Priority  // NEW
    Source    string
    CreatedAt time.Time
}
```

### Queue Insert Logic (internal/queue/queue.go)

```go
func (q *Queue) Insert(item QueueItem) {
    switch item.Priority {
    case PriorityHigh:
        // Find first non-high item and insert before it
        idx := q.findFirstNonHigh()
        q.items = slices.Insert(q.items, idx, item)
    case PriorityLow:
        // Append at end
        q.items = append(q.items, item)
    default:
        // Normal: insert before low items
        idx := q.findFirstLow()
        q.items = slices.Insert(q.items, idx, item)
    }
}

func (q *Queue) findFirstNonHigh() int {
    for i, item := range q.items {
        if item.Priority != PriorityHigh {
            return i
        }
    }
    return len(q.items)
}
```

### DM to Queue Conversion (internal/loop/engine.go)

```go
func (l *Loop) convertDMToQueueItem(dm mail.Message) QueueItem {
    return QueueItem{
        Type:      QueueItemMessage,
        Content:   dm.Body,
        Priority:  mapMailPriority(dm.Priority),
        Source:    "fmail:" + dm.ID,
        CreatedAt: dm.Time,
    }
}

func mapMailPriority(p string) Priority {
    switch p {
    case "high":
        return PriorityHigh
    case "low":
        return PriorityLow
    default:
        return PriorityNormal
    }
}
```

### CLI: forge queue ls

Show priority in queue listing:

```bash
$ forge queue ls my-loop
POS  PRIORITY  TYPE     SOURCE                      CONTENT
1    high      message  fmail:20260111-153000-0001  URGENT: fix prod bug
2    normal    message  fmail:20260111-152000-0001  implement feature X
3    normal    message  forge-cli                   review auth.go
4    low       message  fmail:20260111-151000-0001  nice-to-have cleanup
```

### CLI: forge msg --priority

Also add priority to `forge msg`:

```bash
forge msg my-loop --priority high "urgent task"
```

## Acceptance Criteria

- [ ] QueueItem has Priority field
- [ ] High priority items prepended to queue front
- [ ] Low priority items appended to queue end
- [ ] Normal priority items inserted before low items
- [ ] fmail DM priority mapped correctly when consumed by loop
- [ ] `forge queue ls` shows priority column
- [ ] `forge msg --priority` flag added
- [ ] Priority ordering maintained when multiple high-priority items

## Edge Cases

### Multiple High Priority Items

Maintain FIFO within same priority:
```
# Queue: [high1, high2, normal1]
# Add high3
# Result: [high1, high2, high3, normal1]
```

### Reordering Existing Items

This feature only affects insertion. Manual reorder via `forge queue move` is separate.

## Why This Matters

Enables:
- **Urgent tasks**: Production bugs get immediate attention
- **Background work**: Low-priority cleanup doesn't block important work
- **External coordination**: Other agents can inject urgent work via fmail

## Related

- f-1e51: Bridge forge msg + fmail (priority flows through DM conversion)
- f-54c2: Tags field (could use tags for priority: `--tag priority:high`)
