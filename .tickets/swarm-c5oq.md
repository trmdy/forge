---
id: swarm-c5oq
status: closed
deps: []
links: []
created: 2025-12-28T12:20:59.262663786+01:00
type: task
priority: 0
parent: swarm-wc7o
---
# Add database connection and repositories to forged daemon

Add database connectivity to forged daemon. Currently forged is stateless - it tracks agents in an in-memory map. To run the scheduler, we need access to the SQLite database where agents, queues, and workspaces are persisted.

**File**: internal/forged/daemon.go

**Changes Required**:
1. Add database, agentRepo, queueRepo, wsRepo, eventRepo, nodeRepo, portRepo fields to Daemon struct
2. Open database in New() using cfg.Database.Path
3. Run migrations automatically
4. Create all repositories
5. Add Close() method to clean up database on shutdown
6. Add DisableDatabase option for testing

**Testing**:
- Verify forged starts with valid database path
- Verify forged fails gracefully with invalid path
- Verify database closes on shutdown
- Verify migrations run on first start

See docs/design/scheduler-daemon-tasks.md#task-11 for full details.


