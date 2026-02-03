// Package state provides agent state management and change notifications.
package state

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/adapters"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/logging"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/tmux"
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

	// UsageMetrics contains parsed usage metrics when available.
	UsageMetrics *models.UsageMetrics

	// DiffMetadata contains parsed diff metadata when available.
	DiffMetadata *models.DiffMetadata

	// ProcessStats contains process resource metrics when available.
	ProcessStats *models.ProcessStats
}

// Engine manages agent state detection and notifications.
type Engine struct {
	repo           *db.AgentRepository
	eventRepo      *db.EventRepository
	tmuxClient     *tmux.Client
	registry       *adapters.Registry
	subscribers    map[string]Subscriber
	statsCollector *ProcessStatsCollector
	mu             sync.RWMutex
	logger         zerolog.Logger
}

// NewEngine creates a new StateEngine.
func NewEngine(repo *db.AgentRepository, eventRepo *db.EventRepository, tmuxClient *tmux.Client, registry *adapters.Registry) *Engine {
	return &Engine{
		repo:           repo,
		eventRepo:      eventRepo,
		tmuxClient:     tmuxClient,
		registry:       registry,
		subscribers:    make(map[string]Subscriber),
		statsCollector: NewProcessStatsCollector(),
		logger:         logging.Component("state-engine"),
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

// GetAgent retrieves the full agent record by ID.
func (e *Engine) GetAgent(ctx context.Context, agentID string) (*models.Agent, error) {
	agent, err := e.repo.Get(ctx, agentID)
	if err != nil {
		if errors.Is(err, db.ErrAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	return agent, nil
}

// ListAgents retrieves all agents from the repository.
func (e *Engine) ListAgents(ctx context.Context) ([]*models.Agent, error) {
	if e.repo == nil {
		return nil, nil
	}
	return e.repo.List(ctx)
}

// UpdateState updates an agent's state and notifies subscribers.
func (e *Engine) UpdateState(ctx context.Context, agentID string, state models.AgentState, info models.StateInfo, usage *models.UsageMetrics, diff *models.DiffMetadata) error {
	return e.UpdateStateWithStats(ctx, agentID, state, info, usage, diff, nil)
}

// UpdateStateWithStats updates an agent's state with optional process stats.
func (e *Engine) UpdateStateWithStats(ctx context.Context, agentID string, state models.AgentState, info models.StateInfo, usage *models.UsageMetrics, diff *models.DiffMetadata, stats *models.ProcessStats) error {
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
	if usage != nil {
		agent.Metadata.UsageMetrics = usage
	}
	if diff != nil {
		agent.Metadata.DiffMetadata = diff
	}
	if stats != nil {
		agent.Metadata.ProcessStats = stats
	}

	if previousState != state && e.eventRepo != nil {
		event, err := buildStateChangeEvent(agentID, previousState, state, info, now)
		if err != nil {
			return err
		}
		if err := e.repo.UpdateWithEvent(ctx, agent, event, e.eventRepo); err != nil {
			return err
		}
	} else if err := e.repo.Update(ctx, agent); err != nil {
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
	snapshot, err := CaptureSnapshot(ctx, e.tmuxClient, agent.TmuxPane, false)
	if err != nil {
		return nil, err
	}

	screen := snapshot.Content
	screenHash := snapshot.Hash

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

	// Use adapter for state detection, passing agent metadata for richer detection
	state, reason, err := adapter.DetectState(screen, agent.Metadata)
	if err != nil {
		return nil, err
	}

	var usage *models.UsageMetrics
	if extractor, ok := adapter.(adapters.UsageMetricsExtractor); ok {
		metrics, matched, err := extractor.ExtractUsageMetrics(screen)
		if err != nil {
			e.logger.Debug().Err(err).Str("agent_id", agentID).Msg("failed to extract usage metrics")
		} else if matched && metrics != nil {
			usage = metrics
		}
	}

	var diff *models.DiffMetadata
	if extractor, ok := adapter.(adapters.DiffMetadataExtractor); ok {
		metadata, matched, err := extractor.ExtractDiffMetadata(screen)
		if err != nil {
			e.logger.Debug().Err(err).Str("agent_id", agentID).Msg("failed to extract diff metadata")
		} else if matched && metadata != nil {
			diff = metadata
		}
	}

	// Collect process stats if PID is known
	var processStats *models.ProcessStats
	if agent.Metadata.PID > 0 && e.statsCollector != nil {
		processStats = e.statsCollector.Collect(agent.Metadata.PID)
	}

	result := &DetectionResult{
		State:        state,
		Confidence:   reason.Confidence,
		Reason:       reason.Reason,
		Evidence:     reason.Evidence,
		ScreenHash:   screenHash,
		UsageMetrics: usage,
		DiffMetadata: diff,
		ProcessStats: processStats,
	}

	// Apply rule-based inference on top of adapter result when needed.
	ApplyRuleBasedInference(result, screen)
	return result, nil
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

	if err := e.UpdateStateWithStats(ctx, agentID, result.State, info, result.UsageMetrics, result.DiffMetadata, result.ProcessStats); err != nil {
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

// logStateChange logs a state change to the event repository.
func (e *Engine) logStateChange(ctx context.Context, agentID string, oldState, newState models.AgentState, info models.StateInfo) error {
	if e.eventRepo == nil {
		return nil
	}

	event, err := buildStateChangeEvent(agentID, oldState, newState, info, time.Time{})
	if err != nil {
		return err
	}

	return e.eventRepo.Create(ctx, event)
}

var _ = (*Engine).logStateChange

func buildStateChangeEvent(agentID string, oldState, newState models.AgentState, info models.StateInfo, timestamp time.Time) (*models.Event, error) {
	payload := models.StateChangedPayload{
		OldState:   oldState,
		NewState:   newState,
		Confidence: info.Confidence,
		Reason:     info.Reason,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	event := &models.Event{
		Type:       models.EventTypeAgentStateChanged,
		EntityType: models.EntityTypeAgent,
		EntityID:   agentID,
		Payload:    payloadBytes,
	}
	if !timestamp.IsZero() {
		event.Timestamp = timestamp
	}
	return event, nil
}
