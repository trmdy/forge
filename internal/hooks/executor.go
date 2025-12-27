package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/opencode-ai/swarm/internal/logging"
	"github.com/opencode-ai/swarm/internal/models"
	"github.com/rs/zerolog"
)

// Executor runs hook actions for incoming events.
type Executor struct {
	client *http.Client
	logger zerolog.Logger
}

// DefaultTimeout is used when a hook does not specify a timeout.
const DefaultTimeout = 30 * time.Second

// NewExecutor returns a hook executor with a default HTTP client.
func NewExecutor() *Executor {
	return &Executor{
		client: &http.Client{},
		logger: logging.Component("hooks"),
	}
}

// Execute runs the hook for the given event.
func (e *Executor) Execute(ctx context.Context, hook Hook, event *models.Event) error {
	if hook.Kind == "" {
		hook.Kind = inferKind(hook)
	}

	switch hook.Kind {
	case KindCommand:
		return e.runCommand(ctx, hook, event)
	case KindWebhook:
		return e.sendWebhook(ctx, hook, event)
	default:
		return fmt.Errorf("unsupported hook kind: %s", hook.Kind)
	}
}

func inferKind(hook Hook) Kind {
	if strings.TrimSpace(hook.URL) != "" {
		return KindWebhook
	}
	return KindCommand
}

func (e *Executor) runCommand(ctx context.Context, hook Hook, event *models.Event) error {
	command := strings.TrimSpace(hook.Command)
	if command == "" {
		return fmt.Errorf("hook command is required")
	}

	payload, err := marshalEvent(event)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = append(os.Environ(), eventEnv(event)...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (e *Executor) sendWebhook(ctx context.Context, hook Hook, event *models.Event) error {
	url := strings.TrimSpace(hook.URL)
	if url == "" {
		return fmt.Errorf("hook URL is required")
	}

	payload, err := marshalEvent(event)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to build webhook request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	for key, value := range hook.Headers {
		if strings.TrimSpace(key) == "" {
			continue
		}
		request.Header.Set(key, value)
	}

	response, err := e.client.Do(request)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode >= http.StatusMultipleChoices {
		e.logger.Warn().
			Str("hook_id", hook.ID).
			Int("status", response.StatusCode).
			Msg("webhook returned non-2xx status")
		return fmt.Errorf("webhook returned status %d", response.StatusCode)
	}

	return nil
}

func marshalEvent(event *models.Event) ([]byte, error) {
	if event == nil {
		return json.Marshal(map[string]any{"event": nil})
	}

	return json.Marshal(event)
}

func eventEnv(event *models.Event) []string {
	if event == nil {
		return nil
	}

	env := []string{}
	if event.ID != "" {
		env = append(env, "SWARM_EVENT_ID="+event.ID)
	}
	if !event.Timestamp.IsZero() {
		env = append(env, "SWARM_EVENT_TIMESTAMP="+event.Timestamp.UTC().Format(time.RFC3339))
	}
	if event.Type != "" {
		env = append(env, "SWARM_EVENT_TYPE="+string(event.Type))
	}
	if event.EntityType != "" {
		env = append(env, "SWARM_ENTITY_TYPE="+string(event.EntityType))
	}
	if event.EntityID != "" {
		env = append(env, "SWARM_ENTITY_ID="+event.EntityID)
	}
	if len(event.Payload) > 0 {
		env = append(env, "SWARM_EVENT_PAYLOAD="+string(event.Payload))
	}
	return env
}
