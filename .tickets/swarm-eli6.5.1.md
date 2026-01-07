---
id: swarm-eli6.5.1
status: closed
deps: [swarm-eli6.1.2]
links: []
created: 2025-12-22T10:17:13.290272779+01:00
type: task
priority: 1
parent: swarm-eli6.5
---
# Update CLI docs + quickstart to match reality

Update docs/cli.md and docs/quickstart.md to reflect implemented commands and flags (node/ws/agent, swarm init, swarm status when shipped). Remove planned labels for shipped commands and add examples that match actual output.

## Acceptance Criteria

- docs match current CLI behavior
- examples use real commands and outputs
- includes note about JSON/JSONL and `--watch`


