---
id: swarm-uhxn
status: closed
deps: []
links: []
created: 2025-12-27T08:49:54.991761596+01:00
type: epic
priority: 0
---
# EPIC: Agent Runner Architecture

Implement a proper Agent Runner harness that wraps agent CLI processes for reliable state detection and control.

## Problem (from UX_FEEDBACK_2.md)
If "agent control" is mostly `tmux send-keys` + "guess state from terminal text", you get:
- Flaky state detection
- Broken automation after upstream CLI updates
- Hard-to-reproduce bugs ("it was idle but we thought it was working")
- Brittle cooldown enforcement

## Solution
Run agents through a structured runner harness:
```
tmux pane command: swarm-agent-runner --workspace W --agent A -- <actual agent cli...>
```

## What the Runner Does
1. **Owns a PTY** around the agent CLI process
2. **Emits structured events**:
   - Last input sent
   - Last output lines (tail)
   - Heartbeat timestamps
   - Detected "prompt ready" vs "thinking"
3. **Implements control protocol**:
   - SendMessage(text)
   - Pause(duration) / Cooldown(until)
   - SwapAccount(account_id)
4. **Writes events** into event log / SQLite

## Tiered Capability Model
- **Tier 0 (Basic)**: send-keys + "prompt regex" idle detection
- **Tier 1 (Better)**: runner-level PTY + output parsing + explicit heartbeat
- **Tier 2 (Best)**: native hooks/plugin/events (OpenCode lives here)

## Success Criteria
- Tmux is no longer the place you infer state from
- Standardized control/telemetry across very different CLIs
- Deterministic state machine with testable transitions


