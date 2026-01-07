---
id: swarm-ai5f
status: closed
deps: []
links: []
created: 2025-12-27T08:52:13.454682893+01:00
type: task
priority: 1
---
# Implement deterministic scheduler with testable tick function

Replace heuristic scheduling with a deterministic queue engine.

## From UX_FEEDBACK_2.md - Phase 3

## Current Problem
Scheduling decisions are scattered and hard to test.

## Solution
Create a single "scheduler tick" function that is easy to unit test.

## Interface
```go
type SchedulerInput struct {
    Agents     []AgentSnapshot
    QueueItems []QueueItemSnapshot
    Accounts   []AccountSnapshot
    Now        time.Time
}

type SchedulerAction struct {
    Type    ActionType // dispatch, pause, cooldown_start, etc
    AgentID string
    ItemID  string
    Reason  string
}

func Tick(input SchedulerInput) []SchedulerAction {
    // Pure function - no side effects
    // Returns list of actions to take
}
```

## Scheduling Rules
1. Only dispatch to IDLE agents
2. Respect account cooldowns
3. Handle conditional items (requires_idle, etc)
4. Process queue in order (respecting priority)
5. Emit detailed reasons for blocking

## Test Cases
- "dispatch only when idle"
- "insert cooldown after N messages"
- "dont dispatch if WAITING_PERMISSION unless message is permission-grant"
- "handle queue priority correctly"
- "conditional items wait for conditions"

## Integration
- Run tick on configurable interval (default 1s)
- Apply returned actions to DB and agents
- Emit events for all actions taken


