package adapters

import (
	"testing"

	"github.com/opencode-ai/swarm/internal/models"
)

func TestGenericAdapter_Name(t *testing.T) {
	adapter := NewGenericAdapter("test-adapter", "test-cmd")

	if adapter.Name() != "test-adapter" {
		t.Errorf("expected 'test-adapter', got %q", adapter.Name())
	}
}

func TestGenericAdapter_Tier(t *testing.T) {
	adapter := NewGenericAdapter("test", "cmd")

	if adapter.Tier() != models.AdapterTierGeneric {
		t.Errorf("expected AdapterTierGeneric, got %v", adapter.Tier())
	}
}

func TestGenericAdapter_SpawnCommand(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		wantCmd     string
		wantArgsLen int
	}{
		{
			name:        "simple command",
			command:     "opencode",
			wantCmd:     "opencode",
			wantArgsLen: 0,
		},
		{
			name:        "command with args",
			command:     "opencode --model claude",
			wantCmd:     "opencode",
			wantArgsLen: 2,
		},
		{
			name:        "empty command",
			command:     "",
			wantCmd:     "",
			wantArgsLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewGenericAdapter("test", tt.command)
			cmd, args := adapter.SpawnCommand(SpawnOptions{})

			if cmd != tt.wantCmd {
				t.Errorf("cmd: got %q, want %q", cmd, tt.wantCmd)
			}
			if len(args) != tt.wantArgsLen {
				t.Errorf("args length: got %d, want %d", len(args), tt.wantArgsLen)
			}
		})
	}
}

func TestGenericAdapter_DetectReady(t *testing.T) {
	adapter := NewGenericAdapter("test", "cmd",
		WithIdleIndicators("ready>", "idle"),
		WithBusyIndicators("working", "processing"),
	)

	tests := []struct {
		name      string
		screen    string
		wantReady bool
	}{
		{
			name:      "idle indicator present",
			screen:    "Output complete.\nready> ",
			wantReady: true,
		},
		{
			name:      "busy indicator present",
			screen:    "working on task...",
			wantReady: false,
		},
		{
			name:      "no indicator",
			screen:    "random output",
			wantReady: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready, err := adapter.DetectReady(tt.screen)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ready != tt.wantReady {
				t.Errorf("ready: got %v, want %v", ready, tt.wantReady)
			}
		})
	}
}

func TestGenericAdapter_DetectState(t *testing.T) {
	adapter := NewGenericAdapter("test", "cmd")

	tests := []struct {
		name      string
		screen    string
		wantState models.AgentState
	}{
		{
			name:      "error state",
			screen:    "Error: something went wrong",
			wantState: models.AgentStateError,
		},
		{
			name:      "rate limited",
			screen:    "Rate limit exceeded. Try again later.",
			wantState: models.AgentStateRateLimited,
		},
		{
			name:      "awaiting approval",
			screen:    "Do you want to proceed? [y/n]",
			wantState: models.AgentStateAwaitingApproval,
		},
		{
			name:      "working (spinner)",
			screen:    "Processing ⠋",
			wantState: models.AgentStateWorking,
		},
		{
			name:      "idle (prompt)",
			screen:    "Task complete.\n❯ ",
			wantState: models.AgentStateIdle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, reason, err := adapter.DetectState(tt.screen, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if state != tt.wantState {
				t.Errorf("state: got %v, want %v (reason: %s)", state, tt.wantState, reason.Reason)
			}
		})
	}
}

func TestGenericAdapter_DetectState_Confidence(t *testing.T) {
	adapter := NewGenericAdapter("test", "cmd")

	// Error detection should have medium confidence
	state, reason, _ := adapter.DetectState("Error: test", nil)
	if state != models.AgentStateError {
		t.Errorf("expected Error state, got %v", state)
	}
	if reason.Confidence != models.StateConfidenceMedium {
		t.Errorf("expected Medium confidence for error, got %v", reason.Confidence)
	}

	// Idle detection should have low confidence (heuristic)
	state, reason, _ = adapter.DetectState("❯ ", nil)
	if state != models.AgentStateIdle {
		t.Errorf("expected Idle state, got %v", state)
	}
	if reason.Confidence != models.StateConfidenceLow {
		t.Errorf("expected Low confidence for idle heuristic, got %v", reason.Confidence)
	}
}

type mockTmuxClient struct {
	sentKeys   string
	sentTarget string
	literal    bool
}

func (m *mockTmuxClient) SendKeys(target, keys string, literal bool) error {
	m.sentTarget = target
	m.sentKeys = keys
	m.literal = literal
	return nil
}

func TestGenericAdapter_SendMessage(t *testing.T) {
	adapter := NewGenericAdapter("test", "cmd")
	mock := &mockTmuxClient{}

	err := adapter.SendMessage(mock, "session:0.1", "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.sentTarget != "session:0.1" {
		t.Errorf("target: got %q, want 'session:0.1'", mock.sentTarget)
	}
	if mock.sentKeys != "hello world\n" {
		t.Errorf("keys: got %q, want 'hello world\\n'", mock.sentKeys)
	}
	if !mock.literal {
		t.Error("expected literal=true for message")
	}
}

func TestGenericAdapter_Interrupt(t *testing.T) {
	adapter := NewGenericAdapter("test", "cmd")
	mock := &mockTmuxClient{}

	err := adapter.Interrupt(mock, "session:0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.sentTarget != "session:0.1" {
		t.Errorf("target: got %q, want 'session:0.1'", mock.sentTarget)
	}
	if mock.sentKeys != "C-c" {
		t.Errorf("keys: got %q, want 'C-c'", mock.sentKeys)
	}
	if mock.literal {
		t.Error("expected literal=false for interrupt")
	}
}

func TestGenericAdapter_Capabilities(t *testing.T) {
	adapter := NewGenericAdapter("test", "cmd")

	// Generic adapter doesn't support advanced features
	if adapter.SupportsApprovals() {
		t.Error("generic adapter should not support approvals")
	}
	if adapter.SupportsUsageMetrics() {
		t.Error("generic adapter should not support usage metrics")
	}
	if adapter.SupportsDiffMetadata() {
		t.Error("generic adapter should not support diff metadata")
	}
}

func TestBuiltinAdapters(t *testing.T) {
	tests := []struct {
		name    string
		adapter AgentAdapter
		tier    models.AdapterTier
	}{
		{"opencode", OpenCodeAdapter(), models.AdapterTierGeneric},
		{"claude-code", ClaudeCodeAdapter(), models.AdapterTierTelemetry},
		{"codex", CodexAdapter(), models.AdapterTierGeneric},
		{"gemini", GeminiAdapter(), models.AdapterTierGeneric},
		{"generic", GenericFallbackAdapter(), models.AdapterTierGeneric},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.adapter.Name() != tt.name {
				t.Errorf("name: got %q, want %q", tt.adapter.Name(), tt.name)
			}
			if tt.adapter.Tier() != tt.tier {
				t.Errorf("tier: got %v, want %v", tt.adapter.Tier(), tt.tier)
			}
		})
	}
}

func TestWithCustomIndicators(t *testing.T) {
	adapter := NewGenericAdapter("custom", "cmd",
		WithIdleIndicators("READY", "DONE"),
		WithBusyIndicators("BUSY", "WORKING"),
	)

	// Should detect custom idle indicator
	ready, _ := adapter.DetectReady("Operation complete. READY")
	if !ready {
		t.Error("expected ready=true for custom idle indicator")
	}

	// Should detect custom busy indicator
	ready, _ = adapter.DetectReady("BUSY processing data...")
	if ready {
		t.Error("expected ready=false for custom busy indicator")
	}
}
