---
id: swarm-j25q.2
status: closed
deps: []
links: []
created: 2025-12-27T07:10:29.086476732+01:00
type: task
priority: 1
parent: swarm-j25q
---
# Implement swarm lock CLI commands

Add advisory file locking commands for multi-agent coordination.

## Commands

### swarm lock claim
Claim advisory lock on files/patterns.
```bash
swarm lock claim --agent agent-a1 --path "src/api/*.go"
swarm lock claim --agent agent-a1 --path "src/api/users.go" --ttl 30m
swarm lock claim --agent agent-a1 --path "src/**/*.ts" --exclusive
```

Flags:
- --agent, -a: Agent claiming the lock (required)
- --path, -p: File path or glob pattern (required, repeatable)
- --ttl: Lock duration (default: 1h)
- --exclusive: Exclusive lock (default: true)
- --reason: Why the lock is needed
- --json: Output JSON

Output:
```
Lock claimed:
  Agent:   agent-a1
  Paths:   src/api/*.go
  TTL:     30m
  Expires: 2025-12-27T12:30:00Z
```

### swarm lock release
Release locks held by an agent.
```bash
swarm lock release --agent agent-a1                    # Release all
swarm lock release --agent agent-a1 --path "src/api/*" # Release specific
swarm lock release --agent agent-a1 --lock-id lock-123
```

### swarm lock status
Check lock status.
```bash
swarm lock status                          # All locks in workspace
swarm lock status --path "src/api/users.go" # Check specific file
swarm lock status --agent agent-a1          # Locks held by agent
```

Output:
```
LOCK-ID     AGENT       PATH              EXPIRES    EXCLUSIVE
lock-001    agent-a1    src/api/*.go      in 28m     yes
lock-002    agent-b2    src/models/*      in 45m     yes
```

### swarm lock check
Check if a path is lockable before claiming.
```bash
swarm lock check --path "src/api/users.go"
```

Output (if locked):
```
Path is locked:
  Holder: agent-a1
  Pattern: src/api/*.go
  Expires: in 28m
```

## Conflict resolution
- If lock conflicts, show holder and suggest waiting or contacting
- Support --force for emergency override (requires confirmation)


