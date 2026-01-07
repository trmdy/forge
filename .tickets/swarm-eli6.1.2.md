---
id: swarm-eli6.1.2
status: closed
deps: []
links: []
created: 2025-12-22T10:17:12.18342422+01:00
type: task
priority: 1
parent: swarm-eli6.1
---
# Define CLI output style guide

Scope:
Define a CLI output style guide: naming conventions, column order, truncation rules, status icons/colors, and error formatting (human + JSON).

Rationale:
The CLI is the primary interface today. Inconsistent output makes automation brittle and human usage frustrating.

## Acceptance Criteria

- docs/ux/cli-style.md describing:\n  - table layouts for node/ws/agent/queue\n  - status icon + color semantics\n  - rules for truncation and ID display\n  - JSON error envelope schema\n- aligns naming with docs/ux/terminology.md\n- guidance for --no-color and NO_COLOR


