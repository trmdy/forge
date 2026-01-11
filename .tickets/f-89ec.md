---
id: f-89ec
status: open
deps: []
links: []
created: 2026-01-11T17:51:05Z
type: feature
priority: 3
assignee: Tormod Haugland
parent: f-4c08
---
# Emit loop events to fmail topic

## Problem

Loop state changes are invisible to the fmail ecosystem:
- No way for external agents to know when a loop starts/stops
- No notification when a loop errors or enters cooldown
- Can't build coordination patterns (e.g., "notify me when loop finishes")

Currently, loop state is only visible via:
- `forge ps` (polling, CLI only)
- TUI (interactive, local only)
- Event log (requires DB access)

## Solution

Loops emit lifecycle events to a reserved fmail topic `forge-events`:

```json
{
  "id": "20260111-153000-0001",
  "from": "loop:coder-1",
  "to": "forge-events",
  "body": {
    "event": "iteration_complete",
    "loop": "coder-1",
    "iteration": 42,
    "exit_code": 0,
    "duration_ms": 45000,
    "trigger_msg_id": "20260111-152500-0001",
    "ledger_summary": "Fixed auth bug, created PR #42"
  },
  "tags": ["loop", "coder-1", "complete"]
}
```

### Event Types

| Event | When | Key Fields |
|-------|------|------------|
| `loop_start` | Loop begins | loop, pool, profile |
| `iteration_start` | Iteration begins | loop, iteration, trigger_msg_id |
| `iteration_complete` | Iteration ends | loop, iteration, exit_code, duration_ms, ledger_summary |
| `loop_stop` | Loop stops (graceful) | loop, reason, total_iterations |
| `loop_error` | Loop encounters error | loop, error, will_retry |
| `loop_cooldown` | Loop enters cooldown | loop, profile, until, reason |

## Implementation Notes

### Event Publisher (internal/loop/events.go)

```go
type LoopEventPublisher struct {
    store    *mail.Store
    loopName string
}

func (p *LoopEventPublisher) EmitIterationComplete(iter *Iteration) error {
    event := LoopEvent{
        Event:         "iteration_complete",
        Loop:          p.loopName,
        Iteration:     iter.Number,
        ExitCode:      iter.ExitCode,
        DurationMs:    iter.Duration.Milliseconds(),
        TriggerMsgID:  iter.TriggerMsgID,
        LedgerSummary: iter.LedgerSummary,
    }
    
    msg := mail.Message{
        From: "loop:" + p.loopName,
        To:   "forge-events",
        Body: event,
        Tags: []string{"loop", p.loopName, "complete"},
    }
    
    return p.store.Send("forge-events", msg)
}
```

### Loop Engine Integration (internal/loop/engine.go)

```go
func (l *Loop) runIteration() error {
    l.events.EmitIterationStart(l.iteration)
    
    // ... run harness ...
    
    result := l.harness.Run(ctx, prompt)
    
    l.events.EmitIterationComplete(&Iteration{
        Number:        l.iteration,
        ExitCode:      result.ExitCode,
        Duration:      result.Duration,
        TriggerMsgID:  l.currentTriggerMsgID,
        LedgerSummary: l.extractLedgerSummary(result),
    })
    
    return nil
}

func (l *Loop) enterCooldown(reason string, until time.Time) {
    l.events.EmitCooldown(reason, until)
    // ... existing cooldown logic ...
}
```

### Watching Events

External agents can subscribe to loop events:

```bash
# Watch all loop events
fmail watch forge-events

# Filter by loop (using tags once implemented)
fmail watch forge-events --tag coder-1

# Wait for specific loop to complete
fmail watch forge-events --json | jq 'select(.body.loop == "coder-1" and .body.event == "iteration_complete")'
```

### Event Retention

`forge-events` can get noisy. Recommendations:
- Aggressive gc: `fmail gc --topic forge-events --days 1`
- Or configure per-topic retention in forged

### Configuration

```yaml
# .forge/forge.yaml
events:
  enabled: true
  topic: forge-events  # Customizable
  emit:
    - iteration_complete
    - loop_error
    - loop_cooldown
    # - iteration_start  # Disabled by default (too noisy)
```

## Acceptance Criteria

- [ ] Loops emit `iteration_complete` event after each iteration
- [ ] Loops emit `loop_start` and `loop_stop` events
- [ ] Loops emit `loop_error` on errors
- [ ] Loops emit `loop_cooldown` when entering cooldown
- [ ] Events include relevant metadata (iteration, duration, trigger_msg_id)
- [ ] Events tagged with loop name for filtering
- [ ] Can disable via config
- [ ] `forge-events` topic auto-created on first event

## Why This Matters

Enables:
- **External monitoring**: Agents can watch for errors/completions
- **Coordination**: "When coder-1 finishes, start reviewer-1"
- **Alerting**: Watch for `loop_error` events
- **Metrics**: Aggregate iteration durations, success rates
- **Debugging**: Trace loop behavior over time

## Related

- f-1e51: Bridge forge msg + fmail (loops become fmail citizens)
- f-b3a1: Ledger traceability (events include trigger_msg_id)
- f-54c2: Tags field (events use tags for filtering)
