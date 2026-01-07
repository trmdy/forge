---
id: swarm-ns0x.17
status: closed
deps: []
links: []
created: 2026-01-06T20:03:09.888439783+01:00
type: task
priority: 2
parent: swarm-ns0x
---
# Implement loop naming + selector filters

Add loop naming and selector filters per PRD section 6.2/6.3.

Scope:
- Auto-generate loop names using Simpsons-style adjective+noun/verb combos with >=256 unique combos.
- Allow user-provided --name and --name-prefix.
- Selector filters for loop/pool/profile/state/tag in `forge ps`, `forge msg`, `forge stop/kill`.

Acceptance:
- Name generator yields unique, hype-style names.
- Selectors work consistently across commands.



