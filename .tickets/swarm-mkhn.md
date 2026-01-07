---
id: swarm-mkhn
status: closed
deps: [swarm-18rc]
links: []
created: 2025-12-28T12:24:57.623924201+01:00
type: task
priority: 2
---
# Update docs to indicate forged is required for queue dispatch

Update documentation to explain that forged must be running for messages to be dispatched automatically.

**Files**: README.md, docs/ files

**Changes Required**:
1. Update README.md quick start to include 'forged' step
2. Add architecture explanation (forge CLI vs forged daemon)
3. Explain what forged provides: automatic dispatch, SSE state detection, resource monitoring
4. Add troubleshooting section mentioning forged
5. Update any existing queue/send documentation

See docs/design/scheduler-daemon-tasks.md#task-43 for full details.


