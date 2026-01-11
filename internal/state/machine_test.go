package state

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tOgg1/forge/internal/models"
)

func TestIsValidTransition(t *testing.T) {
	tests := []struct {
		name  string
		from  models.AgentState
		to    models.AgentState
		valid bool
	}{
		// Starting transitions
		{"starting to idle", models.AgentStateStarting, models.AgentStateIdle, true},
		{"starting to error", models.AgentStateStarting, models.AgentStateError, true},
		{"starting to stopped", models.AgentStateStarting, models.AgentStateStopped, true},
		{"starting to working", models.AgentStateStarting, models.AgentStateWorking, true},
		{"starting to paused invalid", models.AgentStateStarting, models.AgentStatePaused, false},

		// Idle transitions
		{"idle to working", models.AgentStateIdle, models.AgentStateWorking, true},
		{"idle to awaiting approval", models.AgentStateIdle, models.AgentStateAwaitingApproval, true},
		{"idle to paused", models.AgentStateIdle, models.AgentStatePaused, true},
		{"idle to rate limited", models.AgentStateIdle, models.AgentStateRateLimited, true},
		{"idle to starting invalid", models.AgentStateIdle, models.AgentStateStarting, false},

		// Working transitions
		{"working to idle", models.AgentStateWorking, models.AgentStateIdle, true},
		{"working to awaiting approval", models.AgentStateWorking, models.AgentStateAwaitingApproval, true},
		{"working to error", models.AgentStateWorking, models.AgentStateError, true},
		{"working to starting invalid", models.AgentStateWorking, models.AgentStateStarting, false},

		// Awaiting approval transitions
		{"awaiting to working", models.AgentStateAwaitingApproval, models.AgentStateWorking, true},
		{"awaiting to idle", models.AgentStateAwaitingApproval, models.AgentStateIdle, true},
		{"awaiting to rate limited invalid", models.AgentStateAwaitingApproval, models.AgentStateRateLimited, false},

		// Rate limited transitions
		{"rate limited to idle", models.AgentStateRateLimited, models.AgentStateIdle, true},
		{"rate limited to working", models.AgentStateRateLimited, models.AgentStateWorking, true},
		{"rate limited to starting invalid", models.AgentStateRateLimited, models.AgentStateStarting, false},

		// Paused transitions
		{"paused to idle", models.AgentStatePaused, models.AgentStateIdle, true},
		{"paused to working", models.AgentStatePaused, models.AgentStateWorking, true},
		{"paused to starting invalid", models.AgentStatePaused, models.AgentStateStarting, false},

		// Error transitions
		{"error to idle", models.AgentStateError, models.AgentStateIdle, true},
		{"error to starting", models.AgentStateError, models.AgentStateStarting, true},
		{"error to stopped", models.AgentStateError, models.AgentStateStopped, true},
		{"error to working invalid", models.AgentStateError, models.AgentStateWorking, false},

		// Stopped transitions
		{"stopped to starting", models.AgentStateStopped, models.AgentStateStarting, true},
		{"stopped to idle invalid", models.AgentStateStopped, models.AgentStateIdle, false},
		{"stopped to working invalid", models.AgentStateStopped, models.AgentStateWorking, false},

		// Same state is always valid
		{"idle to idle", models.AgentStateIdle, models.AgentStateIdle, true},
		{"working to working", models.AgentStateWorking, models.AgentStateWorking, true},
		{"stopped to stopped", models.AgentStateStopped, models.AgentStateStopped, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidTransition(tt.from, tt.to)
			assert.Equal(t, tt.valid, result, "IsValidTransition(%s, %s)", tt.from, tt.to)
		})
	}
}

func TestValidTargetStates(t *testing.T) {
	// Test that idle has expected targets
	targets := ValidTargetStates(models.AgentStateIdle)
	assert.Contains(t, targets, models.AgentStateWorking)
	assert.Contains(t, targets, models.AgentStatePaused)
	assert.NotContains(t, targets, models.AgentStateStarting)

	// Test unknown state returns nil
	targets = ValidTargetStates("unknown_state")
	assert.Nil(t, targets)
}

func TestStateMachine_SetInitialState(t *testing.T) {
	sm := NewStateMachine(false)

	// Valid initial states
	err := sm.SetInitialState("agent-1", models.AgentStateStarting)
	assert.NoError(t, err)

	err = sm.SetInitialState("agent-2", models.AgentStateStopped)
	assert.NoError(t, err)

	// Invalid initial state
	err = sm.SetInitialState("agent-3", models.AgentStateIdle)
	assert.Error(t, err)
	var transErr *TransitionError
	assert.ErrorAs(t, err, &transErr)

	// Cannot set initial state twice
	err = sm.SetInitialState("agent-1", models.AgentStateStarting)
	assert.Error(t, err)
}

func TestStateMachine_Transition(t *testing.T) {
	sm := NewStateMachine(false)

	// Setup agent
	err := sm.SetInitialState("agent-1", models.AgentStateStarting)
	require.NoError(t, err)

	// Valid transition
	err = sm.Transition("agent-1", models.AgentStateIdle, "started successfully", models.StateConfidenceHigh, nil)
	assert.NoError(t, err)

	state, ok := sm.GetState("agent-1")
	assert.True(t, ok)
	assert.Equal(t, models.AgentStateIdle, state)

	// Another valid transition
	err = sm.Transition("agent-1", models.AgentStateWorking, "received task", models.StateConfidenceMedium, []string{"evidence"})
	assert.NoError(t, err)

	state, _ = sm.GetState("agent-1")
	assert.Equal(t, models.AgentStateWorking, state)

	// Invalid transition (working -> starting)
	err = sm.Transition("agent-1", models.AgentStateStarting, "invalid", models.StateConfidenceLow, nil)
	assert.Error(t, err)

	// State should not have changed
	state, _ = sm.GetState("agent-1")
	assert.Equal(t, models.AgentStateWorking, state)

	// Same state is no-op
	err = sm.Transition("agent-1", models.AgentStateWorking, "same state", models.StateConfidenceHigh, nil)
	assert.NoError(t, err)
}

func TestStateMachine_TransitionUnknownAgent(t *testing.T) {
	sm := NewStateMachine(false)

	err := sm.Transition("unknown-agent", models.AgentStateIdle, "test", models.StateConfidenceHigh, nil)
	assert.Error(t, err)
	var transErr *TransitionError
	assert.ErrorAs(t, err, &transErr)
	assert.Contains(t, transErr.Reason, "agent not found")
}

func TestStateMachine_StrictMode(t *testing.T) {
	sm := NewStateMachine(true)

	err := sm.SetInitialState("agent-1", models.AgentStateStarting)
	require.NoError(t, err)

	err = sm.Transition("agent-1", models.AgentStateIdle, "ready", models.StateConfidenceHigh, nil)
	require.NoError(t, err)

	// Invalid transition should panic in strict mode
	assert.Panics(t, func() {
		_ = sm.Transition("agent-1", models.AgentStateStarting, "invalid", models.StateConfidenceLow, nil)
	})
}

func TestStateMachine_Callbacks(t *testing.T) {
	sm := NewStateMachine(false)

	var events []TransitionEvent
	var mu sync.Mutex

	sm.OnTransition(func(event TransitionEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Initial state should trigger callback
	err := sm.SetInitialState("agent-1", models.AgentStateStarting)
	require.NoError(t, err)

	mu.Lock()
	assert.Len(t, events, 1)
	assert.Equal(t, models.AgentState(""), events[0].FromState)
	assert.Equal(t, models.AgentStateStarting, events[0].ToState)
	mu.Unlock()

	// Transition should trigger callback
	err = sm.Transition("agent-1", models.AgentStateIdle, "ready", models.StateConfidenceHigh, []string{"prompt detected"})
	require.NoError(t, err)

	mu.Lock()
	assert.Len(t, events, 2)
	assert.Equal(t, models.AgentStateStarting, events[1].FromState)
	assert.Equal(t, models.AgentStateIdle, events[1].ToState)
	assert.Equal(t, "ready", events[1].Reason)
	assert.Equal(t, []string{"prompt detected"}, events[1].Evidence)
	mu.Unlock()

	// Same state should not trigger callback
	err = sm.Transition("agent-1", models.AgentStateIdle, "no change", models.StateConfidenceHigh, nil)
	require.NoError(t, err)

	mu.Lock()
	assert.Len(t, events, 2) // Still 2
	mu.Unlock()
}

func TestStateMachine_Remove(t *testing.T) {
	sm := NewStateMachine(false)

	err := sm.SetInitialState("agent-1", models.AgentStateStarting)
	require.NoError(t, err)

	_, ok := sm.GetState("agent-1")
	assert.True(t, ok)

	sm.Remove("agent-1")

	_, ok = sm.GetState("agent-1")
	assert.False(t, ok)
}

func TestStateMachine_AllStates(t *testing.T) {
	sm := NewStateMachine(false)

	_ = sm.SetInitialState("agent-1", models.AgentStateStarting)
	_ = sm.SetInitialState("agent-2", models.AgentStateStopped)
	_ = sm.Transition("agent-1", models.AgentStateIdle, "ready", models.StateConfidenceHigh, nil)

	states := sm.AllStates()
	assert.Len(t, states, 2)
	assert.Equal(t, models.AgentStateIdle, states["agent-1"])
	assert.Equal(t, models.AgentStateStopped, states["agent-2"])

	// Modifying returned map shouldn't affect internal state
	states["agent-1"] = models.AgentStateError
	internal, _ := sm.GetState("agent-1")
	assert.Equal(t, models.AgentStateIdle, internal)
}

func TestStateMachine_Concurrent(t *testing.T) {
	sm := NewStateMachine(false)

	// Create multiple agents
	for i := 0; i < 10; i++ {
		agentID := fmt.Sprintf("agent-%d", i)
		err := sm.SetInitialState(agentID, models.AgentStateStarting)
		require.NoError(t, err)
	}

	// Concurrent transitions
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			agentID := fmt.Sprintf("agent-%d", id)

			// Multiple transitions per agent
			_ = sm.Transition(agentID, models.AgentStateIdle, "ready", models.StateConfidenceHigh, nil)
			_ = sm.Transition(agentID, models.AgentStateWorking, "task", models.StateConfidenceMedium, nil)
			_ = sm.Transition(agentID, models.AgentStateIdle, "done", models.StateConfidenceHigh, nil)
		}(i)
	}

	wg.Wait()

	// All agents should be in idle state
	states := sm.AllStates()
	for i := 0; i < 10; i++ {
		agentID := fmt.Sprintf("agent-%d", i)
		assert.Equal(t, models.AgentStateIdle, states[agentID])
	}
}

func TestGetStateInfo(t *testing.T) {
	tests := []struct {
		state      models.AgentState
		isBlocking bool
		isActive   bool
		isTerminal bool
	}{
		{models.AgentStateStarting, true, true, false},
		{models.AgentStateIdle, false, true, false},
		{models.AgentStateWorking, true, true, false},
		{models.AgentStateAwaitingApproval, true, true, false},
		{models.AgentStateRateLimited, true, true, false},
		{models.AgentStatePaused, true, true, false},
		{models.AgentStateError, true, false, false},
		{models.AgentStateStopped, true, false, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			info := GetStateInfo(tt.state)
			assert.Equal(t, tt.state, info.State)
			assert.Equal(t, tt.isBlocking, info.IsBlocking, "IsBlocking")
			assert.Equal(t, tt.isActive, info.IsActive, "IsActive")
			assert.Equal(t, tt.isTerminal, info.IsTerminal, "IsTerminal")
			assert.NotEmpty(t, info.DisplayName)
			assert.NotEmpty(t, info.Description)
		})
	}
}

func TestCanDispatchTo(t *testing.T) {
	// Only idle state allows dispatch
	assert.True(t, CanDispatchTo(models.AgentStateIdle))
	assert.False(t, CanDispatchTo(models.AgentStateWorking))
	assert.False(t, CanDispatchTo(models.AgentStateStarting))
	assert.False(t, CanDispatchTo(models.AgentStatePaused))
	assert.False(t, CanDispatchTo(models.AgentStateStopped))
}
