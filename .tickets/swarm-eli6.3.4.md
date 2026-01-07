---
id: swarm-eli6.3.4
status: closed
deps: [swarm-eli6.1.2]
links: []
created: 2025-12-22T10:17:12.841683975+01:00
type: task
priority: 1
parent: swarm-eli6.3
---
# Support multi-line input for `agent send`

Scope:
Add multi-line message input for `swarm agent send`:
- `--file` reads from file
- `--stdin` reads from stdin
- optional `--editor` to open $EDITOR

UX goal:
Make sending complex prompts ergonomic and consistent with queue file behavior.

## Acceptance Criteria

- messages preserve newlines
- JSON output includes raw message content
- file + stdin errors are actionable


