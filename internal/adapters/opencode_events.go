// Package adapters provides SSE event watcher for OpenCode agents.
package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/logging"
	"github.com/tOgg1/forge/internal/models"
)

// OpenCodeEvent represents a parsed event from the OpenCode SSE stream.
type OpenCodeEvent struct {
	// Type is the event type (e.g., "session.idle", "session.busy", "permission.requested").
	Type string `json:"type"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// SessionID is the OpenCode session this event relates to.
	SessionID string `json:"session_id,omitempty"`

	// Data contains the raw event payload.
	Data json.RawMessage `json:"data,omitempty"`

	// Error contains error details for error events.
	Error string `json:"error,omitempty"`
}

// OpenCodeEventType constants for known event types.
const (
	OpenCodeEventSessionIdle    = "session.idle"
	OpenCodeEventSessionBusy    = "session.busy"
	OpenCodeEventPermission     = "permission.requested"
	OpenCodeEventPermissionDone = "permission.resolved"
	OpenCodeEventError          = "error"
	OpenCodeEventToolStart      = "tool.start"
	OpenCodeEventToolEnd        = "tool.end"
	OpenCodeEventTokenUsage     = "token.usage"
	OpenCodeEventHeartbeat      = "heartbeat"
)

// EventWatcherConfig configures the OpenCode event watcher.
type EventWatcherConfig struct {
	// ReconnectDelay is the initial delay before reconnecting after disconnect.
	// Defaults to 1 second.
	ReconnectDelay time.Duration

	// MaxReconnectDelay is the maximum delay between reconnection attempts.
	// Defaults to 30 seconds.
	MaxReconnectDelay time.Duration

	// ReconnectBackoffFactor multiplies the delay on each failed attempt.
	// Defaults to 2.0.
	ReconnectBackoffFactor float64

	// HTTPClient is an optional HTTP client to use for SSE connections.
	// If nil, a default client with 0 timeout (for streaming) is used.
	HTTPClient *http.Client
}

// DefaultEventWatcherConfig returns a config with sensible defaults.
func DefaultEventWatcherConfig() EventWatcherConfig {
	return EventWatcherConfig{
		ReconnectDelay:         1 * time.Second,
		MaxReconnectDelay:      30 * time.Second,
		ReconnectBackoffFactor: 2.0,
	}
}

// StateUpdateHandler is called when an agent's state should be updated.
type StateUpdateHandler func(agentID string, state models.AgentState, info models.StateInfo)

// AgentEventHandler is called for each parsed OpenCode event.
type AgentEventHandler func(agentID string, event OpenCodeEvent)

// OpenCodeEventWatcher manages SSE connections to multiple OpenCode agents.
type OpenCodeEventWatcher struct {
	config  EventWatcherConfig
	logger  zerolog.Logger
	client  *http.Client
	mu      sync.RWMutex
	agents  map[string]*agentConnection
	onState StateUpdateHandler
	onEvent AgentEventHandler
}

// agentConnection tracks an active SSE connection to an agent's OpenCode server.
type agentConnection struct {
	agentID   string
	eventsURL string
	cancel    context.CancelFunc
	done      chan struct{}
}

// NewOpenCodeEventWatcher creates a new event watcher with the given config.
func NewOpenCodeEventWatcher(config EventWatcherConfig, onState StateUpdateHandler) *OpenCodeEventWatcher {
	if config.ReconnectDelay <= 0 {
		config.ReconnectDelay = 1 * time.Second
	}
	if config.MaxReconnectDelay <= 0 {
		config.MaxReconnectDelay = 30 * time.Second
	}
	if config.ReconnectBackoffFactor <= 0 {
		config.ReconnectBackoffFactor = 2.0
	}

	client := config.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 0, // No timeout for SSE streaming
		}
	}

	return &OpenCodeEventWatcher{
		config:  config,
		logger:  logging.Component("opencode-events"),
		client:  client,
		agents:  make(map[string]*agentConnection),
		onState: onState,
	}
}

// SetEventHandler sets an optional handler for all parsed events.
func (w *OpenCodeEventWatcher) SetEventHandler(handler AgentEventHandler) {
	w.onEvent = handler
}

// Watch starts watching SSE events for an agent.
// The eventsURL should be the full URL to the OpenCode /event endpoint.
// Returns immediately; events are processed in a background goroutine.
func (w *OpenCodeEventWatcher) Watch(ctx context.Context, agentID, eventsURL string) error {
	if agentID == "" {
		return errors.New("agent ID is required")
	}
	if eventsURL == "" {
		return errors.New("events URL is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if already watching
	if _, exists := w.agents[agentID]; exists {
		return fmt.Errorf("already watching agent %s", agentID)
	}

	// Create a cancellable context for this agent
	agentCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	conn := &agentConnection{
		agentID:   agentID,
		eventsURL: eventsURL,
		cancel:    cancel,
		done:      done,
	}
	w.agents[agentID] = conn

	// Start the watcher goroutine
	go w.watchLoop(agentCtx, conn)

	w.logger.Info().
		Str("agent_id", agentID).
		Str("events_url", eventsURL).
		Msg("started watching OpenCode events")

	return nil
}

// Unwatch stops watching SSE events for an agent.
func (w *OpenCodeEventWatcher) Unwatch(agentID string) error {
	w.mu.Lock()
	conn, exists := w.agents[agentID]
	if !exists {
		w.mu.Unlock()
		return fmt.Errorf("not watching agent %s", agentID)
	}
	delete(w.agents, agentID)
	w.mu.Unlock()

	// Cancel the context and wait for goroutine to finish
	conn.cancel()
	<-conn.done

	w.logger.Info().
		Str("agent_id", agentID).
		Msg("stopped watching OpenCode events")

	return nil
}

// UnwatchAll stops all active watchers.
func (w *OpenCodeEventWatcher) UnwatchAll() {
	w.mu.Lock()
	agents := make([]*agentConnection, 0, len(w.agents))
	for _, conn := range w.agents {
		agents = append(agents, conn)
	}
	w.agents = make(map[string]*agentConnection)
	w.mu.Unlock()

	for _, conn := range agents {
		conn.cancel()
		<-conn.done
	}

	w.logger.Info().Msg("stopped all OpenCode event watchers")
}

// IsWatching returns true if the watcher is actively watching the given agent.
func (w *OpenCodeEventWatcher) IsWatching(agentID string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	_, exists := w.agents[agentID]
	return exists
}

// WatchedAgents returns a list of agent IDs currently being watched.
func (w *OpenCodeEventWatcher) WatchedAgents() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	agents := make([]string, 0, len(w.agents))
	for id := range w.agents {
		agents = append(agents, id)
	}
	return agents
}

// watchLoop runs the SSE connection with automatic reconnection.
func (w *OpenCodeEventWatcher) watchLoop(ctx context.Context, conn *agentConnection) {
	defer close(conn.done)

	delay := w.config.ReconnectDelay

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := w.connectAndStream(ctx, conn)
		if errors.Is(err, context.Canceled) {
			return
		}
		// If err is nil, the connection closed cleanly - still need to reconnect
		if err == nil {
			err = errors.New("connection closed")
		}

		w.logger.Warn().
			Err(err).
			Str("agent_id", conn.agentID).
			Dur("retry_in", delay).
			Msg("SSE connection failed, will retry")

		// Wait before reconnecting
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		// Exponential backoff
		delay = time.Duration(float64(delay) * w.config.ReconnectBackoffFactor)
		if delay > w.config.MaxReconnectDelay {
			delay = w.config.MaxReconnectDelay
		}
	}
}

// connectAndStream establishes an SSE connection and processes events.
func (w *OpenCodeEventWatcher) connectAndStream(ctx context.Context, conn *agentConnection) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, conn.eventsURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	w.logger.Debug().
		Str("agent_id", conn.agentID).
		Msg("SSE connection established")

	// Reset backoff on successful connection
	return w.streamEvents(ctx, conn, resp.Body)
}

// streamEvents reads and processes events from the SSE stream.
func (w *OpenCodeEventWatcher) streamEvents(ctx context.Context, conn *agentConnection, body io.Reader) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // Allow up to 1MB lines

	var eventType string
	var dataLines []string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()

		// Empty line means end of event
		if line == "" {
			if len(dataLines) > 0 {
				data := strings.Join(dataLines, "\n")
				w.handleEvent(conn.agentID, eventType, data)
			}
			eventType = ""
			dataLines = nil
			continue
		}

		// Parse SSE fields
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
		}
		// Ignore id:, retry:, and comments (:)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}

	return nil // Connection closed normally
}

// handleEvent processes a single SSE event.
func (w *OpenCodeEventWatcher) handleEvent(agentID, eventType, data string) {
	event := OpenCodeEvent{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
	}

	// Try to parse the data as JSON
	if data != "" {
		if json.Valid([]byte(data)) {
			event.Data = json.RawMessage(data)

			// Try to extract session_id from the data
			var payload struct {
				SessionID string `json:"session_id"`
				Error     string `json:"error"`
			}
			if err := json.Unmarshal([]byte(data), &payload); err == nil {
				event.SessionID = payload.SessionID
				event.Error = payload.Error
			}
		}
	}

	w.logger.Debug().
		Str("agent_id", agentID).
		Str("event_type", eventType).
		Msg("received OpenCode event")

	// Call the event handler if set
	if w.onEvent != nil {
		w.onEvent(agentID, event)
	}

	// Map to agent state and call state handler
	if w.onState != nil {
		state, info, ok := w.mapEventToState(event)
		if ok {
			w.onState(agentID, state, info)
		}
	}
}

// mapEventToState converts an OpenCode event to a Forge agent state.
func (w *OpenCodeEventWatcher) mapEventToState(event OpenCodeEvent) (models.AgentState, models.StateInfo, bool) {
	now := time.Now().UTC()

	switch event.Type {
	case OpenCodeEventSessionIdle:
		return models.AgentStateIdle, models.StateInfo{
			State:      models.AgentStateIdle,
			Confidence: models.StateConfidenceHigh,
			Reason:     "OpenCode SSE: session idle",
			DetectedAt: now,
		}, true

	case OpenCodeEventSessionBusy, OpenCodeEventToolStart:
		return models.AgentStateWorking, models.StateInfo{
			State:      models.AgentStateWorking,
			Confidence: models.StateConfidenceHigh,
			Reason:     fmt.Sprintf("OpenCode SSE: %s", event.Type),
			DetectedAt: now,
		}, true

	case OpenCodeEventPermission:
		return models.AgentStateAwaitingApproval, models.StateInfo{
			State:      models.AgentStateAwaitingApproval,
			Confidence: models.StateConfidenceHigh,
			Reason:     "OpenCode SSE: permission requested",
			DetectedAt: now,
		}, true

	case OpenCodeEventPermissionDone:
		// After permission is resolved, agent goes back to working
		return models.AgentStateWorking, models.StateInfo{
			State:      models.AgentStateWorking,
			Confidence: models.StateConfidenceHigh,
			Reason:     "OpenCode SSE: permission resolved",
			DetectedAt: now,
		}, true

	case OpenCodeEventError:
		reason := "OpenCode SSE: error"
		if event.Error != "" {
			reason = fmt.Sprintf("OpenCode SSE error: %s", event.Error)
		}
		return models.AgentStateError, models.StateInfo{
			State:      models.AgentStateError,
			Confidence: models.StateConfidenceHigh,
			Reason:     reason,
			DetectedAt: now,
		}, true

	case OpenCodeEventToolEnd:
		// Tool end doesn't necessarily mean idle; the agent may continue working
		// So we don't emit a state change here
		return "", models.StateInfo{}, false

	case OpenCodeEventHeartbeat, OpenCodeEventTokenUsage:
		// These are informational, don't change state
		return "", models.StateInfo{}, false

	default:
		// Unknown event type, log but don't change state
		w.logger.Debug().
			Str("event_type", event.Type).
			Msg("unknown OpenCode event type, ignoring")
		return "", models.StateInfo{}, false
	}
}

// WatchAgent is a convenience method that starts watching an agent using its connection info.
func (w *OpenCodeEventWatcher) WatchAgent(ctx context.Context, agent *models.Agent) error {
	if agent == nil {
		return errors.New("agent is nil")
	}
	if !agent.HasOpenCodeConnection() {
		return fmt.Errorf("agent %s has no OpenCode connection", agent.ID)
	}

	eventsURL := agent.Metadata.OpenCode.EventsURL()
	if eventsURL == "" {
		return fmt.Errorf("agent %s has invalid OpenCode connection", agent.ID)
	}

	return w.Watch(ctx, agent.ID, eventsURL)
}
