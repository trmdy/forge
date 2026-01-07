---
id: swarm-kixg
status: closed
deps: [swarm-m0hn]
links: []
created: 2025-12-28T12:23:19.554326242+01:00
type: task
priority: 1
parent: swarm-wc7o
---
# On forged startup, start watching all existing OpenCode agents

When forged starts (or restarts), it should resume watching all existing OpenCode agents that are still running. This handles the case where forged was restarted but agents are still active.

**File**: internal/forged/daemon.go

**Dependencies**: Task 2.3 (maybeStartWatching implemented)

**Changes Required**:
1. Implement startWatchingExistingAgents(ctx) method
2. List all agents from agentRepo
3. Skip Stopped/Error state agents
4. Skip non-OpenCode agents (check HasOpenCodeConnection)
5. Call eventWatcher.WatchAgent for each eligible agent
6. Call in Run() after services are ready, before gRPC server

**Testing**:
- Start agents, restart forged, verify watching resumes
- Verify stopped agents are not watched
- Verify connection errors are handled gracefully
- Verify startup completes even if some agents fail to watch

See docs/design/scheduler-daemon-tasks.md#task-25 for full details.


