---
id: swarm-wqwi
status: closed
deps: []
links: []
created: 2025-12-27T08:54:18.441054665+01:00
type: task
priority: 2
---
# Wire TUI to live state engine

Connect TUI to actual state engine instead of sample data.

## From UX_FEEDBACK_1.md
`internal/cli/ui.go` launches the TUI with `StateEngine: nil`, so it renders
mostly sample/static data.

## Current Code
```go
// internal/cli/ui.go
app := tui.NewApp(tui.AppConfig{
    StateEngine: nil, // BAD: no live data
})
```

## Fix
```go
// internal/cli/ui.go
stateEngine := state.NewEngine(...)
app := tui.NewApp(tui.AppConfig{
    StateEngine: stateEngine, // GOOD: live data
})
```

## Changes Required
1. Initialize state engine in TUI launch
2. Start state poller
3. Connect event stream to TUI updates
4. Update all TUI components to use live data

## TUI Should Answer in 2 Seconds
- What is working?
- What is stuck?
- What needs my permission?
- What is cooling down?

## Real-time Updates
- Subscribe to event stream
- Update agent cards on state change
- Update queue counts
- Show alerts immediately


