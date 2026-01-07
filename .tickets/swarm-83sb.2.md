---
id: swarm-83sb.2
status: closed
deps: []
links: []
created: 2025-12-27T07:04:14.534940814+01:00
type: task
priority: 1
parent: swarm-83sb
---
# Implement swarm inject for immediate tmux injection

Create a new `swarm inject` command for immediate (dangerous) tmux injection.

## Rationale
Current `swarm agent send` does immediate injection. This should become the "dangerous" path while `swarm send` becomes queue-based (safe).

## Command signature
```bash
swarm inject <agent-id> <message>
swarm inject <agent-id> --file prompt.txt
swarm inject <agent-id> --stdin
swarm inject <agent-id> --editor
```

## Behavior
- Sends text directly to tmux pane via send-keys
- No idle check by default (unlike current send)
- Shows warning: "Warning: Direct injection bypasses queue. Use `swarm send` for safe dispatch."
- Requires explicit confirmation for non-idle agents (unless --force)

## Flags
- --force, -f: Skip confirmation for non-idle agents
- --file, -f: Read from file
- --stdin: Read from stdin  
- --editor: Open $EDITOR

## Output
```
⚠ Direct injection to agent abc123
Message sent (bypassed queue)
```

## Implementation
- Move current agent send logic to inject
- Add warning output
- Update help text to discourage routine use


