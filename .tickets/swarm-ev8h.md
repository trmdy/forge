---
id: swarm-ev8h
status: closed
deps: [swarm-18rc]
links: []
created: 2025-12-28T12:21:44.018404954+01:00
type: task
priority: 1
parent: swarm-wc7o
---
# Add scheduler config options to forged (--no-scheduler, intervals)

Add command-line flags and configuration options to control scheduler behavior. Users should be able to disable the scheduler entirely (for gRPC-only mode) or tune its parameters.

**Files**: cmd/forged/main.go, internal/forged/daemon.go, internal/config/config.go

**Dependencies**: Task 1.3 (scheduler running)

**Changes Required**:
1. Add flags: --no-scheduler, --scheduler-tick, --no-state-poller to cmd/forged/main.go
2. Add SchedulerEnabled, SchedulerTickInterval, StatePollerEnabled to Options struct
3. Apply options conditionally in New()
4. Support config.yaml scheduler section

**Testing**:
- forged --no-scheduler starts without scheduler
- forged --scheduler-tick 5s uses custom interval
- Config file settings are applied
- Command-line flags override config file

See docs/design/scheduler-daemon-tasks.md#task-14 for full details.


