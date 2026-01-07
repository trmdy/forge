---
id: swarm-9w5s
status: closed
deps: []
links: []
created: 2025-12-27T21:41:45.568641546+01:00
type: epic
priority: 0
---
# EPIC: Rename Swarm to Forge

Rename the project from Swarm to Forge as recommended in UX_FEEDBACK_2.md.

## Why
- 'Swarm' is generic and conflicts with other projects
- Do it before CLI becomes widely used (command names fossilize)
- Forge evokes 'crafting/building' which fits the agent orchestration theme

## Naming Conventions
- CLI binary: forge
- Node daemon: forged
- Config dir: ~/.config/forge/
- Data dir: ~/.local/share/forge/
- Env vars: FORGE_* (keep SWARM_* as deprecated aliases)

## Scope
- Rename binaries and commands
- Update module path
- Update config/data paths with migration
- Update all documentation
- Update help text and CLI output
- Keep backward compat for env vars (one release)

## Out of Scope (owner will do later)
- Rename repository folder
- Rename GitHub repo


