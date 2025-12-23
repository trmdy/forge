package adapters

import "github.com/opencode-ai/swarm/internal/models"

type openCodeAdapter struct {
	*GenericAdapter
}

// NewOpenCodeAdapter creates an adapter for OpenCode CLI.
func NewOpenCodeAdapter() *openCodeAdapter {
	base := NewGenericAdapter(
		string(models.AgentTypeOpenCode),
		"opencode --hostname 127.0.0.1",
		WithIdleIndicators(
			"opencode>",
			"waiting for input",
			"❯",
		),
		WithBusyIndicators(
			"thinking",
			"generating",
			"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
		),
	)

	return &openCodeAdapter{GenericAdapter: base}
}

// SupportsUsageMetrics indicates if the adapter reports usage metrics.
func (a *openCodeAdapter) SupportsUsageMetrics() bool {
	return true
}

// SupportsDiffMetadata indicates if the adapter reports diff metadata.
func (a *openCodeAdapter) SupportsDiffMetadata() bool {
	return true
}

// ExtractUsageMetrics parses usage metrics from OpenCode stats output.
func (a *openCodeAdapter) ExtractUsageMetrics(screen string) (*models.UsageMetrics, bool, error) {
	return ParseOpenCodeStats(screen)
}

// ExtractDiffMetadata parses diff metadata from OpenCode output.
func (a *openCodeAdapter) ExtractDiffMetadata(screen string) (*models.DiffMetadata, bool, error) {
	return ParseOpenCodeDiffMetadata(screen)
}
