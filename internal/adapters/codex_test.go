package adapters

import (
	"testing"

	"github.com/opencode-ai/swarm/internal/models"
)

func TestNewCodexAdapter(t *testing.T) {
	adapter := NewCodexAdapter()

	if adapter.Name() != string(models.AgentTypeCodex) {
		t.Errorf("expected name %q, got %q", models.AgentTypeCodex, adapter.Name())
	}

	if adapter.Tier() != models.AdapterTierGeneric {
		t.Errorf("expected tier %v, got %v", models.AdapterTierGeneric, adapter.Tier())
	}
}

func TestCodexAdapter_SpawnCommand(t *testing.T) {
	adapter := NewCodexAdapter()

	tests := []struct {
		name           string
		approvalPolicy string
		wantArgs       []string
	}{
		{
			name:           "no approval policy",
			approvalPolicy: "",
			wantArgs:       []string{},
		},
		{
			name:           "permissive approval policy",
			approvalPolicy: "permissive",
			wantArgs:       []string{"--full-auto"},
		},
		{
			name:           "strict approval policy",
			approvalPolicy: "strict",
			wantArgs:       []string{"--ask-for-approval", "untrusted"},
		},
		{
			name:           "default approval policy",
			approvalPolicy: "default",
			wantArgs:       []string{"--ask-for-approval", "on-request"},
		},
		{
			name:           "case insensitive permissive",
			approvalPolicy: "PERMISSIVE",
			wantArgs:       []string{"--full-auto"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := SpawnOptions{ApprovalPolicy: tt.approvalPolicy}
			cmd, args := adapter.SpawnCommand(opts)

			if cmd != "codex" {
				t.Errorf("expected command %q, got %q", "codex", cmd)
			}

			if len(args) != len(tt.wantArgs) {
				t.Errorf("expected %d args, got %d: %v", len(tt.wantArgs), len(args), args)
				return
			}

			for i, arg := range args {
				if arg != tt.wantArgs[i] {
					t.Errorf("arg[%d]: expected %q, got %q", i, tt.wantArgs[i], arg)
				}
			}
		})
	}
}

func TestCodexAdapter_DetectReady(t *testing.T) {
	adapter := NewCodexAdapter()

	tests := []struct {
		name   string
		screen string
		want   bool
	}{
		{
			name:   "codex prompt",
			screen: "codex>",
			want:   true,
		},
		{
			name:   "unicode prompt",
			screen: "Welcome to Codex\n❯",
			want:   true,
		},
		{
			name:   "session started",
			screen: "Session started, ready for input",
			want:   true,
		},
		{
			name:   "processing spinner",
			screen: "⠋ Thinking...",
			want:   false,
		},
		{
			name:   "empty screen",
			screen: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adapter.DetectReady(tt.screen)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("DetectReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCodexAdapter_DetectState(t *testing.T) {
	adapter := NewCodexAdapter()

	tests := []struct {
		name      string
		screen    string
		wantState models.AgentState
	}{
		{
			name:      "approval prompt y/n",
			screen:    "Do you want to proceed? [y/n]",
			wantState: models.AgentStateAwaitingApproval,
		},
		{
			name:      "sandbox approval",
			screen:    "Sandbox execution requires approval. Allow?",
			wantState: models.AgentStateAwaitingApproval,
		},
		{
			name:      "execute prompt",
			screen:    "Run this command? Execute? (y/n)",
			wantState: models.AgentStateAwaitingApproval,
		},
		{
			name:      "idle at prompt",
			screen:    "codex>",
			wantState: models.AgentStateIdle,
		},
		{
			name:      "working with spinner",
			screen:    "⠋ Processing request...",
			wantState: models.AgentStateWorking,
		},
		{
			name:      "error state",
			screen:    "Error: Connection failed",
			wantState: models.AgentStateError,
		},
		{
			name:      "rate limited",
			screen:    "Rate limit exceeded. Try again later.",
			wantState: models.AgentStateRateLimited,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, reason, err := adapter.DetectState(tt.screen, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if state != tt.wantState {
				t.Errorf("DetectState() = %v (reason: %s), want %v", state, reason.Reason, tt.wantState)
			}
		})
	}
}

func TestCodexAdapter_SupportsApprovals(t *testing.T) {
	adapter := NewCodexAdapter()
	if !adapter.SupportsApprovals() {
		t.Error("expected SupportsApprovals() to return true")
	}
}

func TestCodexAdapter_ImplementsInterface(t *testing.T) {
	var _ AgentAdapter = NewCodexAdapter()
}
