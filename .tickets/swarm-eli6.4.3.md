---
id: swarm-eli6.4.3
status: closed
deps: [swarm-9l9w, swarm-fl8, swarm-eli6.1.3]
links: []
created: 2025-12-22T10:17:13.157802614+01:00
type: task
priority: 1
parent: swarm-eli6.4
---
# Add TUI loading/refresh indicators

Scope:
Add loading + refresh indicators:
- show last refresh time
- subtle spinner when data refreshes
- staleness indicator if updates stop

## Acceptance Criteria

- refresh indicator visible on dashboard
- staleness warning after 2x refresh interval
- integrates with existing `swarm-9l9w` task for timestamps


