---
id: swarm-5ipa
status: closed
deps: []
links: []
created: 2025-12-27T08:50:32.309730489+01:00
type: epic
priority: 0
---
# EPIC: OpenCode Native Integration

Replace tmux injection with OpenCode native API for reliable agent control.

## Why This Matters (from UX_FEEDBACK_1.md)
OpenCode gives you:
- Built-in server control surface (OpenAPI endpoints + SSE event stream)
- Plugin system for tools and event subscription
- Skills system compatible with Claude skills
- Structured API instead of terminal UI heuristics

This is your competitive advantage: you can build *reliable orchestration* on a *structured API*.

## OpenCode APIs to Use
- Server endpoints: OpenAPI `/doc`
- Event streams: SSE `/event` and `/global/event`
- TUI endpoints: `/tui/append-prompt`, `/tui/submit-prompt`
- Session endpoints: `/session`, `/session/{id}/message`

## Spawn Model: One OpenCode Server Per Agent
1. Allocate port per agent (node-local)
2. Start `opencode serve --port X --hostname 127.0.0.1`
3. In tmux pane, run `opencode --port X --hostname 127.0.0.1`
4. Save host/port into Agent.Metadata

## Key Benefits
- Message injection via API (not tmux send-keys)
- State updates via SSE events (not screen scraping)
- Permission prompts detected reliably
- Token usage tracked accurately

## Success Criteria
- `swarm send` uses OpenCode API, not tmux
- Agent state updates come from SSE, not polling
- Works even if tmux pane isnt focused


