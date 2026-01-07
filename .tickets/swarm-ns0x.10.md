---
id: swarm-ns0x.10
status: closed
deps: []
links: []
created: 2026-01-06T20:01:58.857923337+01:00
type: task
priority: 1
parent: swarm-ns0x
---
# Implement pool selection + cooldown/concurrency enforcement

Implement pool-based profile selection and availability rules (PRD section 10).

Scope:
- Selection strategy (round-robin default; allow future LRU/weighted).
- Enforce max_concurrency per profile and cooldown timers.
- Pinned profile errors when unavailable; pooled loops wait until availability.

Acceptance:
- Pool selection is deterministic and respects concurrency caps.
- Cooldown blocking sets loop to waiting state with next-available timestamp.
- CLI surfaces waiting status in `forge ps`.



