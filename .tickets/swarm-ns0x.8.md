---
id: swarm-ns0x.8
status: closed
deps: []
links: []
created: 2026-01-06T20:01:34.760424968+01:00
type: task
priority: 1
parent: swarm-ns0x
---
# Implement queue execution + templates/sequences integration

Wire queue items into loop execution and integrate templates/sequences (PRD sections 3.7, 3.8, 4.1).

Scope:
- Implement queue types: MessageAppend, NextPromptOverrideOnce, Pause, StopGraceful, KillNow, SteerMessage.
- MessageAppend uses operator message wrapper with timestamp.
- `--template` and `--seq` resolve from .forge/templates + .forge/sequences YAML formats.

Acceptance:
- Queue items are persisted, dequeued, and marked completed/skipped appropriately.
- NextPromptOverrideOnce applies once and is consumed.
- Pause delays next iteration.



