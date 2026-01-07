---
id: swarm-blrp
status: closed
deps: []
links: []
created: 2025-12-27T08:53:24.880114587+01:00
type: epic
priority: 2
---
# EPIC: CASS Integration

Integrate with CASS (Code Agent Session Search) for session indexing and context injection.

## From UX_FEEDBACK_2.md

## Two Directions of Flow

### A) Swarm → CASS (write/search index)
Every agent session should produce:
- Durable transcript/log file location
- Metadata: repo, workspace, node, agent type, start/end, outcome

Command: `swarm cass index --workspace W --agent A --session <id>`

### B) CASS → Swarm (context injection)
Before dispatching a new task (or even before queueing), coordinator layer can ask:
- `swarm cass search --query "..."`
- `swarm cass context --task "..."`

Swarm exposes these as CLI calls returning JSON.

## Minimal Learning Loop (cass-memory style)
Record outcomes per task/session:
- Success/failure
- Duration
- Retries
- Interruptions

Attach them to a "pattern id" or "workflow id" that coordinator layer assigns.
Use that later for ranking strategies.

## Implementation
- Add CASS client package
- Add session metadata export
- Add search/context CLI commands
- Track outcomes per task/session


