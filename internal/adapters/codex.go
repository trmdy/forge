package adapters

import (
	"strings"

	"github.com/opencode-ai/swarm/internal/models"
)

// codexAdapter provides Codex CLI-specific state detection and spawn options.
type codexAdapter struct {
	*GenericAdapter
}

// NewCodexAdapter creates a Codex adapter with tuned indicators.
func NewCodexAdapter() *codexAdapter {
	base := NewGenericAdapter(
		string(models.AgentTypeCodex),
		"codex",
		WithIdleIndicators(
			"codex>",
			">",
			"❯",
			"waiting for input",
		),
		WithBusyIndicators(
			"thinking",
			"working",
			"processing",
			"generating",
			"executing",
			"running",
			"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏", // spinner chars
		),
	)

	return &codexAdapter{GenericAdapter: base}
}

// Tier returns the adapter integration tier.
func (a *codexAdapter) Tier() models.AdapterTier {
	return models.AdapterTierGeneric
}

// SpawnCommand returns the command and args to launch Codex CLI.
func (a *codexAdapter) SpawnCommand(opts SpawnOptions) (cmd string, args []string) {
	cmd = "codex"
	args = []string{}

	// Handle approval policy
	if opts.ApprovalPolicy != "" {
		switch strings.ToLower(strings.TrimSpace(opts.ApprovalPolicy)) {
		case "permissive":
			// Use full-auto for permissive mode (sandboxed but minimal prompts)
			args = append(args, "--full-auto")
		case "strict":
			// Use untrusted for strict mode (asks for approval on non-trusted commands)
			args = append(args, "--ask-for-approval", "untrusted")
		default:
			// Default: let model decide when to ask
			args = append(args, "--ask-for-approval", "on-request")
		}
	}

	return cmd, args
}

// DetectReady reports whether the agent is ready based on screen output.
func (a *codexAdapter) DetectReady(screen string) (bool, error) {
	lower := strings.ToLower(screen)

	// Codex-specific ready patterns
	if strings.Contains(lower, "codex>") || strings.Contains(screen, "❯") {
		return true, nil
	}

	// Check for session start indicators
	if strings.Contains(lower, "session started") || strings.Contains(lower, "ready") {
		return true, nil
	}

	return a.GenericAdapter.DetectReady(screen)
}

// DetectState returns the current state with a reason.
func (a *codexAdapter) DetectState(screen string, meta any) (models.AgentState, StateReason, error) {
	lower := strings.ToLower(screen)

	// Check for Codex-specific approval patterns
	approvalPatterns := []string{
		"do you want to proceed",
		"approve this action",
		"allow execution",
		"run this command",
		"execute?",
		"[y/n]",
		"(y/n)",
	}
	for _, pattern := range approvalPatterns {
		if strings.Contains(lower, pattern) {
			return models.AgentStateAwaitingApproval, StateReason{
				Reason:     "codex approval prompt detected",
				Confidence: models.StateConfidenceMedium,
				Evidence:   []string{pattern},
			}, nil
		}
	}

	// Check for sandbox-related prompts (Codex-specific)
	if strings.Contains(lower, "sandbox") && (strings.Contains(lower, "approve") || strings.Contains(lower, "allow")) {
		return models.AgentStateAwaitingApproval, StateReason{
			Reason:     "codex sandbox approval detected",
			Confidence: models.StateConfidenceMedium,
			Evidence:   []string{"sandbox approval"},
		}, nil
	}

	// Delegate to generic detection for other states
	return a.GenericAdapter.DetectState(screen, meta)
}

// SupportsApprovals indicates if the adapter supports approvals routing.
func (a *codexAdapter) SupportsApprovals() bool {
	return true
}

// Ensure codexAdapter implements AgentAdapter
var _ AgentAdapter = (*codexAdapter)(nil)
