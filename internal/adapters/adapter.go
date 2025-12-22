// Package adapters defines the adapter interface for supported agent CLIs.
package adapters

import "github.com/opencode-ai/swarm/internal/models"

// TmuxClient describes the subset of tmux functionality adapters rely on.
type TmuxClient interface {
	// SendKeys sends keys to a tmux pane. literal indicates raw input without key interpretation.
	SendKeys(target, keys string, literal bool) error
}

// SpawnOptions controls how an agent process is launched.
type SpawnOptions struct {
	// AgentType identifies the adapter type.
	AgentType models.AgentType

	// AccountID is an optional account/profile identifier.
	AccountID string

	// InitialPrompt is an optional prompt to send on startup.
	InitialPrompt string

	// Environment overrides for the agent process.
	Environment map[string]string
}

// StateReason describes why an adapter reported a state.
type StateReason struct {
	Reason     string
	Confidence models.StateConfidence
	Evidence   []string
}

// AgentAdapter defines the interface implemented by agent CLI adapters.
type AgentAdapter interface {
	// Name returns the adapter name (e.g., opencode, claude-code).
	Name() string

	// Tier returns the adapter integration tier.
	Tier() models.AdapterTier

	// SpawnCommand returns the command and args to launch the agent.
	SpawnCommand(opts SpawnOptions) (cmd string, args []string)

	// DetectReady reports whether the agent is ready based on screen output.
	DetectReady(screen string) (bool, error)

	// DetectState returns the current state with a reason.
	DetectState(screen string, meta any) (models.AgentState, StateReason, error)

	// SendMessage dispatches a message to the agent.
	SendMessage(tmux TmuxClient, pane, message string) error

	// Interrupt sends an interrupt signal (e.g., Ctrl-C).
	Interrupt(tmux TmuxClient, pane string) error

	// SupportsApprovals indicates if the adapter supports approvals routing.
	SupportsApprovals() bool

	// SupportsUsageMetrics indicates if the adapter reports usage metrics.
	SupportsUsageMetrics() bool

	// SupportsDiffMetadata indicates if the adapter reports diff metadata.
	SupportsDiffMetadata() bool
}

// UsageMetricsExtractor allows adapters to extract usage metrics from screen output.
type UsageMetricsExtractor interface {
	ExtractUsageMetrics(screen string) (*models.UsageMetrics, bool, error)
}

// DiffMetadataExtractor allows adapters to extract diff metadata from screen output.
type DiffMetadataExtractor interface {
	ExtractDiffMetadata(screen string) (*models.DiffMetadata, bool, error)
}
