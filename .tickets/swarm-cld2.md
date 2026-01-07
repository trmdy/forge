---
id: swarm-cld2
status: closed
deps: [swarm-mkhn]
links: []
created: 2025-12-28T12:25:38.922688528+01:00
type: task
priority: 2
---
# Document forged setup in README

Comprehensive documentation for setting up and running forged.

**File**: README.md

**Changes Required**:
1. Add 'Running the Daemon' section
2. Document development/manual mode (foreground)
3. Link to systemd and launchd setup guides
4. Document all configuration options (config.yaml and CLI flags)
5. Add troubleshooting section

**Topics to cover**:
- Why forged is needed (automatic dispatch, SSE state detection)
- Running in foreground vs background
- Configuration options
- Checking status
- Common issues

See docs/design/scheduler-daemon-tasks.md#task-53 for full details.


