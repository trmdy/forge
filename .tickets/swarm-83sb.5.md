---
id: swarm-83sb.5
status: closed
deps: []
links: []
created: 2025-12-27T07:05:05.319824522+01:00
type: task
priority: 1
parent: swarm-83sb
---
# Implement swarm explain for human-readable status

Add an explain command that provides human-readable status explanations.

## Command
```bash
swarm explain <agent-id>          # Explain agent status
swarm explain <queue-item-id>     # Explain why queue item is blocked
swarm explain                     # Explain current context agent
```

## Agent explanation output
```
Agent abc123 is BLOCKED

Reason: Account cooldown active
  - Profile "work-account" used 847 tokens in last hour
  - Cooldown ends at 18:42:13 (in 23 minutes)
  
Suggestions:
  1. Wait for cooldown to expire
  2. Switch to a different profile: swarm agent rotate abc123
  3. Queue messages for later: swarm send abc123 "continue"

Queue Status:
  - 3 pending messages
  - Next dispatch: after cooldown expires
```

## Queue item explanation output
```
Queue item qi_123 is BLOCKED

Reason: Waiting for agent idle state
  - Agent is currently working (detected tool calls in output)
  - Last activity: 30s ago
  - Estimated completion: unknown

This is a conditional item that will dispatch when:
  ✓ Agent reaches idle state
  ✓ No higher-priority items pending
```

## Implementation
- Add explain subcommand to agent and queue
- Create explainer package with status analysis logic
- Pull data from scheduler, agent state, account cooldowns


