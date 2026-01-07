---
id: swarm-eli6.2.4
status: closed
deps: [swarm-eli6.1.2]
links: []
created: 2025-12-22T10:17:12.526321259+01:00
type: task
priority: 1
parent: swarm-eli6.2
---
# Add short-ID resolution + suggestions

Scope:
Allow short ID prefixes and fuzzy name matching for node/workspace/agent commands.

UX goal:
Reduce copy/paste friction. When ambiguous, show a short suggestion list.

## Acceptance Criteria

- prefix matching for IDs in CLI
- ambiguous matches return a short list and ask for clarification
- errors include example input format


