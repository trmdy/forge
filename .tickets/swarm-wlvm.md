---
id: swarm-wlvm
status: closed
deps: []
links: []
created: 2025-12-27T08:52:48.697546349+01:00
type: task
priority: 1
---
# Implement crash recovery and session restore

Handle agent crashes gracefully and restore session state.

## From UX_FEEDBACK_2.md - Minor Issues Triage

## Crash Recovery
If tmux pane disappears:
1. Runner emits `agent.crashed` event
2. Queue items go back to `pending` (or `blocked`) with clear reason
3. Agent state set to ERROR
4. Alert generated for user

## Session Restore
On swarm startup:
1. Scan for existing tmux sessions matching workspace patterns
2. Check if agents are still running
3. Re-attach to running agents
4. Mark crashed agents as crashed
5. Preserve queue state

## Audit Trail
- Every crash gets logged with:
  - Last known state
  - Last output captured
  - Timestamp
  - Queue items affected

## Recovery Actions
- `swarm agent restart <id>` - Respawn crashed agent
- `swarm agent recover --workspace <ws>` - Recover all crashed agents
- `swarm queue retry --agent <id>` - Retry failed queue items

## Implementation
- Add crash detection to state poller
- Add recovery service
- Add CLI commands for recovery


