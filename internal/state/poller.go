// Package state provides agent state management and change notifications.
package state

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/logging"
	"github.com/tOgg1/forge/internal/models"
)

// Poller errors.
var (
	ErrPollerAlreadyRunning = errors.New("poller already running")
	ErrPollerNotRunning     = errors.New("poller not running")
)

// PollerConfig contains configuration for the state poller.
type PollerConfig struct {
	// ActiveInterval is how often to poll active agents.
	// Default: 500ms
	ActiveInterval time.Duration

	// IdleInterval is how often to poll idle agents.
	// Default: 2s
	IdleInterval time.Duration

	// InactiveInterval is how often to poll inactive agents (paused, stopped, error).
	// Default: 5s
	InactiveInterval time.Duration

	// MaxConcurrentPolls limits concurrent state detection operations.
	// Default: 10
	MaxConcurrentPolls int

	// FailureBackoffBase is the base delay for retry backoff after poll failures.
	// Default: 1s
	FailureBackoffBase time.Duration

	// FailureBackoffMax caps the retry backoff for repeated failures.
	// Default: 30s
	FailureBackoffMax time.Duration
}

// DefaultPollerConfig returns sensible defaults.
func DefaultPollerConfig() PollerConfig {
	return PollerConfig{
		ActiveInterval:     500 * time.Millisecond,
		IdleInterval:       2 * time.Second,
		InactiveInterval:   5 * time.Second,
		MaxConcurrentPolls: 10,
		FailureBackoffBase: 1 * time.Second,
		FailureBackoffMax:  30 * time.Second,
	}
}

// agentPollState tracks polling state for an agent.
type agentPollState struct {
	agentID      string
	lastPolledAt time.Time
	lastState    models.AgentState
	lastError    string
	lastErrorAt  time.Time
	failureCount int
	nextPollAt   time.Time
	stale        bool
	staleSince   time.Time
}

// Poller manages periodic state detection for all agents.
type Poller struct {
	config    PollerConfig
	engine    *Engine
	agentRepo *db.AgentRepository
	logger    zerolog.Logger

	mu         sync.RWMutex
	running    bool
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	pollSem    chan struct{}
	pollStates map[string]*agentPollState
}

// NewPoller creates a new state Poller.
func NewPoller(config PollerConfig, engine *Engine, agentRepo *db.AgentRepository) *Poller {
	if config.ActiveInterval <= 0 {
		config.ActiveInterval = DefaultPollerConfig().ActiveInterval
	}
	if config.IdleInterval <= 0 {
		config.IdleInterval = DefaultPollerConfig().IdleInterval
	}
	if config.InactiveInterval <= 0 {
		config.InactiveInterval = DefaultPollerConfig().InactiveInterval
	}
	if config.MaxConcurrentPolls <= 0 {
		config.MaxConcurrentPolls = DefaultPollerConfig().MaxConcurrentPolls
	}
	if config.FailureBackoffBase <= 0 {
		config.FailureBackoffBase = DefaultPollerConfig().FailureBackoffBase
	}
	if config.FailureBackoffMax <= 0 {
		config.FailureBackoffMax = DefaultPollerConfig().FailureBackoffMax
	}
	if config.FailureBackoffMax < config.FailureBackoffBase {
		config.FailureBackoffMax = config.FailureBackoffBase
	}

	return &Poller{
		config:     config,
		engine:     engine,
		agentRepo:  agentRepo,
		logger:     logging.Component("state-poller"),
		pollSem:    make(chan struct{}, config.MaxConcurrentPolls),
		pollStates: make(map[string]*agentPollState),
	}
}

// Start begins the polling loop.
func (p *Poller) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return ErrPollerAlreadyRunning
	}

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.running = true

	p.logger.Info().
		Dur("active_interval", p.config.ActiveInterval).
		Dur("idle_interval", p.config.IdleInterval).
		Dur("inactive_interval", p.config.InactiveInterval).
		Int("max_concurrent", p.config.MaxConcurrentPolls).
		Msg("state poller starting")

	// Start the main polling loop
	p.wg.Add(1)
	go p.runLoop()

	return nil
}

// Stop halts the polling loop.
func (p *Poller) Stop() error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return ErrPollerNotRunning
	}

	p.logger.Info().Msg("state poller stopping")
	p.cancel()
	p.running = false
	p.mu.Unlock()

	p.wg.Wait()
	p.logger.Info().Msg("state poller stopped")
	return nil
}

// IsRunning returns true if the poller is running.
func (p *Poller) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

// runLoop is the main polling loop.
func (p *Poller) runLoop() {
	defer p.wg.Done()

	// Use the shortest interval as the tick interval
	tickInterval := p.config.ActiveInterval
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.pollTick()
		}
	}
}

// pollTick performs one polling cycle.
func (p *Poller) pollTick() {
	ctx := p.ctx

	// Get all agents
	agents, err := p.agentRepo.List(ctx)
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to list agents for polling")
		return
	}

	now := time.Now()

	for _, agent := range agents {
		if p.shouldPoll(agent, now) {
			p.pollAgent(agent.ID)
		}
	}
}

// shouldPoll determines if an agent should be polled based on priority.
func (p *Poller) shouldPoll(agent *models.Agent, now time.Time) bool {
	p.mu.RLock()
	state, exists := p.pollStates[agent.ID]
	p.mu.RUnlock()

	if exists && !state.nextPollAt.IsZero() && now.Before(state.nextPollAt) {
		return false
	}

	var interval time.Duration
	switch {
	case agent.State == models.AgentStateWorking:
		// Active agents get polled most frequently
		interval = p.config.ActiveInterval
	case agent.State == models.AgentStateIdle || agent.State == models.AgentStateAwaitingApproval:
		// Idle agents get polled less frequently
		interval = p.config.IdleInterval
	default:
		// Inactive agents (paused, stopped, error, starting) get polled least frequently
		interval = p.config.InactiveInterval
	}

	if !exists {
		// Never polled before, should poll
		return true
	}

	return now.Sub(state.lastPolledAt) >= interval
}

// pollAgent triggers state detection for an agent.
func (p *Poller) pollAgent(agentID string) {
	// Acquire semaphore
	select {
	case p.pollSem <- struct{}{}:
	default:
		// Max concurrent polls reached, skip this one
		return
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer func() { <-p.pollSem }()

		p.doPoll(agentID)
	}()
}

// doPoll performs the actual state detection.
func (p *Poller) doPoll(agentID string) {
	ctx := p.ctx

	result, err := p.engine.DetectAndUpdate(ctx, agentID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		p.recordPollFailure(agentID, err)
		p.logger.Warn().Err(err).Str("agent_id", agentID).Msg("state detection failed")
		return
	}

	// Update poll state
	p.recordPollSuccess(agentID, result.State)

	p.logger.Debug().
		Str("agent_id", agentID).
		Str("state", string(result.State)).
		Str("confidence", string(result.Confidence)).
		Msg("polled agent state")
}

// PollNow triggers an immediate poll for a specific agent.
func (p *Poller) PollNow(agentID string) error {
	p.mu.RLock()
	running := p.running
	p.mu.RUnlock()

	if !running {
		return ErrPollerNotRunning
	}

	p.pollAgent(agentID)
	return nil
}

// GetLastPollTime returns when an agent was last polled.
func (p *Poller) GetLastPollTime(agentID string) (time.Time, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	state, exists := p.pollStates[agentID]
	if !exists {
		return time.Time{}, false
	}
	return state.lastPolledAt, true
}

// IsStale reports whether the agent's last poll attempt failed and is stale.
func (p *Poller) IsStale(agentID string) (bool, time.Time) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	state, exists := p.pollStates[agentID]
	if !exists || !state.stale {
		return false, time.Time{}
	}
	return true, state.staleSince
}

func (p *Poller) recordPollFailure(agentID string, err error) {
	now := time.Now()

	p.mu.Lock()
	defer p.mu.Unlock()

	state := p.pollStates[agentID]
	if state == nil {
		state = &agentPollState{agentID: agentID}
		p.pollStates[agentID] = state
	}

	state.lastPolledAt = now
	state.lastError = err.Error()
	state.lastErrorAt = now
	state.failureCount++
	if !state.stale {
		state.stale = true
		state.staleSince = now
	}
	backoff := p.backoffDuration(state.failureCount)
	if backoff > 0 {
		state.nextPollAt = now.Add(backoff)
	} else {
		state.nextPollAt = time.Time{}
	}
}

func (p *Poller) recordPollSuccess(agentID string, stateVal models.AgentState) {
	now := time.Now()

	p.mu.Lock()
	defer p.mu.Unlock()

	state := p.pollStates[agentID]
	if state == nil {
		state = &agentPollState{agentID: agentID}
		p.pollStates[agentID] = state
	}

	state.lastPolledAt = now
	state.lastState = stateVal
	state.lastError = ""
	state.lastErrorAt = time.Time{}
	state.failureCount = 0
	state.nextPollAt = time.Time{}
	state.stale = false
	state.staleSince = time.Time{}
}

func (p *Poller) backoffDuration(failureCount int) time.Duration {
	if failureCount <= 0 {
		return 0
	}

	base := p.config.FailureBackoffBase
	if base <= 0 {
		return 0
	}
	max := p.config.FailureBackoffMax
	if max <= 0 {
		max = base
	}

	backoff := base
	for i := 1; i < failureCount; i++ {
		if backoff >= max {
			return max
		}
		backoff *= 2
	}
	if backoff > max {
		backoff = max
	}
	return backoff
}

// ClearPollState removes poll state for an agent (e.g., when agent is terminated).
func (p *Poller) ClearPollState(agentID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pollStates, agentID)
}
