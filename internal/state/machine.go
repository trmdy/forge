// Package state provides agent state management with validated transitions.
package state

import (
	"fmt"
	"sync"
	"time"

	"github.com/opencode-ai/swarm/internal/models"
)

// TransitionError is returned when an invalid state transition is attempted.
type TransitionError struct {
	AgentID   string
	FromState models.AgentState
	ToState   models.AgentState
	Reason    string
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("invalid state transition for agent %s: %s -> %s: %s",
		e.AgentID, e.FromState, e.ToState, e.Reason)
}

// TransitionEvent represents a state transition that occurred.
type TransitionEvent struct {
	AgentID    string
	FromState  models.AgentState
	ToState    models.AgentState
	Reason     string
	Confidence models.StateConfidence
	Evidence   []string
	Timestamp  time.Time
}

// TransitionCallback is called when a state transition occurs.
type TransitionCallback func(event TransitionEvent)

// validTransitions defines which state transitions are allowed.
// Map key is the current state, value is a set of valid target states.
var validTransitions = map[models.AgentState]map[models.AgentState]bool{
	models.AgentStateStarting: {
		models.AgentStateIdle:             true, // Successfully started
		models.AgentStateError:            true, // Failed to start
		models.AgentStateStopped:          true, // Terminated before ready
		models.AgentStateWorking:          true, // Started directly into work
		models.AgentStateAwaitingApproval: true, // Immediate permission request
	},
	models.AgentStateIdle: {
		models.AgentStateWorking:          true, // Received work
		models.AgentStateAwaitingApproval: true, // Permission needed
		models.AgentStatePaused:           true, // User paused
		models.AgentStateError:            true, // Error detected
		models.AgentStateStopped:          true, // Terminated
		models.AgentStateRateLimited:      true, // Rate limit hit
	},
	models.AgentStateWorking: {
		models.AgentStateIdle:             true, // Work completed
		models.AgentStateAwaitingApproval: true, // Permission needed mid-work
		models.AgentStatePaused:           true, // User paused
		models.AgentStateError:            true, // Error during work
		models.AgentStateStopped:          true, // Terminated
		models.AgentStateRateLimited:      true, // Rate limit hit
	},
	models.AgentStateAwaitingApproval: {
		models.AgentStateWorking: true, // Approved, continuing
		models.AgentStateIdle:    true, // Denied, back to idle
		models.AgentStatePaused:  true, // User paused
		models.AgentStateError:   true, // Error detected
		models.AgentStateStopped: true, // Terminated
	},
	models.AgentStateRateLimited: {
		models.AgentStateIdle:    true, // Cooldown ended
		models.AgentStateWorking: true, // Cooldown ended, resuming work
		models.AgentStatePaused:  true, // User paused during cooldown
		models.AgentStateError:   true, // Error detected
		models.AgentStateStopped: true, // Terminated
	},
	models.AgentStatePaused: {
		models.AgentStateIdle:        true, // Resumed
		models.AgentStateWorking:     true, // Resumed into work
		models.AgentStateError:       true, // Error while paused
		models.AgentStateStopped:     true, // Terminated
		models.AgentStateRateLimited: true, // Rate limit detected while paused
	},
	models.AgentStateError: {
		models.AgentStateIdle:     true, // Recovered
		models.AgentStateStarting: true, // Restarting
		models.AgentStateStopped:  true, // Terminated after error
	},
	models.AgentStateStopped: {
		models.AgentStateStarting: true, // Restarting
	},
}

// IsValidTransition checks if a state transition is allowed.
func IsValidTransition(from, to models.AgentState) bool {
	if from == to {
		return true // Same state is always valid (no-op)
	}
	targets, ok := validTransitions[from]
	if !ok {
		return false
	}
	return targets[to]
}

// ValidTargetStates returns the list of valid states that can be transitioned to from the given state.
func ValidTargetStates(from models.AgentState) []models.AgentState {
	targets, ok := validTransitions[from]
	if !ok {
		return nil
	}
	result := make([]models.AgentState, 0, len(targets))
	for state := range targets {
		result = append(result, state)
	}
	return result
}

// StateMachine manages agent state with validated transitions.
type StateMachine struct {
	mu        sync.RWMutex
	states    map[string]models.AgentState // agentID -> current state
	callbacks []TransitionCallback
	strict    bool // If true, invalid transitions panic in dev
}

// NewStateMachine creates a new state machine.
func NewStateMachine(strict bool) *StateMachine {
	return &StateMachine{
		states:    make(map[string]models.AgentState),
		callbacks: make([]TransitionCallback, 0),
		strict:    strict,
	}
}

// OnTransition registers a callback to be called on state transitions.
func (sm *StateMachine) OnTransition(cb TransitionCallback) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.callbacks = append(sm.callbacks, cb)
}

// GetState returns the current state for an agent.
func (sm *StateMachine) GetState(agentID string) (models.AgentState, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	state, ok := sm.states[agentID]
	return state, ok
}

// SetInitialState sets the initial state for a new agent (must be Starting or Stopped).
func (sm *StateMachine) SetInitialState(agentID string, state models.AgentState) error {
	if state != models.AgentStateStarting && state != models.AgentStateStopped {
		return &TransitionError{
			AgentID:   agentID,
			FromState: "",
			ToState:   state,
			Reason:    "initial state must be 'starting' or 'stopped'",
		}
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.states[agentID]; exists {
		return &TransitionError{
			AgentID:   agentID,
			FromState: sm.states[agentID],
			ToState:   state,
			Reason:    "agent already has a state; use Transition instead",
		}
	}

	sm.states[agentID] = state

	// Emit initial state event
	event := TransitionEvent{
		AgentID:    agentID,
		FromState:  "",
		ToState:    state,
		Reason:     "initial state",
		Confidence: models.StateConfidenceHigh,
		Timestamp:  time.Now(),
	}
	for _, cb := range sm.callbacks {
		cb(event)
	}

	return nil
}

// Transition attempts to transition an agent to a new state.
func (sm *StateMachine) Transition(agentID string, toState models.AgentState, reason string, confidence models.StateConfidence, evidence []string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	fromState, exists := sm.states[agentID]
	if !exists {
		return &TransitionError{
			AgentID:   agentID,
			FromState: "",
			ToState:   toState,
			Reason:    "agent not found; use SetInitialState first",
		}
	}

	// Same state is a no-op
	if fromState == toState {
		return nil
	}

	// Validate transition
	if !IsValidTransition(fromState, toState) {
		err := &TransitionError{
			AgentID:   agentID,
			FromState: fromState,
			ToState:   toState,
			Reason:    "transition not allowed",
		}
		if sm.strict {
			panic(err)
		}
		return err
	}

	// Perform transition
	sm.states[agentID] = toState

	// Emit transition event
	event := TransitionEvent{
		AgentID:    agentID,
		FromState:  fromState,
		ToState:    toState,
		Reason:     reason,
		Confidence: confidence,
		Evidence:   evidence,
		Timestamp:  time.Now(),
	}
	for _, cb := range sm.callbacks {
		cb(event)
	}

	return nil
}

// Remove removes an agent from the state machine.
func (sm *StateMachine) Remove(agentID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.states, agentID)
}

// AllStates returns a copy of all agent states.
func (sm *StateMachine) AllStates() map[string]models.AgentState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	result := make(map[string]models.AgentState, len(sm.states))
	for k, v := range sm.states {
		result[k] = v
	}
	return result
}

// StateInfo provides detailed information about a state.
type StateInfo struct {
	State       models.AgentState
	DisplayName string
	Description string
	IsBlocking  bool // True if this state blocks queue dispatch
	IsActive    bool // True if agent is considered active/running
	IsTerminal  bool // True if agent has stopped
}

// GetStateInfo returns detailed information about a state.
func GetStateInfo(state models.AgentState) StateInfo {
	switch state {
	case models.AgentStateStarting:
		return StateInfo{
			State:       state,
			DisplayName: "Starting",
			Description: "Agent is initializing",
			IsBlocking:  true,
			IsActive:    true,
			IsTerminal:  false,
		}
	case models.AgentStateIdle:
		return StateInfo{
			State:       state,
			DisplayName: "Idle",
			Description: "Agent is ready for work",
			IsBlocking:  false,
			IsActive:    true,
			IsTerminal:  false,
		}
	case models.AgentStateWorking:
		return StateInfo{
			State:       state,
			DisplayName: "Working",
			Description: "Agent is processing a task",
			IsBlocking:  true,
			IsActive:    true,
			IsTerminal:  false,
		}
	case models.AgentStateAwaitingApproval:
		return StateInfo{
			State:       state,
			DisplayName: "Awaiting Approval",
			Description: "Agent is waiting for user approval",
			IsBlocking:  true,
			IsActive:    true,
			IsTerminal:  false,
		}
	case models.AgentStateRateLimited:
		return StateInfo{
			State:       state,
			DisplayName: "Rate Limited",
			Description: "Agent is in cooldown due to rate limiting",
			IsBlocking:  true,
			IsActive:    true,
			IsTerminal:  false,
		}
	case models.AgentStatePaused:
		return StateInfo{
			State:       state,
			DisplayName: "Paused",
			Description: "Agent is paused by user",
			IsBlocking:  true,
			IsActive:    true,
			IsTerminal:  false,
		}
	case models.AgentStateError:
		return StateInfo{
			State:       state,
			DisplayName: "Error",
			Description: "Agent encountered an error",
			IsBlocking:  true,
			IsActive:    false,
			IsTerminal:  false,
		}
	case models.AgentStateStopped:
		return StateInfo{
			State:       state,
			DisplayName: "Stopped",
			Description: "Agent has been terminated",
			IsBlocking:  true,
			IsActive:    false,
			IsTerminal:  true,
		}
	default:
		return StateInfo{
			State:       state,
			DisplayName: string(state),
			Description: "Unknown state",
			IsBlocking:  true,
			IsActive:    false,
			IsTerminal:  false,
		}
	}
}

// CanDispatchTo returns true if the agent state allows queue dispatch.
func CanDispatchTo(state models.AgentState) bool {
	info := GetStateInfo(state)
	return !info.IsBlocking
}
