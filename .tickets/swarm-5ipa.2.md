---
id: swarm-5ipa.2
status: closed
deps: []
links: []
created: 2025-12-27T08:51:03.881615924+01:00
type: task
priority: 0
parent: swarm-5ipa
---
# Implement OpenCode API client for message injection

Replace tmux send-keys with OpenCode prompt API for message injection.

## Package: internal/adapters/opencode_client.go

## API Endpoints to Use
- `POST /tui/append-prompt` - Add text to prompt buffer
- `POST /tui/submit-prompt` - Submit the prompt

## Client Interface
```go
type OpenCodeClient struct {
    BaseURL string
    Client  *http.Client
}

func (c *OpenCodeClient) SendMessage(ctx context.Context, text string) error {
    // 1. POST /tui/append-prompt with text
    // 2. POST /tui/submit-prompt
    // 3. Return error if either fails
}

func (c *OpenCodeClient) GetStatus(ctx context.Context) (*SessionStatus, error) {
    // GET /session or similar
}
```

## Integration with Agent Service
- Check if agent has OpenCode metadata
- If yes, use API client instead of tmux
- Fall back to tmux if API unavailable

## Error Handling
- Timeout if OpenCode server not responding
- Retry with backoff
- Fall back to tmux injection as last resort
- Log which method was used


