---
id: swarm-okhc
status: closed
deps: [swarm-c5oq]
links: []
created: 2025-12-28T12:21:13.928418816+01:00
type: task
priority: 0
parent: swarm-wc7o
---
# Initialize StateEngine, AgentService, QueueService in forged

Create the service layer that the scheduler needs. These services wrap the repositories and provide business logic for agent management, queue operations, and state tracking.

**File**: internal/forged/daemon.go

**Dependencies**: Task 1.1 (database connection)

**Changes Required**:
1. Add tmuxClient, stateEngine, agentService, queueService, wsService, nodeService fields to Daemon
2. Create tmux.NewLocalClient()
3. Create adapters.NewRegistry()
4. Initialize all services with proper dependencies
5. Configure agentService with EventRepository, PortRepository, ArchiveDir options

**Testing**:
- Verify all services initialize without error
- Verify services can perform basic operations
- Integration test: spawn agent via CLI, verify forged can see it

See docs/design/scheduler-daemon-tasks.md#task-12 for full details.


