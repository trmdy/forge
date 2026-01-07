---
id: swarm-ns0x.3
status: closed
deps: []
links: []
created: 2026-01-06T20:00:37.233259266+01:00
type: task
priority: 1
parent: swarm-ns0x
---
# Rewrite forge init for repo .forge scaffolding

Implement the new `forge init` flow per PRD section 6.1 and 7.1.

Scope:
- Create .forge/forge.yaml, .forge/prompts/, .forge/templates/, .forge/sequences/, .forge/ledgers/.
- Optionally create PROMPT.md if missing (configurable).
- Support --prompts-from and --no-create-prompt flags.
- Remove/disable legacy init behavior and preflight references.

Acceptance:
- `forge init` creates expected files with correct defaults.
- CLI help/examples match PRD.
- Legacy init path is no longer reachable.



