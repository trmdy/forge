---
id: swarm-eli6.2.2
status: closed
deps: [swarm-eli6.1.1, swarm-eli6.1.2]
links: []
created: 2025-12-22T10:17:12.405821313+01:00
type: task
priority: 1
parent: swarm-eli6.2
---
# Add preflight checks + actionable errors

Add global preflight checks and improve error messaging for common failures: missing tmux/ssh, config not found, database not migrated, invalid repo path. If config or DB is missing, suggest or prompt to run swarm init (TTY) or print a non-interactive hint. Errors should include a one-line fix command.

## Acceptance Criteria

- errors include hint and next_step for CLI\n- preflight invoked before TUI and high-impact commands\n- missing config/DB suggests swarm init (or prints hint if non-interactive)\n- failure output matches CLI style guide


