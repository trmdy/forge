---
id: swarm-eli6.3.3
status: closed
deps: [swarm-eli6.1.2]
links: []
created: 2025-12-22T10:17:12.774113822+01:00
type: task
priority: 1
parent: swarm-eli6.3
---
# Implement structured JSON error output

Scope:
Standardize errors when `--json` or `--jsonl` is set. Use a consistent error envelope (code, message, hint, details).

## Acceptance Criteria

- structured JSON error output for CLI
- non-zero exit codes preserved
- human mode errors remain concise


