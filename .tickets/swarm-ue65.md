---
id: swarm-ue65
status: closed
deps: [swarm-6q4u]
links: []
created: 2025-12-28T12:22:30.767794598+01:00
type: task
priority: 0
parent: swarm-wc7o
---
# Wire onState handler to update StateEngine (persists to DB)

Connect the SSE event watcher to the state engine so that events from OpenCode update the database. When we receive session.idle or session.busy, the agent's state in the database should be updated immediately with high confidence.

**File**: internal/forged/daemon.go

**Dependencies**: Task 2.1 (watcher created)

**Changes Required**:
1. Create onStateUpdate function that calls stateEngine.UpdateState()
2. Pass handler to NewOpenCodeEventWatcher()
3. Add timeout context for state updates (5s)
4. Add logging for state changes and errors

The SSE events already map to StateConfidenceHigh in opencode_events.go mapEventToState().

**Testing**:
- Mock SSE server, send session.idle event, verify DB is updated
- Verify state confidence is High
- Verify error handling when StateEngine fails
- Verify logging of state changes

See docs/design/scheduler-daemon-tasks.md#task-22 for full details.


