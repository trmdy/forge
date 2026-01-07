---
id: swarm-ns0x.13
status: closed
deps: []
links: []
created: 2026-01-06T20:02:28.917745152+01:00
type: task
priority: 2
parent: swarm-ns0x
---
# Build minimal loop TUI (2x2 grid + log panes)

Implement the minimal TUI view for loops per PRD section 8 and clarified scope.

Scope:
- 2x2 grid of loop cards, each with log tail pane.
- Pagination/tabs for more than 4 loops.
- Shows loop name, profile/pool, state, queue length, last run.

Acceptance:
- `forge`/`forge tui` launches TUI and lists running loops.
- Logs display correctly from centralized log files.



