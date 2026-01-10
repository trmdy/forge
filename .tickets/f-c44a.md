---
id: f-c44a
status: closed
deps: []
links: []
created: 2026-01-10T20:06:39Z
type: task
priority: 0
assignee: Tormod Haugland
parent: f-c2d0
---
# Implement fmail core (root detection, IDs, validation, file store)

Implement the core Forge Mail primitives that both fmail (CLI) and forged (server) will use.

Scope:
- Project root discovery:
  - If FMAIL_ROOT is set, use it as the project root
  - Walk up for .fmail/ first
  - Else walk up for .git/ (git root becomes project root)
  - Else use current directory
- Project ID derivation:
  - FMAIL_PROJECT override
  - Else from git remote URL (stable across clones)
  - Else fallback from directory name
- Agent identity:
  - FMAIL_AGENT if set
  - Else prompt or generate anon-<pid> (define non-interactive behavior)
- Topic and agent name validation:
  - Topics: lowercase [a-z0-9-]
  - Direct messages use @prefix
- Message model + JSON encoding/decoding:
  - Required fields: id, from, to, time, body
  - Optional: reply_to, priority, host
- Message ID generation + atomic write:
  - Format YYYYMMDD-HHMMSS-NNNN
  - Use O_EXCL to avoid collisions; retry on exist
  - Enforce 1MB message size limit
- File store layout under .fmail/:
  - topics/<topic>/<id>.json
  - dm/<agent>/<id>.json
  - agents/<agent>.json
  - project.json

References:
- docs/forge-mail/SPEC.md (Message Format, Storage Layout)
- docs/forge-mail/DESIGN.md (edge cases, concurrency)

## Acceptance Criteria

- Core package exposes APIs used by CLI commands (send/log/watch/etc) without depending on forge config
- ID generation matches spec and is collision-resistant under concurrent writers (unit test)
- Topic/agent validation matches spec (unit test)
- Store can write/read/list messages for a topic and for a DM inbox (unit test)
- 1MB guard enforced consistently (unit test)


## Notes

**2026-01-10T20:55:22Z**

Implemented core fmail package: root/project discovery helpers, validation, message IDs, store with size guard + tests.
