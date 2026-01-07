---
id: swarm-ilty
status: closed
deps: [swarm-ue65]
links: []
created: 2025-12-28T12:23:52.731667664+01:00
type: task
priority: 0
parent: swarm-wc7o
---
# StateEngine.UpdateState should respect confidence - don't downgrade

Modify StateEngine.UpdateState() to not overwrite high-confidence state with low-confidence state, unless the state actually changes. This prevents polling from undoing SSE updates.

**File**: internal/state/engine.go

**Changes Required**:
1. Add confidenceRank() helper function (High=3, Medium=2, Low=1)
2. In UpdateState(), check if state is the same
3. If same state, only update if new confidence >= current confidence
4. Add debug logging for skipped updates
5. State changes always succeed regardless of confidence

**Testing**:
- SSE sets idle (high), polling tries working (low) -> stays idle
- SSE sets idle (high), SSE sets working (high) -> changes to working
- Polling sets idle (low), SSE sets working (high) -> changes to working
- State changes always succeed regardless of confidence
- Unit tests cover all confidence combinations

See docs/design/scheduler-daemon-tasks.md#task-32 for full details.


