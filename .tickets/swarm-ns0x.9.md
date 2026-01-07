---
id: swarm-ns0x.9
status: closed
deps: []
links: []
created: 2026-01-06T20:01:46.539724913+01:00
type: task
priority: 1
parent: swarm-ns0x
---
# Implement centralized logs + per-loop ledger format

Implement logging + ledger outputs per PRD section 9 and clarified behavior.

Scope:
- Centralized log files under global.data_dir (per-loop log paths stored in DB).
- `forge logs` tails per-loop logs with --follow/--since/--lines.
- Ledger files under .forge/ledgers/<loop>.md with YAML front-matter.
- Ledger entries include prompt source (base vs override), profile used, exit code, timestamps, and output tail; include git summary if configured.
- On `forge msg --now`, include ledger + last log tail in next prompt.

Acceptance:
- Logs are written for each iteration and `forge logs` can tail them.
- Ledger entries are appended per iteration in repo.
- YAML front-matter is consistent and parseable.



