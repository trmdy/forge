// Package state provides agent state management and change notifications.
package state

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/opencode-ai/swarm/internal/adapters"
	"github.com/opencode-ai/swarm/internal/db"
	"github.com/opencode-ai/swarm/internal/logging"
	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/tmux"
	"github.com/rs/zerolog"
)

// Engine errors.
var (
	ErrAgentNotFound     = errors.New("agent not found")
	ErrSubscriberExists  = errors.New("subscriber already exists")
	ErrSubscriberMissing = errors.New("subscriber not found")
)

// StateChange represents a state transition event.
type StateChange struct {
	// AgentID is the agent whose state changed.
	AgentID string

	// PreviousState is the state before the change.
	PreviousState models.AgentState

	// CurrentState is the new state.
	CurrentState models.AgentState

	// StateInfo contains detailed information about the new state.
	StateInfo models.StateInfo

	// Timestamp is when the change was detected.
	Timestamp time.Time
}

// Subscriber receives state change notifications.
type Subscriber interface {
	// OnStateChange is called when an agent's state changes.
	OnStateChange(change StateChange)
}

// SubscriberFunc is a function adapter for Subscriber.
type SubscriberFunc func(StateChange)

// OnStateChange implements Subscriber.
func (f SubscriberFunc) OnStateChange(change StateChange) {
	f(change)
}

// DetectionResult contains the result of state detection.
type DetectionResult struct {
	// State is the detected state.
	State models.AgentState

	// Confidence indicates how certain we are.
	Confidence models.StateConfidence

	// Reason explains why we detected this state.
	Reason string

	// Evidence contains supporting data.
	Evidence []string

	// ScreenHash is a hash of the screen content (for change detection).
	ScreenHash string
}

// Engine manages agent state detection and notifications.
type Engine struct {
	repo        *db.AgentRepository
	tmuxClient  *tmux.Client
	registry    *adapters.Registry
	subscribers map[string]Subscriber
	mu          sync.RWMutex
	logger      zerolog.Logger
}

// NewEngine creates a new StateEngine.
func NewEngine(repo *db.AgentRepository, tmuxClient *tmux.Client, registry *adapters.Registry) *Engine {
	return &Engine{
		repo:        repo,
		tmuxClient:  tmuxClient,
		registry:    registry,
		subscribers: make(map[string]Subscriber),
		logger:      logging.Component("state-engine"),
	}
}

// GetState retrieves the current state for an agent.
func (e *Engine) GetState(ctx context.Context, agentID string) (*models.StateInfo, error) {
	agent, err := e.repo.Get(ctx, agentID)
	if err != nil {
		if errors.Is(err, db.ErrAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	return &agent.StateInfo, nil
}

// UpdateState updates an agent's state and notifies subscribers.
func (e *Engine) UpdateState(ctx context.Context, agentID string, state models.AgentState, info models.StateInfo) error {
	agent, err := e.repo.Get(ctx, agentID)
	if err != nil {
		if errors.Is(err, db.ErrAgentNotFound) {
			return ErrAgentNotFound
		}
		return err
	}

	previousState := agent.State

	// Update agent state
	agent.State = state
	agent.StateInfo = info
	now := time.Now().UTC()
	agent.LastActivity = &now

	if err := e.repo.Update(ctx, agent); err != nil {
		return err
	}

	// Notify subscribers if state changed
	if previousState != state {
		change := StateChange{
			AgentID:       agentID,
			PreviousState: previousState,
			CurrentState:  state,
			StateInfo:     info,
			Timestamp:     now,
		}
		e.notifySubscribers(change)
	}

	return nil
}

// DetectState captures the current screen and detects the agent's state.
func (e *Engine) DetectState(ctx context.Context, agentID string) (*DetectionResult, error) {
	agent, err := e.repo.Get(ctx, agentID)
	if err != nil {
		if errors.Is(err, db.ErrAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	// Capture the current screen
	screen, err := e.tmuxClient.CapturePane(ctx, agent.TmuxPane, false)
	if err != nil {
		return nil, err
	}

	// Calculate screen hash for change detection
	screenHash := tmux.HashSnapshot(screen)

	// Get the appropriate adapter
	adapter := e.registry.GetByAgentType(agent.Type)
	if adapter == nil {
		// Fall back to generic detection
		adapter = e.registry.Get("generic")
	}

	if adapter == nil {
		// No adapter available, use basic heuristics
		return e.detectBasicState(screen, screenHash), nil
	}

	// Use adapter for state detection
	state, reason, err := adapter.DetectState(screen, nil)
	if err != nil {
		return nil, err
	}

	return &DetectionResult{
		State:      state,
		Confidence: reason.Confidence,
		Reason:     reason.Reason,
		Evidence:   reason.Evidence,
		ScreenHash: screenHash,
	}, nil
}

// DetectAndUpdate detects the current state and updates the agent.
func (e *Engine) DetectAndUpdate(ctx context.Context, agentID string) (*DetectionResult, error) {
	result, err := e.DetectState(ctx, agentID)
	if err != nil {
		return nil, err
	}

	info := models.StateInfo{
		State:      result.State,
		Confidence: result.Confidence,
		Reason:     result.Reason,
		Evidence:   result.Evidence,
		DetectedAt: time.Now().UTC(),
	}

	if err := e.UpdateState(ctx, agentID, result.State, info); err != nil {
		return nil, err
	}

	return result, nil
}

// Subscribe registers a subscriber for state change notifications.
func (e *Engine) Subscribe(id string, subscriber Subscriber) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.subscribers[id]; exists {
		return ErrSubscriberExists
	}

	e.subscribers[id] = subscriber
	e.logger.Debug().Str("subscriber_id", id).Msg("subscriber registered")
	return nil
}

// Unsubscribe removes a subscriber.
func (e *Engine) Unsubscribe(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.subscribers[id]; !exists {
		return ErrSubscriberMissing
	}

	delete(e.subscribers, id)
	e.logger.Debug().Str("subscriber_id", id).Msg("subscriber unregistered")
	return nil
}

// SubscribeFunc is a convenience method to subscribe with a function.
func (e *Engine) SubscribeFunc(id string, fn func(StateChange)) error {
	return e.Subscribe(id, SubscriberFunc(fn))
}

// notifySubscribers sends a state change to all subscribers.
func (e *Engine) notifySubscribers(change StateChange) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	e.logger.Debug().
		Str("agent_id", change.AgentID).
		Str("from", string(change.PreviousState)).
		Str("to", string(change.CurrentState)).
		Int("subscribers", len(e.subscribers)).
		Msg("notifying state change")

	for id, subscriber := range e.subscribers {
		go func(subID string, sub Subscriber) {
			defer func() {
				if r := recover(); r != nil {
					e.logger.Error().
						Str("subscriber_id", subID).
						Interface("panic", r).
						Msg("subscriber panicked")
				}
			}()
			sub.OnStateChange(change)
		}(id, subscriber)
	}
}

// detectBasicState provides fallback state detection without an adapter.
func (e *Engine) detectBasicState(screen, screenHash string) *DetectionResult {
	// Very basic heuristics
	result := &DetectionResult{
		State:      models.AgentStateIdle,
		Confidence: models.StateConfidenceLow,
		Reason:     "Basic heuristic detection",
		ScreenHash: screenHash,
	}

	// Look for common indicators
	if len(screen) == 0 {
		result.State = models.AgentStateStarting
		result.Reason = "Empty screen, agent may be starting"
		return result
	}

	// Check for error indicators
	if containsAny(screen, "error", "Error", "ERROR", "failed", "Failed") {
		result.State = models.AgentStateError
		result.Reason = "Error indicator found in output"
		return result
	}

	// Check for rate limit indicators
	if containsAny(screen, "rate limit", "Rate limit", "429", "too many requests") {
		result.State = models.AgentStateRateLimited
		result.Reason = "Rate limit indicator found"
		return result
	}

	// Check for approval indicators
	if containsAny(screen, "approve", "Approve", "confirm", "Confirm", "[y/n]", "(y/n)") {
		result.State = models.AgentStateAwaitingApproval
		result.Reason = "Approval prompt detected"
		return result
	}

	// Check for working indicators (spinners, progress)
	if containsAny(screen, "...", "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏", "thinking", "Thinking") {
		result.State = models.AgentStateWorking
		result.Confidence = models.StateConfidenceMedium
		result.Reason = "Working indicator detected"
		return result
	}

	// Default to idle
	result.Reason = "No special indicators found, assuming idle"
	return result
}

// containsAny checks if text contains any of the given substrings.
func containsAny(text string, substrings ...string) bool {
	for _, s := range substrings {
		if len(s) > 0 && len(text) >= len(s) {
			for i := 0; i <= len(text)-len(s); i++ {
				if text[i:i+len(s)] == s {
					return true
				}
			}
		}
	}
	return false
}

// WatchAgent starts a goroutine that periodically detects and updates state.
// Returns a cancel function to stop watching.
func (e *Engine) WatchAgent(ctx context.Context, agentID string, interval time.Duration) (cancel func()) {
	watchCtx, cancelFn := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				if _, err := e.DetectAndUpdate(watchCtx, agentID); err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					e.logger.Warn().Err(err).Str("agent_id", agentID).Msg("state detection failed")
				}
			}
		}
	}()

	return cancelFn
}
