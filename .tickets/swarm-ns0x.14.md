---
id: swarm-ns0x.14
status: closed
deps: []
links: []
created: 2026-01-06T20:02:38.42223382+01:00
type: task
priority: 1
parent: swarm-ns0x
---
# Hide legacy node/workspace/tmux CLI surface

Disable legacy CLI groups (node/ws/agent/accounts/vault/tmux) from help and command registry.

Scope:
- Remove from root command tree (or gate behind build tag).
- Any invocation returns a clear "disabled in loop mode" error.
- Update preflight/UX docs to remove references.

Acceptance:
- `forge --help` only shows loop-centric commands.
- Legacy commands cannot be invoked silently.



