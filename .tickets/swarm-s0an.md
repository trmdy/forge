---
id: swarm-s0an
status: closed
deps: [swarm-ilty]
links: []
created: 2025-12-28T12:24:08.690812849+01:00
type: task
priority: 1
parent: swarm-wc7o
---
# Optionally skip polling for agents with active SSE connections

As an optimization, skip polling for agents that have active SSE connections since SSE provides more timely and accurate state information.

**Files**: internal/state/poller.go, internal/forged/daemon.go

**Changes Required**:
1. Add SSEWatchChecker interface to poller (IsWatching method)
2. Add SSEWatcher and SkipSSEWatchedAgents to PollerConfig
3. Modify shouldPoll() to skip if SSE watching is active
4. Wire up in forged: pollerConfig.SSEWatcher = eventWatcher
5. Make behavior configurable

**Testing**:
- Agent with SSE connection -> polling skipped
- Agent without SSE connection -> polling continues
- SSE connection drops -> polling resumes
- Config can disable this behavior

See docs/design/scheduler-daemon-tasks.md#task-33 for full details.


