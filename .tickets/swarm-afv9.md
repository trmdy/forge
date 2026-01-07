---
id: swarm-afv9
status: closed
deps: []
links: []
created: 2025-12-27T08:51:54.728513669+01:00
type: bug
priority: 0
---
# Fix state engine to pass adapter metadata

State engine currently passes nil to DetectState, preventing native integrations.

## Bug Description (from UX_FEEDBACK_1.md)
`AgentAdapter.DetectState(screen, meta any)` supports richer detection, but
`internal/state/engine.go` calls `DetectState(screen, nil)`.

This is a straight bug/unfinished wiring that prevents "native" integrations.

## Current Code
```go
// internal/state/engine.go
state, _ := adapter.DetectState(screen, nil) // BAD: always nil
```

## Fix
```go
// internal/state/engine.go
state, _ := adapter.DetectState(screen, agent.Metadata) // GOOD: pass metadata
```

## Impact
- OpenCode adapter can use metadata to check API endpoints
- Claude Code adapter can use metadata for session info
- All adapters get richer context for state detection

## Verification
- Add test that metadata is passed
- Verify OpenCode adapter receives metadata
- Check that existing adapters handle nil gracefully


