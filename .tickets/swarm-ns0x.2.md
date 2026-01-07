---
id: swarm-ns0x.2
status: closed
deps: []
links: []
created: 2026-01-06T20:00:26.861170467+01:00
type: task
priority: 1
parent: swarm-ns0x
---
# Add profiles/pools/loop config sections

Update config loading/validation to support profiles, pools, loop defaults, and runtime paths (docs/simplification-prd.md section 7).

Scope:
- Extend config schema with top-level profiles/pools/default_pool/loop_defaults.
- Ensure global.data_dir is used for SQLite + centralized logs.
- Add config validation for prompt defaults, profile definitions, max_concurrency.

Acceptance:
- Config loader supports new sections with defaults.
- Documentation in docs/config.md updated accordingly.
- Unit tests cover parsing and validation.



