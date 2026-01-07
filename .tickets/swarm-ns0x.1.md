---
id: swarm-ns0x.1
status: closed
deps: []
links: []
created: 2026-01-06T20:00:16.666960058+01:00
type: task
priority: 1
parent: swarm-ns0x
---
# Define loop runtime schema + repositories

Design and implement the new SQLite schema for loop runtime state per docs/simplification-prd.md (sections 4, 7, 10).

Scope:
- New tables for loops, profiles, pools, pool_members, queue_items (new types), loop_runs, cooldowns/availability.
- Repository layer + models for loops, profiles, pools, queue, run history.
- Store log path pointers and repo path for loops.

Acceptance:
- Migration(s) added with forward/backward support.
- Models + repositories compile and are covered by unit tests.
- Queue item types match PRD + clarified semantics (MessageAppend, NextPromptOverrideOnce, Pause, StopGraceful, KillNow, SteerMessage).



