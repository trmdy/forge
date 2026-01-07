---
id: swarm-toqx
status: closed
deps: []
links: []
created: 2025-12-28T12:23:35.256060197+01:00
type: task
priority: 0
parent: swarm-wc7o
---
# Verify SSE events have StateConfidenceHigh - add tests

Verify that SSE events are already mapped to high-confidence states and add unit tests to ensure this remains true.

**File**: internal/adapters/opencode_events_test.go

**Verification needed in opencode_events.go mapEventToState()**:
- session.idle -> StateConfidenceHigh
- session.busy -> StateConfidenceHigh  
- permission.requested -> StateConfidenceHigh
- error -> StateConfidenceHigh

**Changes Required**:
1. Add unit test TestSSEEventsHaveHighConfidence
2. Test all event types that map to states
3. Document confidence levels in code comments

**Testing**:
- All SSE events return StateConfidenceHigh
- Test passes and prevents regression

See docs/design/scheduler-daemon-tasks.md#task-31 for full details.


