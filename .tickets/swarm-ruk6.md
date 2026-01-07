---
id: swarm-ruk6
status: closed
deps: [swarm-0llx]
links: []
created: 2025-12-28T12:24:40.16839466+01:00
type: task
priority: 2
---
# CLI commands warn if forged is not running

When users queue messages, warn them if forged is not running so they understand the message won't be dispatched automatically.

**Files**: internal/cli/send.go, internal/cli/queue.go, internal/cli/helpers.go

**Changes Required**:
1. Add checkForgedRunning() helper with 1s timeout connection check
2. Add warnIfForgedNotRunning() that prints warning to stderr
3. Call after successful queue in send command
4. Make warning suppressible with --quiet flag
5. Don't break JSON output mode

**Warning text**:
⚠ forged not running. Message queued but won't be dispatched.
  Run 'forged' in another terminal to enable autonomous dispatch.

**Testing**:
- Queue without forged -> warning shown
- Queue with forged -> no warning
- --quiet suppresses warning
- Warning doesn't break scripts (exit code still 0)

See docs/design/scheduler-daemon-tasks.md#task-42 for full details.


