---
id: swarm-83sb.9
status: closed
deps: []
links: []
created: 2025-12-27T09:33:21.365023919+01:00
type: task
priority: 2
parent: swarm-83sb
---
# Add top-level swarm attach command

Add ergonomic top-level attach command for quick tmux access.

## From UX_FEEDBACK_2.md - Phase 1

## Command
```bash
swarm attach                 # Attach to context agent/workspace
swarm attach <agent>         # Attach to specific agent pane
swarm attach <workspace>     # Attach to workspace session
swarm attach --select        # Interactive selection
```

## Behavior
1. If agent specified: focus that agents tmux pane
2. If workspace specified: attach to workspace tmux session
3. If neither: use context, or prompt for selection

## Implementation
- Alias/wrapper around `swarm ws attach` and `swarm agent attach`
- Add context resolution
- Add interactive selection when ambiguous

## UX Goal
Quick access to any agent or workspace with minimal typing.


