---
id: swarm-83sb.1
status: closed
deps: []
links: []
created: 2025-12-27T07:03:56.465192204+01:00
type: task
priority: 1
parent: swarm-83sb
---
# Implement context system (swarm use)

Add a context system that eliminates repetitive --workspace and --agent flags.

## Commands to implement

### swarm use <workspace|agent>
Sets the current context target. Persists to ~/.config/swarm/context.yaml

```bash
swarm use my-project           # Set workspace context
swarm use my-project:agent-a1  # Set both workspace and agent context
swarm use --agent agent-a1     # Set only agent context within current workspace
swarm use --clear              # Clear all context
```

### Context resolution
1. Explicit --workspace/--agent flags (highest priority)
2. Current directory detection (if in a git repo that matches a workspace)
3. Stored context from `swarm use`
4. Prompt for selection (interactive mode only)

## Files to create/modify
- internal/cli/context.go - New file for context commands
- internal/cli/resolve.go - Add context resolution logic
- internal/config/context.go - Context persistence

## Context file format
```yaml
# ~/.config/swarm/context.yaml
workspace: ws_abc123
agent: agent_xyz789
updated_at: 2025-12-27T00:00:00Z
```

## Integration points
- All agent commands should use resolved context
- All workspace commands should use resolved context
- Show current context in `swarm status` output
- Show context hint in shell prompt (optional)


