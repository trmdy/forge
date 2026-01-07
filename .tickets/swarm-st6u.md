---
id: swarm-st6u
status: closed
deps: [swarm-m0hn]
links: []
created: 2025-12-28T12:23:02.465161028+01:00
type: task
priority: 0
parent: swarm-wc7o
---
# Subscribe to agent terminate events - stop SSE watching

When an agent is terminated or stopped, stop watching its SSE endpoint to clean up resources and avoid connection errors.

**File**: internal/forged/daemon.go

**Dependencies**: Task 2.3 (auto-start watching)

**Changes Required**:
1. Extend the state change subscriber to watch for Stopped/Error states
2. Implement maybeStopWatching(agentID) helper
3. Check IsWatching before calling Unwatch
4. Add logging for watch stop

**Testing**:
- Terminate agent via CLI, verify forged stops watching
- Kill agent forcefully, verify watching stops
- Verify error state triggers watch stop
- Verify graceful handling of already-stopped watches

See docs/design/scheduler-daemon-tasks.md#task-24 for full details.


