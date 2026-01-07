---
id: swarm-6q4u
status: closed
deps: [swarm-okhc]
links: []
created: 2025-12-28T12:22:13.856369159+01:00
type: task
priority: 0
parent: swarm-wc7o
---
# Create OpenCodeEventWatcher in forged daemon initialization

Instantiate the OpenCodeEventWatcher in the forged daemon. This watcher manages SSE connections to multiple OpenCode agents and translates events into state updates.

**File**: internal/forged/daemon.go

**Dependencies**: Task 1.2 (StateEngine available)

**Changes Required**:
1. Add eventWatcher field to Daemon struct
2. Create adapters.NewOpenCodeEventWatcher() in New() with DefaultEventWatcherConfig
3. Pass nil for onState handler (will be wired in Task 2.2)
4. Add eventWatcher.UnwatchAll() to shutdown sequence

**Testing**:
- Verify eventWatcher is created successfully
- Verify eventWatcher is cleaned up on shutdown
- Verify no goroutine leaks

See docs/design/scheduler-daemon-tasks.md#task-21 for full details.


