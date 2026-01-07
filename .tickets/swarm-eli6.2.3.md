---
id: swarm-eli6.2.3
status: closed
deps: [swarm-eli6.1.2, swarm-j39n]
links: []
created: 2025-12-22T10:17:12.466437365+01:00
type: task
priority: 1
parent: swarm-eli6.2
---
# Standardize confirmations + `--yes` for destructive actions

Standardize destructive-action confirmations across CLI commands (node remove, ws kill, agent terminate, queue clear). Use global --yes/non-interactive mode (swarm-j39n) for automation; prompts should include resource name/id and impact summary.

## Acceptance Criteria

- confirm prompts are consistent and include resource name/id
- `--yes` bypasses prompts
- JSON mode returns deterministic output without interactive prompt


