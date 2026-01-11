---
id: f-bfec
status: closed
deps: [f-0fd1, f-c44a]
links: []
created: 2026-01-10T20:07:19Z
type: task
priority: 1
assignee: Tormod Haugland
parent: f-c2d0
---
# Implement agent registry + who/status/topics

Implement presence metadata and discovery commands.

Required behavior (per docs/forge-mail/SPEC.md):
- Track agents automatically when they send messages (no explicit registration)
- Store agent registry as .fmail/agents/<name>.json with:
  - name, host (optional), status (optional), first_seen, last_seen

Commands:
- fmail who
  - Lists known agents (name, last seen, status)
  - --json supported
- fmail status
  - With no args: show current status
  - With message: set your status
  - --clear clears status
- fmail topics
  - List topics with message count and last activity
  - --json supported

## Acceptance Criteria

- Sending a message creates/updates .fmail/agents/<from>.json with first_seen/last_seen
- `fmail status "..."` updates status for the current agent; `--clear` removes it
- `fmail who` output matches spec fields (human-readable + --json)
- `fmail topics` reports accurate counts and last activity based on .fmail/topics/*


## Notes

**2026-01-11T07:22:14Z**

Implemented agent registry access plus who/status/topics outputs, including topic summaries and status updates.
