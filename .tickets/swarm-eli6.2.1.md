---
id: swarm-eli6.2.1
status: closed
deps: [swarm-j39n]
links: []
created: 2025-12-22T10:17:12.342151976+01:00
type: task
priority: 1
parent: swarm-eli6.2
---
# Implement `swarm init` first-run wizard

Scope:
Implement `swarm init` as a first-run wizard:
- create config file (from template)
- run migrations
- auto-register local node (if configured)
- verify tmux + git availability
- print clear next steps

Considerations:
Must support non-interactive mode and `--yes` for automation (tie into swarm-j39n).

## Acceptance Criteria

- `swarm init` exits with clear success summary
- `--yes` skips prompts, uses defaults
- failure modes include actionable hints (missing tmux, db path not writable)


