---
id: swarm-ns0x.6
status: closed
deps: []
links: []
created: 2026-01-06T20:01:10.165056817+01:00
type: task
priority: 1
parent: swarm-ns0x
---
# Implement profile + pool CLI management

Add `forge profile` and `forge pool` command groups per PRD section 6.3.

Scope:
- profile: ls/add/edit/rm/doctor/cooldown set/clear.
- pool: create/add/ls/show/set-default, with selection strategy and max_concurrency.
- Pools accept profile references across harness/auth combinations; profiles can be in multiple pools.

Acceptance:
- CLI supports JSON and human output.
- Cooldown and max_concurrency enforced during selection.
- Default pool used when loops are not pinned.



