---
id: swarm-8bub.3
status: closed
deps: []
links: []
created: 2025-12-27T07:06:50.002133024+01:00
type: task
priority: 1
parent: swarm-8bub
---
# Implement template CLI commands

Add CLI commands for template management.

## Commands

### swarm template list
```bash
swarm template list              # List all templates
swarm template list --tags work  # Filter by tag
```

Output:
```
NAME        DESCRIPTION                     SOURCE      TAGS
continue    Resume current task             builtin     
commit      Commit changes                  builtin     git
explain     Ask agent to explain state      builtin     
my-review   Custom review template          user        review, work
```

### swarm template show <name>
```bash
swarm template show continue
```

Output:
```
Template: continue
Source: builtin
Description: Resume current task

Message:
  Continue working on the current task. If you are blocked,
  explain what you need to proceed.

Variables: (none)
```

### swarm template add <name>
Opens $EDITOR with template skeleton
```bash
swarm template add my-review
```

### swarm template edit <name>
Opens existing template in $EDITOR
```bash
swarm template edit my-review
```

### swarm template run <name> [--agent A1] [--var key=value]
Enqueue template message to agent
```bash
swarm template run continue --agent abc123
swarm template run bugfix --var issue_id=123
```

### swarm template delete <name>
```bash
swarm template delete my-review
```


