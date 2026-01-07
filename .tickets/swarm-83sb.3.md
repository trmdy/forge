---
id: swarm-83sb.3
status: closed
deps: []
links: []
created: 2025-12-27T07:04:29.899493216+01:00
type: task
priority: 1
parent: swarm-83sb
---
# Make swarm send queue-based by default

Refactor `swarm send` to enqueue messages instead of immediate injection.

## New behavior
```bash
swarm send <agent-id> "Fix the bug"     # Enqueues to scheduler
swarm send "Fix the bug"                 # Uses context agent
swarm send --all "Continue"              # Sends to all workspace agents
```

## Changes
1. `swarm send` → calls queue.Enqueue() instead of tmux send-keys
2. `swarm agent send` → deprecated alias pointing to `swarm send`
3. Shows queue position: "Queued at position #3 for agent abc123"

## Flags
- --priority high|normal|low: Queue priority
- --after <queue-item-id>: Insert after specific item
- --front: Insert at front of queue
- --when idle: Only dispatch when agent is idle (conditional)
- --all: Send to all agents in workspace

## Output
```
Queued message for agent abc123 (position #3)
  "Fix the bug"
```

## Migration
- Add deprecation warning to `swarm agent send`
- Update all documentation
- Add `--immediate` flag to send for backwards compatibility (deprecated)


