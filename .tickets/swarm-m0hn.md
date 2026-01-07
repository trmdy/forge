---
id: swarm-m0hn
status: closed
deps: [swarm-ue65]
links: []
created: 2025-12-28T12:22:46.556219115+01:00
type: task
priority: 0
parent: swarm-wc7o
---
# Subscribe to agent spawn events - auto-start SSE watching

When a new OpenCode agent is spawned (either via forged or CLI), automatically start watching its SSE endpoint. This ensures we don't miss state updates for newly created agents.

**File**: internal/forged/daemon.go

**Dependencies**: Task 2.2 (watcher wired to state engine)

**Changes Required**:
1. Subscribe to stateEngine events in Run() using SubscribeFunc
2. Watch for agents transitioning from Starting to Idle
3. Implement maybeStartWatching(ctx, agentID) helper
4. Check agent.HasOpenCodeConnection() before watching
5. Skip if already watching (IsWatching check)

**Testing**:
- Spawn OpenCode agent via CLI, verify forged starts watching
- Verify non-OpenCode agents are not watched
- Verify duplicate watch attempts are ignored
- Verify agents with incomplete connection info are skipped

See docs/design/scheduler-daemon-tasks.md#task-23 for full details.


