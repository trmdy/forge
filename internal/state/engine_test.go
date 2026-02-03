package state

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tOgg1/forge/internal/models"
)

// TestDetectBasicState tests the fallback state detection heuristics.
func TestDetectBasicState(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		name           string
		screen         string
		expectedState  models.AgentState
		expectedConf   models.StateConfidence
		reasonContains string
	}{
		{
			name:           "empty screen indicates starting",
			screen:         "",
			expectedState:  models.AgentStateStarting,
			expectedConf:   models.StateConfidenceLow,
			reasonContains: "starting",
		},
		{
			name:           "error indicator lowercase",
			screen:         "Something went wrong: error occurred",
			expectedState:  models.AgentStateError,
			expectedConf:   models.StateConfidenceLow,
			reasonContains: "Error",
		},
		{
			name:           "error indicator uppercase",
			screen:         "FATAL ERROR: connection failed",
			expectedState:  models.AgentStateError,
			expectedConf:   models.StateConfidenceLow,
			reasonContains: "Error",
		},
		{
			name:           "failed indicator",
			screen:         "Build failed with exit code 1",
			expectedState:  models.AgentStateError,
			expectedConf:   models.StateConfidenceLow,
			reasonContains: "Error",
		},
		{
			name:           "rate limit text",
			screen:         "API rate limit exceeded, please wait",
			expectedState:  models.AgentStateRateLimited,
			expectedConf:   models.StateConfidenceLow,
			reasonContains: "Rate limit",
		},
		{
			name:           "rate limit 429",
			screen:         "HTTP 429: Too many requests",
			expectedState:  models.AgentStateRateLimited,
			expectedConf:   models.StateConfidenceLow,
			reasonContains: "Rate limit",
		},
		{
			name:           "approval prompt y/n",
			screen:         "Do you want to continue? [y/n]",
			expectedState:  models.AgentStateAwaitingApproval,
			expectedConf:   models.StateConfidenceLow,
			reasonContains: "Approval",
		},
		{
			name:           "approval prompt confirm",
			screen:         "Please confirm this action",
			expectedState:  models.AgentStateAwaitingApproval,
			expectedConf:   models.StateConfidenceLow,
			reasonContains: "Approval",
		},
		{
			name:           "spinner indicator",
			screen:         "Processing... ⠋ loading data",
			expectedState:  models.AgentStateWorking,
			expectedConf:   models.StateConfidenceMedium,
			reasonContains: "Working",
		},
		{
			name:           "thinking indicator",
			screen:         "Claude is thinking about your request",
			expectedState:  models.AgentStateWorking,
			expectedConf:   models.StateConfidenceMedium,
			reasonContains: "Working",
		},
		{
			name:           "ellipsis working",
			screen:         "Generating response...",
			expectedState:  models.AgentStateWorking,
			expectedConf:   models.StateConfidenceMedium,
			reasonContains: "Working",
		},
		{
			name:           "default to idle",
			screen:         "Ready for input",
			expectedState:  models.AgentStateIdle,
			expectedConf:   models.StateConfidenceLow,
			reasonContains: "idle",
		},
		{
			name:           "normal output defaults to idle",
			screen:         "$ ls -la\ntotal 0\ndrwxr-xr-x 2 user user 40 Dec 22 10:00 .",
			expectedState:  models.AgentStateIdle,
			expectedConf:   models.StateConfidenceLow,
			reasonContains: "idle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.detectBasicState(tt.screen, "testhash")

			assert.Equal(t, tt.expectedState, result.State, "unexpected state")
			assert.Equal(t, tt.expectedConf, result.Confidence, "unexpected confidence")
			assert.Contains(t, result.Reason, tt.reasonContains, "reason should contain expected text")
			assert.Equal(t, "testhash", result.ScreenHash, "screen hash should be preserved")
		})
	}
}

// TestDetectBasicStatePriority tests that error states take priority.
func TestDetectBasicStatePriority(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		name          string
		screen        string
		expectedState models.AgentState
	}{
		{
			name:          "error takes priority over working",
			screen:        "Processing... Error: connection timeout",
			expectedState: models.AgentStateError,
		},
		{
			name:          "rate limit takes priority over approval",
			screen:        "Please approve [y/n] - rate limit reached",
			expectedState: models.AgentStateRateLimited,
		},
		{
			name:          "error takes priority over rate limit",
			screen:        "Error: 429 rate limit - failed to connect",
			expectedState: models.AgentStateError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.detectBasicState(tt.screen, "hash")
			assert.Equal(t, tt.expectedState, result.State)
		})
	}
}

// TestContainsAny tests the string matching helper.
func TestContainsAny(t *testing.T) {
	tests := []struct {
		text       string
		substrings []string
		expected   bool
	}{
		{"hello world", []string{"hello"}, true},
		{"hello world", []string{"world"}, true},
		{"hello world", []string{"foo", "bar"}, false},
		{"hello world", []string{"foo", "hello"}, true},
		{"", []string{"foo"}, false},
		{"hello", []string{""}, false},
		{"abc", []string{"abcd"}, false},
		{"Rate limit exceeded", []string{"rate limit", "Rate limit"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := containsAny(tt.text, tt.substrings...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSubscribeUnsubscribe tests subscriber management.
func TestSubscribeUnsubscribe(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil)

	// Subscribe
	err := engine.Subscribe("test-sub", SubscriberFunc(func(change StateChange) {}))
	require.NoError(t, err)

	// Duplicate subscribe should fail
	err = engine.Subscribe("test-sub", SubscriberFunc(func(change StateChange) {}))
	assert.ErrorIs(t, err, ErrSubscriberExists)

	// Unsubscribe
	err = engine.Unsubscribe("test-sub")
	require.NoError(t, err)

	// Unsubscribe non-existent should fail
	err = engine.Unsubscribe("test-sub")
	assert.ErrorIs(t, err, ErrSubscriberMissing)

	// Re-subscribe should work after unsubscribe
	err = engine.Subscribe("test-sub", SubscriberFunc(func(change StateChange) {}))
	require.NoError(t, err)
}

// TestSubscribeFuncConvenience tests the SubscribeFunc helper.
func TestSubscribeFuncConvenience(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil)

	called := make(chan struct{}, 1)
	err := engine.SubscribeFunc("test", func(change StateChange) {
		select {
		case called <- struct{}{}:
		default:
		}
	})
	require.NoError(t, err)

	// Manually notify to test
	engine.notifySubscribers(StateChange{
		AgentID:       "agent-1",
		PreviousState: models.AgentStateIdle,
		CurrentState:  models.AgentStateWorking,
	})

	select {
	case <-called:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("subscriber function should have been called")
	}
}

// TestNotifySubscribersConcurrency tests concurrent subscriber notifications.
func TestNotifySubscribersConcurrency(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil)

	var mu sync.Mutex
	received := make(map[string]int)

	// Add multiple subscribers
	for i := 0; i < 5; i++ {
		id := string(rune('A' + i))
		err := engine.SubscribeFunc(id, func(change StateChange) {
			mu.Lock()
			received[id]++
			mu.Unlock()
		})
		require.NoError(t, err)
	}

	// Send multiple notifications
	for i := 0; i < 10; i++ {
		engine.notifySubscribers(StateChange{
			AgentID:       "agent-1",
			PreviousState: models.AgentStateIdle,
			CurrentState:  models.AgentStateWorking,
		})
	}

	// Wait for all notifications
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Each subscriber should receive all notifications
	for id := range received {
		assert.Equal(t, 10, received[id], "subscriber %s should receive all notifications", id)
	}
}

// TestSubscriberPanicRecovery tests that panicking subscribers don't crash the engine.
func TestSubscriberPanicRecovery(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil)

	goodCalled := make(chan struct{}, 1)

	// Add a panicking subscriber
	err := engine.SubscribeFunc("panicker", func(change StateChange) {
		panic("intentional panic")
	})
	require.NoError(t, err)

	// Add a good subscriber
	err = engine.SubscribeFunc("good", func(change StateChange) {
		select {
		case goodCalled <- struct{}{}:
		default:
		}
	})
	require.NoError(t, err)

	// Should not panic
	engine.notifySubscribers(StateChange{
		AgentID:       "agent-1",
		PreviousState: models.AgentStateIdle,
		CurrentState:  models.AgentStateWorking,
	})

	select {
	case <-goodCalled:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("good subscriber should still be called despite panic")
	}
}

// TestStateChangeStruct tests StateChange field population.
func TestStateChangeStruct(t *testing.T) {
	now := time.Now()
	change := StateChange{
		AgentID:       "agent-123",
		PreviousState: models.AgentStateIdle,
		CurrentState:  models.AgentStateWorking,
		StateInfo: models.StateInfo{
			State:      models.AgentStateWorking,
			Confidence: models.StateConfidenceHigh,
			Reason:     "detected activity",
			Evidence:   []string{"spinner visible", "output changing"},
		},
		Timestamp: now,
	}

	assert.Equal(t, "agent-123", change.AgentID)
	assert.Equal(t, models.AgentStateIdle, change.PreviousState)
	assert.Equal(t, models.AgentStateWorking, change.CurrentState)
	assert.Equal(t, models.StateConfidenceHigh, change.StateInfo.Confidence)
	assert.Len(t, change.StateInfo.Evidence, 2)
	assert.Equal(t, now, change.Timestamp)
}

// TestDetectionResultStruct tests DetectionResult field population.
func TestDetectionResultStruct(t *testing.T) {
	result := DetectionResult{
		State:      models.AgentStateWorking,
		Confidence: models.StateConfidenceMedium,
		Reason:     "spinner detected",
		Evidence:   []string{"⠋ visible"},
		ScreenHash: "abc123",
		UsageMetrics: &models.UsageMetrics{
			TotalTokens: 1000,
			Sessions:    5,
		},
		DiffMetadata: &models.DiffMetadata{
			FilesChanged: 3,
			Insertions:   50,
			Deletions:    10,
		},
	}

	assert.Equal(t, models.AgentStateWorking, result.State)
	assert.Equal(t, models.StateConfidenceMedium, result.Confidence)
	assert.NotNil(t, result.UsageMetrics)
	assert.Equal(t, int64(1000), result.UsageMetrics.TotalTokens)
	assert.NotNil(t, result.DiffMetadata)
	assert.Equal(t, int64(3), result.DiffMetadata.FilesChanged)
}

// TestEngineErrorConstants tests that error constants are defined.
func TestEngineErrorConstants(t *testing.T) {
	assert.NotNil(t, ErrAgentNotFound)
	assert.NotNil(t, ErrSubscriberExists)
	assert.NotNil(t, ErrSubscriberMissing)

	assert.EqualError(t, ErrAgentNotFound, "agent not found")
	assert.EqualError(t, ErrSubscriberExists, "subscriber already exists")
	assert.EqualError(t, ErrSubscriberMissing, "subscriber not found")
}

// TestNewEngine tests engine initialization.
func TestNewEngine(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil)

	require.NotNil(t, engine)
	assert.NotNil(t, engine.subscribers)
	assert.Empty(t, engine.subscribers)
}

// testdataPath returns the path to a test fixture file.
func testdataPath(t *testing.T, filename string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get caller info")
	return filepath.Join(filepath.Dir(file), "testdata", filename)
}

// readTestdata reads a test fixture file.
func readTestdata(t *testing.T, filename string) string {
	t.Helper()
	path := testdataPath(t, filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read testdata: %s", path)
	return string(data)
}

// TestDetectBasicStateGolden tests state detection with fixture files.
func TestDetectBasicStateGolden(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		filename      string
		expectedState models.AgentState
		minConfidence models.StateConfidence
	}{
		{
			filename:      "screen_rate_limited.txt",
			expectedState: models.AgentStateRateLimited,
			minConfidence: models.StateConfidenceLow,
		},
		{
			filename:      "screen_working.txt",
			expectedState: models.AgentStateWorking,
			minConfidence: models.StateConfidenceMedium,
		},
		{
			filename:      "screen_approval.txt",
			expectedState: models.AgentStateAwaitingApproval,
			minConfidence: models.StateConfidenceLow,
		},
		{
			filename:      "screen_error.txt",
			expectedState: models.AgentStateError,
			minConfidence: models.StateConfidenceLow,
		},
		{
			filename:      "screen_idle.txt",
			expectedState: models.AgentStateIdle,
			minConfidence: models.StateConfidenceLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			screen := readTestdata(t, tt.filename)
			result := engine.detectBasicState(screen, "hash-"+tt.filename)

			assert.Equal(t, tt.expectedState, result.State,
				"unexpected state for %s", tt.filename)
			assert.GreaterOrEqual(t, confidenceRank(result.Confidence), confidenceRank(tt.minConfidence),
				"confidence too low for %s", tt.filename)
			assert.NotEmpty(t, result.Reason, "reason should not be empty")
		})
	}
}
