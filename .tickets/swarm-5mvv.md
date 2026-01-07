---
id: swarm-5mvv
status: closed
deps: []
links: []
created: 2025-12-27T09:33:39.766880846+01:00
type: task
priority: 2
---
# Print actionable next steps after every CLI command

Every successful CLI command should print what to do next.

## From UX_FEEDBACK_2.md - UX Rule

## Current Problem
Commands succeed but user doesnt know what to do next.

## Solution
After every command, print contextual "next steps":

### Example: swarm up
```
✓ Workspace created: my-project (ws_abc123)
✓ Agent spawned: oc-1 (agent_xyz789)
✓ Tmux session: swarm-my-project

Next steps:
  swarm send oc-1 "your task here"   # Send instructions
  swarm attach oc-1                   # Watch agent work
  swarm log oc-1                      # View transcript
  swarm ps                            # List all agents
```

### Example: swarm send
```
✓ Message queued for agent oc-1 (position #2)

Next steps:
  swarm queue ls --agent oc-1        # View queue
  swarm log oc-1 --follow            # Watch output
  swarm explain oc-1                  # Check status
```

### Example: swarm agent spawn
```
✓ Agent spawned: oc-2 (agent_def456)

Next steps:
  swarm send oc-2 "your task"        # Send instructions
  swarm attach oc-2                   # Watch agent
  swarm agent status oc-2            # Check status
```

## Implementation
- Add `printNextSteps(ctx, action string)` helper
- Call after successful command execution
- Context-aware suggestions based on current state
- Respect --json flag (no next steps in JSON mode)


