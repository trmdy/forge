---
id: swarm-83sb.6
status: closed
deps: []
links: []
created: 2025-12-27T07:05:24.577397043+01:00
type: task
priority: 1
parent: swarm-83sb
---
# Implement swarm wait for automation

Add a wait command for automation scripts that blocks until conditions are met.

## Command
```bash
swarm wait --agent A1 --until idle          # Wait for agent to be idle
swarm wait --agent A1 --until queue-empty   # Wait for queue to drain
swarm wait --agent A1 --until cooldown-over # Wait for account cooldown
swarm wait --workspace W1 --until all-idle  # Wait for all agents idle
swarm wait --timeout 5m                     # With timeout
```

## Conditions
- `idle`: Agent state is idle
- `queue-empty`: No pending queue items
- `cooldown-over`: Account cooldown expired
- `all-idle`: All agents in workspace are idle
- `any-idle`: At least one agent is idle
- `ready`: Agent is idle AND no cooldown AND queue empty

## Output (streaming)
```
Waiting for agent abc123 to be idle...
  Current state: working (tool_calls detected)
  Queue: 3 pending items
  Elapsed: 45s

Agent abc123 is now idle (waited 1m23s)
```

## Exit codes
- 0: Condition met
- 1: Timeout reached
- 2: Agent/workspace not found

## Flags
- --until, -u: Condition to wait for (required)
- --agent, -a: Agent to wait for
- --workspace, -w: Workspace to wait for
- --timeout, -t: Maximum wait time (default: 30m)
- --poll-interval: Check interval (default: 2s)
- --quiet, -q: No output, just wait

## Use cases
- CI/CD: Wait for queue to drain before deploying
- Automation: Chain commands that depend on agent state
- Cooldown management: Wait before sending more prompts


