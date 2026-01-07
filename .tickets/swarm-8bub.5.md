---
id: swarm-8bub.5
status: closed
deps: []
links: []
created: 2025-12-27T07:07:27.081728903+01:00
type: task
priority: 1
parent: swarm-8bub
---
# Implement recipes for mass agent spawning

Add recipe system for rapid multi-agent spawning with initial tasking.

## Problem
Spinning up 8-20 agents should feel like one action, not 8-20 commands.

## Solution
Recipes define agent configurations and initial sequences for batch spawning.

## Recipe format
```yaml
# ~/.config/swarm/recipes/baseline.yaml
name: baseline
description: Standard development setup with 4 agents

agents:
  - count: 2
    type: opencode
    profile_rotation: round-robin  # Rotate through profiles
    initial_sequence: continue
  - count: 2  
    type: claude-code
    profile: work-account
    initial_sequence: review-loop

profile_rotation_order:
  - personal
  - work
  - backup
```

## Commands

### swarm recipe list
```bash
swarm recipe list
```

Output:
```
NAME        AGENTS   DESCRIPTION
baseline    4        Standard development setup
heavy       8        Heavy parallel workload
review      3        Code review focused
```

### swarm recipe show <name>
Show recipe details

### swarm recipe run <name> [--workspace W1]
```bash
swarm recipe run baseline --workspace my-project
```

Equivalent to:
```bash
swarm up --workspace my-project --agents 2 --type opencode --rotate-profiles
swarm up --workspace my-project --agents 2 --type claude-code --profile work
swarm seq run continue --agent agent-1
swarm seq run continue --agent agent-2
# ... etc
```

### swarm up --recipe <name>
Shorthand for workspace creation + recipe execution
```bash
swarm up --recipe baseline --path /my/repo
```

## Profile rotation
- round-robin: Cycle through profiles in order
- random: Random profile selection
- balanced: Prefer profiles with most remaining cooldown time


