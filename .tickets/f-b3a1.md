---
id: f-b3a1
status: open
deps: []
links: []
created: 2026-01-11T17:51:10Z
type: feature
priority: 3
assignee: Tormod Haugland
parent: f-4c08
---
# Ledger traceability: store triggering fmail msg ID

## Problem

The ledger records what each loop iteration did, but not **why** it ran:

```markdown
## 2026-01-11 15:30:00 - coder-1 (iteration 42)

**Profile:** claude-a
**Exit code:** 0
**Duration:** 45s

### Summary
Fixed authentication bug in login flow. Created PR #42.

### Git changes
- M src/auth/login.go
- A src/auth/login_test.go
```

Missing: What triggered this iteration? Was it:
- A scheduled iteration (base prompt)?
- A message from `forge msg`?
- An fmail DM from another agent?

Without this, you can't trace "who asked for this work?"

## Solution

Store the triggering message ID in the ledger entry:

```markdown
## 2026-01-11 15:30:00 - coder-1 (iteration 42)

**Profile:** claude-a
**Trigger:** fmail:20260111-152500-0001 (from: architect)
**Exit code:** 0
**Duration:** 45s
```

And in the structured data:

```go
type LedgerEntry struct {
    Timestamp     time.Time
    Loop          string
    Iteration     int
    Profile       string
    TriggerSource string  // "scheduled", "fmail:<id>", "queue:<id>"
    TriggerFrom   string  // Agent name if from message
    TriggerMsgID  string  // Original fmail message ID
    ExitCode      int
    Duration      time.Duration
    Summary       string
    GitChanges    []string
}
```

## Implementation Notes

### Trigger Sources

| Source | TriggerSource Format | TriggerFrom |
|--------|---------------------|-------------|
| Base prompt (scheduled) | `scheduled` | (empty) |
| fmail DM | `fmail:<msg-id>` | Sender agent |
| forge msg | `queue:<item-id>` | `forge-cli` or agent |
| Queue message | `queue:<item-id>` | Original sender |

### Loop Engine Changes (internal/loop/engine.go)

```go
type IterationContext struct {
    TriggerSource string
    TriggerFrom   string
    TriggerMsgID  string
}

func (l *Loop) runIteration() error {
    ctx := l.determineIterationContext()
    
    // ... run harness ...
    
    entry := LedgerEntry{
        // ... existing fields ...
        TriggerSource: ctx.TriggerSource,
        TriggerFrom:   ctx.TriggerFrom,
        TriggerMsgID:  ctx.TriggerMsgID,
    }
    
    l.ledger.Append(entry)
}

func (l *Loop) determineIterationContext() IterationContext {
    // Check if processing a queue item
    if l.currentQueueItem != nil {
        item := l.currentQueueItem
        if strings.HasPrefix(item.Source, "fmail:") {
            return IterationContext{
                TriggerSource: item.Source,
                TriggerFrom:   item.From,
                TriggerMsgID:  strings.TrimPrefix(item.Source, "fmail:"),
            }
        }
        return IterationContext{
            TriggerSource: "queue:" + item.ID,
            TriggerFrom:   item.From,
        }
    }
    
    // Default: scheduled iteration
    return IterationContext{
        TriggerSource: "scheduled",
    }
}
```

### Ledger Format Update

Markdown format:
```markdown
## 2026-01-11 15:30:00 - coder-1 (iteration 42)

**Profile:** claude-a
**Trigger:** fmail:20260111-152500-0001 (from: architect)
**Exit code:** 0
**Duration:** 45s

### Summary
Fixed authentication bug in login flow.
```

JSON format (for `forge ledger --json`):
```json
{
  "timestamp": "2026-01-11T15:30:00Z",
  "loop": "coder-1",
  "iteration": 42,
  "profile": "claude-a",
  "trigger_source": "fmail:20260111-152500-0001",
  "trigger_from": "architect",
  "trigger_msg_id": "20260111-152500-0001",
  "exit_code": 0,
  "duration_ms": 45000,
  "summary": "Fixed authentication bug in login flow."
}
```

### Query by Trigger

Enable querying ledger by trigger:

```bash
# Find all iterations triggered by a specific message
forge ledger --trigger-msg 20260111-152500-0001

# Find all iterations triggered by a specific agent
forge ledger --triggered-by architect

# Find scheduled vs triggered iterations
forge ledger --trigger-source scheduled
```

## Acceptance Criteria

- [ ] LedgerEntry includes TriggerSource, TriggerFrom, TriggerMsgID fields
- [ ] Scheduled iterations marked as `scheduled`
- [ ] fmail-triggered iterations include message ID and sender
- [ ] Queue-triggered iterations include queue item source
- [ ] Markdown ledger shows trigger info
- [ ] JSON ledger includes trigger fields
- [ ] `forge ledger --trigger-msg <id>` query works

## Why This Matters

Enables:
- **Audit trail**: "Who asked for this change?"
- **Debugging**: "Which message caused this error?"
- **Metrics**: "How many iterations were triggered by agent X?"
- **Correlation**: Link fmail request → ledger entry → git changes

## Related

- f-1e51: Bridge forge msg + fmail (source of trigger messages)
- f-89ec: Loop events (events include trigger_msg_id)
