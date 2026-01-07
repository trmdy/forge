---
id: swarm-eli6.1.3
status: closed
deps: []
links: []
created: 2025-12-22T10:17:12.231868811+01:00
type: task
priority: 1
parent: swarm-eli6.1
---
# Define TUI visual system + theme tokens

Scope:
Define a TUI visual system for Swarm: color tokens, spacing, typography constraints (terminal-safe), semantic states, and layout rules.

Rationale:
A consistent theme system is needed before we can deliver a premium TUI. This must be explicit and reusable across views/components.

## Acceptance Criteria

- `docs/ux/tui-theme.md` with theme tokens and usage rules
- `internal/tui/styles` includes base token definitions + at least 2 palettes (default, high-contrast)


