---
id: swarm-18rc
status: closed
deps: [swarm-okhc]
links: []
created: 2025-12-28T12:21:29.294107336+01:00
type: task
priority: 0
parent: swarm-wc7o
---
# Initialize and start Scheduler in forged.Run()

Create and start the scheduler that will dispatch queued messages to agents. The scheduler runs a tick loop that checks for idle agents with pending queue items and dispatches them.

**File**: internal/forged/daemon.go

**Dependencies**: Task 1.2 (services)

**Changes Required**:
1. Add scheduler and statePoller fields to Daemon
2. Create state.NewPoller() with DefaultPollerConfig
3. Create scheduler.New() with config from cfg.Scheduler
4. Start statePoller in Run() before gRPC server
5. Start scheduler in Run() after poller
6. Add proper shutdown order in select block

**Testing**:
- Start forged, queue a message via CLI, verify it gets dispatched
- Verify scheduler tick interval is configurable
- Verify scheduler stops cleanly on shutdown
- Verify scheduler doesn't dispatch to busy agents

See docs/design/scheduler-daemon-tasks.md#task-13 for full details.


