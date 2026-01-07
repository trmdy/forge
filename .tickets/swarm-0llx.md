---
id: swarm-0llx
status: closed
deps: [swarm-18rc]
links: []
created: 2025-12-28T12:24:23.960953753+01:00
type: task
priority: 1
---
# Add forge daemon status command

Add a command to check if forged is running and display its status.

**File**: internal/cli/daemon.go (new file)

**Changes Required**:
1. Create daemonCmd parent command
2. Create daemonStatusCmd subcommand
3. Try to connect to forged via forged.Dial()
4. If connection fails, show 'forged is not running' with helpful message
5. If connected, call GetStatus RPC and display version, uptime, agent count, scheduler status
6. Support JSON output

**Testing**:
- Run without forged -> shows 'not running' with helpful hint
- Run with forged -> shows status info
- JSON output format works

See docs/design/scheduler-daemon-tasks.md#task-41 for full details.


