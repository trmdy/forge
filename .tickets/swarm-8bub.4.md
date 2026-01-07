---
id: swarm-8bub.4
status: closed
deps: []
links: []
created: 2025-12-27T07:07:06.547318933+01:00
type: task
priority: 1
parent: swarm-8bub
---
# Implement sequence CLI commands

Add CLI commands for sequence management.

## Commands

### swarm seq list
```bash
swarm seq list                   # List all sequences
swarm seq list --tags bugfix     # Filter by tag
```

Output:
```
NAME          STEPS   DESCRIPTION                    SOURCE
bugfix        5       Find → Fix → Test → Commit     builtin
feature       6       Implement feature workflow     builtin
my-workflow   3       Custom workflow                user
```

### swarm seq show <name>
```bash
swarm seq show bugfix
```

Output:
```
Sequence: bugfix
Source: builtin
Description: Standard bug fix workflow

Steps:
  1. [message] Find and fix the bug described in issue {{issue_id}}
  2. [pause 30s] Wait for initial analysis
  3. [conditional:idle] Run the tests and report results
  4. [pause 60s]
  5. [message] Commit the fix with a descriptive message

Variables:
  - issue_id (required): The issue number to fix
```

### swarm seq add <name>
Opens $EDITOR with sequence skeleton

### swarm seq edit <name>
Opens existing sequence in $EDITOR

### swarm seq run <name> [--agent A1] [--var key=value]
Enqueue entire sequence to agent
```bash
swarm seq run bugfix --agent abc123 --var issue_id=42
```

Output:
```
Queued sequence "bugfix" (5 steps) for agent abc123
  Step 1: message → queued
  Step 2: pause 30s → queued
  Step 3: conditional → queued
  Step 4: pause 60s → queued  
  Step 5: message → queued
```

### swarm seq delete <name>
```bash
swarm seq delete my-workflow
```


