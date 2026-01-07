---
id: swarm-eli6.3.1
status: closed
deps: [swarm-eli6.1.2]
links: []
created: 2025-12-22T10:17:12.650230385+01:00
type: task
priority: 1
parent: swarm-eli6.3
---
# Refactor human CLI output to structured tables

Scope:
Refactor human-readable output across node/ws/agent/queue commands to use a shared formatter. Use tables with consistent columns, stable ordering, and explicit labels.

## Acceptance Criteria

- node/ws/agent list + status output match CLI style guide
- consistent headers and spacing
- long values truncate deterministically


