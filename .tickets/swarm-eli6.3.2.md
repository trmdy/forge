---
id: swarm-eli6.3.2
status: closed
deps: [swarm-eli6.3.1]
links: []
created: 2025-12-22T10:17:12.710611274+01:00
type: task
priority: 1
parent: swarm-eli6.3
---
# Add color + icon semantics to CLI output

Scope:
Add optional color + icon semantics to human output (e.g., ✓ idle, ⟳ working, ⚠ approval). Respect `NO_COLOR` and `--no-color`.

## Acceptance Criteria

- color is disabled in JSON/JSONL modes
- `--no-color` flag and `NO_COLOR` env var both supported
- icons/colors match CLI style guide


