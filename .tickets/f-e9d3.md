---
id: f-e9d3
status: closed
deps: [f-c44a, f-cd1f, f-e081, f-5633, f-bfec, f-76d0, f-074a, f-e441, f-c648]
links: []
created: 2026-01-10T20:10:57Z
type: task
priority: 2
assignee: Tormod Haugland
parent: f-c2d0
---
# Add integration tests for fmail (standalone + connected)

Add automated coverage for Forge Mail behavior so we can iterate safely.

Scope:
- Standalone mode integration tests:
  - send -> log returns message
  - watch receives new messages
  - dm inbox handling
  - --since filtering
- Connected mode integration tests:
  - start forged mail server in-process
  - fmail client send/watch end-to-end

Notes:
- Use temp directories for project roots.
- Include at least one concurrency test for message ID collisions.

## Acceptance Criteria

- Tests cover standalone send/log/watch and DM inbox
- Tests cover connected send/watch (server + client)
- Tests are stable (no flakiness from timing; use deterministic timeouts)


## Notes

**2026-01-11T08:53:20Z**

Added fmail integration tests for standalone send/log/watch/DM/since and connected send/watch via unix socket; included concurrent SaveMessage ID uniqueness.
