---
id: swarm-uhxn.1
status: closed
deps: []
links: []
created: 2025-12-27T08:50:11.954761456+01:00
type: task
priority: 0
parent: swarm-uhxn
---
# Implement Agent Runner binary

Create the swarm-agent-runner binary that wraps agent CLI processes.

## Package: internal/agent/runner + cmd/swarm-agent-runner

## Core Interface
```go
type Runner struct {
    WorkspaceID string
    AgentID     string
    Command     []string // The actual agent CLI command
    PTY         *os.File
}

// Events emitted
type RunnerEvent struct {
    Type      string    // heartbeat, input_sent, output_line, prompt_ready, busy
    Timestamp time.Time
    Data      any
}
```

## Implementation
1. Create PTY around agent process
2. Capture stdout/stderr to ring buffer
3. Parse output for state signals (prompt patterns, tool calls, etc)
4. Emit events to event log via unix socket or direct DB write
5. Accept control commands via stdin or unix socket

## Command Line
```bash
swarm-agent-runner \
  --workspace ws_123 \
  --agent agent_456 \
  --event-socket /tmp/swarm-events.sock \
  -- opencode --model claude-sonnet
```

## State Detection
- Parse output for adapter-specific patterns
- Detect "prompt ready" vs "thinking/working"
- Track last activity timestamp
- Emit heartbeats at configurable interval

## Integration Test
Create fake "agent CLI" script that:
- Prints prompts
- Simulates busy/idle transitions
- Verify runner emits correct events


