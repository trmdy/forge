---
id: swarm-83sb
status: closed
deps: []
links: []
created: 2025-12-27T07:03:24.896401595+01:00
type: epic
priority: 1
---
# EPIC: CLI v2 Redesign - Queue-First Architecture

Comprehensive CLI redesign based on UX feedback to make Swarm feel "elite" and power-user friendly.

## Core Philosophy
- Queue-first by default: `send` = enqueue, `inject` = immediate (dangerous)
- Context system: `swarm use` sets current target to avoid repetitive flags
- Fast-path aliases: `swarm up`, `swarm ls`, `swarm ps`
- Single mental model: queue is the truth

## Key Changes

### Queue Productization
- `swarm send` → enqueues to scheduler
- `swarm inject` → immediate tmux injection (explicit danger)
- `swarm queue ls --agent A1` → shows status: pending/dispatched/blocked + reason
- `swarm explain <agent|queue-item>` → human-readable block reason
- `swarm wait --agent A1 --until idle|queue-empty|cooldown-over` → automation helper

### Context System (`swarm use`)
- `swarm use <workspace|agent>` sets current target
- Implicit repo-based workspace selection
- No more constant `--workspace`/`--agent` flags
- kubectl-like ergonomics

### Top-level Fast Aliases
- `swarm` → TUI (default, already works)
- `swarm up` → create/open workspace + spawn default agents
- `swarm ls` → alias for `swarm ws list`
- `swarm ps` → alias for `swarm agent list`

### Command Surface Cleanup
- One "human default": `swarm status`
- One "dashboards default": `swarm watch --jsonl`
- Everything else is subcommands or flags
- Remove overlapping/confusing commands

## Success Criteria
- Power users can manage 20+ agents without friction
- Automation scripts work reliably with queue-based dispatch
- Context persistence eliminates repetitive typing
- Clear separation between safe (queue) and dangerous (inject) operations


