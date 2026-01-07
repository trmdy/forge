---
id: swarm-5ipa.3
status: closed
deps: []
links: []
created: 2025-12-27T08:51:20.399219544+01:00
type: task
priority: 0
parent: swarm-5ipa
---
# Implement OpenCode SSE event watcher for state updates

Subscribe to OpenCode SSE events for reliable state detection.

## Package: internal/adapters/opencode_events.go

## SSE Endpoints
- `/event` - Per-session events
- `/global/event` - All sessions

## Event Types to Handle
- Session idle/busy signals
- Permission prompts
- Tool calls started/completed
- Errors
- Token usage

## Implementation
```go
type OpenCodeEventWatcher struct {
    agents map[string]*AgentConnection
}

type AgentConnection struct {
    AgentID   string
    BaseURL   string
    EventChan chan OpenCodeEvent
    Done      chan struct{}
}

func (w *OpenCodeEventWatcher) Watch(ctx context.Context, agent *Agent) error {
    // 1. Connect to SSE endpoint
    // 2. Parse events
    // 3. Update agent state in DB
    // 4. Emit swarm events
}
```

## State Mapping
| OpenCode Event | Swarm State |
|----------------|-------------|
| session.idle   | IDLE        |
| session.busy   | BUSY        |
| permission.requested | WAITING_PERMISSION |
| error          | ERROR       |

## Integration
- Start watcher when agent spawns
- Stop watcher when agent terminates
- Handle reconnection on disconnect
- Log all state transitions


