---
id: swarm-83sb.8
status: closed
deps: []
links: []
created: 2025-12-27T09:33:07.476754391+01:00
type: task
priority: 2
parent: swarm-83sb
---
# Add swarm log command to tail agent transcripts

Add a top-level log command to tail agent transcripts/output.

## From UX_FEEDBACK_2.md - Phase 1

## Command
```bash
swarm log <agent>           # Tail transcript in real-time
swarm log <agent> --last 50 # Show last 50 lines
swarm log <agent> --since 1h # Show logs since 1h ago
swarm log <agent> --follow   # Continuous follow (default)
```

## Features
- Real-time transcript tailing
- Historical log viewing
- Filter by time range
- JSON output for parsing

## Sources
1. Tmux pane capture (existing)
2. Runner event log (once agent runner exists)
3. OpenCode session logs (for OpenCode agents)

## Output Format
```
[2025-12-27 10:30:15] [agent:abc123] User: Fix the bug
[2025-12-27 10:30:16] [agent:abc123] Assistant: I will analyze...
[2025-12-27 10:31:45] [agent:abc123] Tool: edit file.go
[2025-12-27 10:32:00] [agent:abc123] Assistant: Done. The bug was...
```

## Integration
- Works with context system (no agent arg = current context)
- Works with remote agents via SSH/daemon


