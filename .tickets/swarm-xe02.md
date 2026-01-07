---
id: swarm-xe02
status: closed
deps: []
links: []
created: 2025-12-27T08:51:38.424704293+01:00
type: task
priority: 0
---
# Define canonical Agent State Machine

Establish a strict agent state machine as the foundation for all state tracking.

## From UX_FEEDBACK_2.md - Phase 0 Invariants

## Canonical States
```go
type AgentState string
const (
    AgentStateStarting          AgentState = "STARTING"
    AgentStateIdle              AgentState = "IDLE"
    AgentStateBusy              AgentState = "BUSY"
    AgentStateWaitingPermission AgentState = "WAITING_PERMISSION"
    AgentStateCooldown          AgentState = "COOLDOWN"
    AgentStateError             AgentState = "ERROR"
    AgentStateStopped           AgentState = "STOPPED"
)
```

## State Transition Rules
- Agent can only be in ONE state at a time
- All transitions emit events
- Invalid transitions panic in dev, log error in prod

## Canonical Events
- `agent.started`
- `agent.idle`
- `agent.busy`
- `agent.waiting_permission`
- `agent.cooldown_started`
- `agent.cooldown_ended`
- `agent.crashed`
- `agent.stopped`
- `queue.enqueued`
- `queue.dispatched`
- `queue.blocked`

## Queue Item States
- `pending` → `dispatched` → `acked/failed/canceled`
- Every dispatched message has audit trail

## Implementation
- Add state machine package: internal/state/machine.go
- Add transition validation
- Add state change event emission
- Update all state setters to use state machine


