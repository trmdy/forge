---
id: swarm-hp02
status: closed
deps: []
links: []
created: 2025-12-27T09:32:29.884392923+01:00
type: task
priority: 1
---
# Make tmux/ssh operations idempotent and safe to retry

Ensure all tmux and ssh operations are idempotent to prevent "it worked yesterday" bugs.

## From UX_FEEDBACK_1.md - Phase 2

## Current Problem
Tmux/ssh operations are sprinkled around without consistent idempotency guarantees.

## Deliverables

### 1. Strict interface boundary
```go
type TmuxTransport interface {
    CreateSession(name string) error      // Idempotent - checks existence
    CreatePane(session, name string) error // Idempotent - checks target exists
    SendKeys(pane string, keys string) error
    CapturePane(pane string) (string, error)
    ListPanes(session string) ([]Pane, error)
}

type SSHTransport interface {
    RunCommand(cmd string) (string, error)
    CopyFile(local, remote string) error
    CheckBinaryVersion(binary string) (string, error)
}
```

### 2. Idempotent operations
- `CreateSession`: checks existence first, returns success if exists
- `CreatePane`: checks target session exists, returns success if pane exists
- `SendKeys`: logs exact keys sent for debugging
- All operations: include verification step where possible

### 3. Error handling
- Clear error messages with context
- Retry-safe (calling twice produces same result)
- Timeout handling for SSH operations

## Verification
- Add integration tests for idempotency
- Test: create session twice → no error
- Test: create pane in non-existent session → clear error


