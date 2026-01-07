---
id: swarm-83sb.7
status: closed
deps: []
links: []
created: 2025-12-27T07:05:41.342999214+01:00
type: task
priority: 2
parent: swarm-83sb
---
# Add top-level fast aliases (up, ls, ps)

Add power-user fast aliases at the root command level.

## Aliases to add

### swarm up
Quick workspace + agent creation
```bash
swarm up                      # Create workspace from cwd, spawn 1 opencode agent
swarm up --agents 3           # Spawn 3 agents
swarm up --path /repo         # Specify repo path
swarm up --type claude-code   # Use specific agent type
swarm up --recipe baseline    # Use recipe (see template epic)
```

### swarm ls
Alias for `swarm ws list`
```bash
swarm ls                      # List workspaces
swarm ls --node prod          # Filter by node
```

### swarm ps  
Alias for `swarm agent list`
```bash
swarm ps                      # List agents
swarm ps --state idle         # Filter by state
swarm ps -w my-project        # Filter by workspace
```

## Implementation
- Add aliases in root.go init()
- Ensure help text shows aliases
- Document in CLI help

## Help text example
```
Quick Commands:
  up          Create workspace and spawn agents
  ls          List workspaces (alias: ws list)
  ps          List agents (alias: agent list)
  send        Send message to agent queue
  inject      Direct tmux injection (dangerous)
```


