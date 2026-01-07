---
id: swarm-5ipa.1
status: closed
deps: []
links: []
created: 2025-12-27T08:50:46.468074995+01:00
type: task
priority: 0
parent: swarm-5ipa
---
# Store OpenCode connection info in Agent.Metadata

Add OpenCode server connection details to agent metadata for native control.

## New Metadata Fields
```go
type AgentMetadata struct {
    OpenCode struct {
        Host      string `json:"host"`
        Port      int    `json:"port"`
        SessionID string `json:"session_id"`
        BaseURL   string `json:"base_url"` // derived
    } `json:"opencode,omitempty"`
}
```

## Changes to Agent Spawn
1. Allocate available port from pool (e.g., 17000-17999)
2. Store in Agent.Metadata.OpenCode
3. Pass port to spawn command

## Port Allocation
- Track used ports in DB
- Allocate from range on spawn
- Release on terminate
- Handle port conflicts gracefully

## Validation
- Add `GetOpenCodeURL(agent)` helper
- Return error if metadata missing for OpenCode agent


