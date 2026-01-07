---
id: swarm-eli6.3.6
status: closed
deps: [swarm-eli6.1.2]
links: []
created: 2025-12-22T10:21:35.785567169+01:00
type: task
priority: 1
parent: swarm-eli6.3
---
# Add CLI progress indicators for long-running actions

Add human-mode progress feedback for long-running commands (node doctor/bootstrap, workspace create/import/attach, agent spawn/terminate). Use consistent step labels and durations; avoid noisy output in JSON/JSONL. Provide a way to disable progress output in automation.

## Acceptance Criteria

- progress indicators are visible only in human output\n- steps include start, active phase, and completion summary\n- JSON/JSONL output remains unchanged\n- optional flag or env disables progress output


