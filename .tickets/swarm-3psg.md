---
id: swarm-3psg
status: closed
deps: []
links: []
created: 2025-12-27T09:33:58.27573862+01:00
type: task
priority: 1
---
# Ensure single scheduler loop owns queue dispatch (kill hidden concurrency)

Eliminate hidden concurrency by ensuring one scheduler loop owns queue dispatch.

## From UX_FEEDBACK_1.md - Minor Issues #2

## Problem
Hidden concurrency leads to race conditions and unpredictable behavior.

## Solution

### 1. Single scheduler loop per workspace (or per agent)
```go
type SchedulerLoop struct {
    workspaceID string
    ticker      *time.Ticker
    stopCh      chan struct{}
}

func (s *SchedulerLoop) Run(ctx context.Context) {
    for {
        select {
        case <-s.ticker.C:
            s.tick(ctx)
        case <-s.stopCh:
            return
        case <-ctx.Done():
            return
        }
    }
}
```

### 2. All writes through state engine API
- No direct DB writes for queue operations
- All mutations go through transactional state engine
- Clear ownership: scheduler owns dispatch, CLI owns enqueue

### 3. Mutex/serialization for critical sections
- Queue dispatch is atomic
- State transitions are serialized
- No two dispatches to same agent simultaneously

## Verification
- Add test: concurrent dispatch attempts → only one succeeds
- Add test: rapid enqueue during dispatch → queue consistent
- Log warnings if concurrent access detected


