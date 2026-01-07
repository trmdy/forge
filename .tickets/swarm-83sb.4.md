---
id: swarm-83sb.4
status: closed
deps: []
links: []
created: 2025-12-27T07:04:48.032921484+01:00
type: task
priority: 1
parent: swarm-83sb
---
# Implement swarm queue ls with status visibility

Enhance queue listing to show dispatch status and blocking reasons.

## Command
```bash
swarm queue ls                    # All queues (uses workspace context)
swarm queue ls --agent A1         # Specific agent queue
swarm queue ls --status pending   # Filter by status
```

## Output columns
- Position: Queue position (1, 2, 3...)
- Type: message | pause | conditional
- Status: pending | dispatched | blocked | completed | failed
- Block Reason: idle_gate, cooldown, paused, busy, dependency
- Content: Truncated message preview
- Created: Relative time

## Example output
```
QUEUE FOR AGENT abc123 (5 items)

POS  TYPE         STATUS    BLOCK REASON           CONTENT               CREATED
1    message      blocked   agent_busy             "Fix the lint..."     2m ago
2    pause        pending   -                      60s pause             2m ago
3    conditional  pending   idle_gate              "Continue when..."    1m ago
4    message      pending   -                      "Then do this..."     1m ago
5    message      pending   -                      "Finally..."          30s ago
```

## Flags
- --agent, -a: Filter by agent
- --status: Filter by status (pending, blocked, completed, failed)
- --limit, -n: Max items to show (default 20)
- --all: Show all items including completed

## JSON output
Include full queue item details with all metadata


