---
id: swarm-qwvc
status: closed
deps: []
links: []
created: 2025-12-27T08:53:06.839129128+01:00
type: task
priority: 2
---
# Make every operation auditable

Ensure all operations have complete audit trails.

## From UX_FEEDBACK_2.md - Minor Issues

## What to Audit
1. **Every injected keystroke/message**
   - ID, timestamp, agent, workspace
   - Full message content
   - Source (user, queue, scheduler)

2. **Every pause inserted**
   - Reason: cooldown, account rotation, manual, policy
   - Duration
   - Who/what requested it

3. **Every state transition**
   - From state, to state
   - Reason/trigger
   - Evidence

4. **Every queue operation**
   - Enqueue, dispatch, complete, fail, cancel
   - Who initiated
   - Queue position at time

## Audit Log Format
```go
type AuditEntry struct {
    ID          string
    Timestamp   time.Time
    Operation   string
    EntityType  string // agent, queue, workspace
    EntityID    string
    Actor       string // user, scheduler, system
    Details     json.RawMessage
    Outcome     string // success, failure
}
```

## Storage
- Write to events table with `audit.` prefix
- Queryable via `swarm audit` command
- Exportable for analysis

## CLI Commands
- `swarm audit --agent <id> --since 1h`
- `swarm audit --workspace <ws> --operation dispatch`
- `swarm audit export --format json


