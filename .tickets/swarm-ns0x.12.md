---
id: swarm-ns0x.12
status: closed
deps: []
links: []
created: 2026-01-06T20:02:19.189230153+01:00
type: task
priority: 2
parent: swarm-ns0x
---
# Add profile import-aliases (Ralph-compatible)

Implement `forge profile import-aliases` compatible with ralph.sh alias resolution.

Scope:
- Read aliases from shell + alias file (like ralph.sh get_alias_output/parse_alias_command).
- Map known aliases (oc1/oc2/cc1/codex1/pi) to harness defaults.
- Create profiles with auth_home + prompt mode defaults; pi is default harness.

Acceptance:
- Command works with standard ~/.zsh_aliases and fallback grep.
- Imported profiles are available to pools immediately.



