---
id: swarm-zy5h
status: closed
deps: [swarm-18rc]
links: []
created: 2025-12-28T12:21:58.950930913+01:00
type: task
priority: 1
parent: swarm-wc7o
---
# Ensure graceful shutdown stops scheduler before gRPC server

Ensure proper shutdown order to prevent race conditions and data corruption. The scheduler should stop first (finish any in-progress dispatches), then the state poller, then the gRPC server.

**File**: internal/forged/daemon.go

**Dependencies**: Task 1.3 (scheduler running)

**Changes Required**:
1. Implement ordered shutdown in Run(): scheduler -> poller -> eventWatcher -> gRPC -> database
2. Add debug logging for each shutdown step
3. Ensure scheduler.Stop() waits for in-progress work
4. Add reasonable timeout to prevent hanging

**Testing**:
- Send SIGINT during active dispatch, verify dispatch completes
- Verify no 'database is closed' errors during shutdown
- Verify all goroutines exit cleanly (no leaks)
- Test with go test -race

See docs/design/scheduler-daemon-tasks.md#task-15 for full details.


