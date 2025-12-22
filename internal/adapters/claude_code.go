package adapters

import (
	"encoding/json"
	"strings"

	"github.com/opencode-ai/swarm/internal/models"
)

type claudeStreamEvent struct {
	Type           string `json:"type"`
	Subtype        string `json:"subtype"`
	PermissionMode string `json:"permissionMode"`
}

// claudeCodeAdapter provides Claude Code-specific state detection and metadata handling.
type claudeCodeAdapter struct {
	*GenericAdapter
}

// NewClaudeCodeAdapter creates a Claude Code adapter with tuned indicators.
func NewClaudeCodeAdapter() *claudeCodeAdapter {
	base := NewGenericAdapter(
		string(models.AgentTypeClaudeCode),
		"claude",
		WithIdleIndicators(
			"claude>",
			"❯",
			"ready",
		),
		WithBusyIndicators(
			"thinking",
			"writing",
			"reading",
			"planning",
			"processing",
			"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
		),
	)

	return &claudeCodeAdapter{GenericAdapter: base}
}

// Tier returns the adapter integration tier.
func (a *claudeCodeAdapter) Tier() models.AdapterTier {
	return models.AdapterTierTelemetry
}

// DetectReady reports whether the agent is ready based on screen output.
func (a *claudeCodeAdapter) DetectReady(screen string) (bool, error) {
	if hasClaudeStreamInit(screen) {
		return true, nil
	}
	return a.GenericAdapter.DetectReady(screen)
}

// DetectState returns the current state with a reason.
func (a *claudeCodeAdapter) DetectState(screen string, meta any) (models.AgentState, StateReason, error) {
	if state, reason, ok := detectClaudeStreamState(screen); ok {
		return state, reason, nil
	}
	return a.GenericAdapter.DetectState(screen, meta)
}

// SupportsApprovals indicates if the adapter supports approvals routing.
func (a *claudeCodeAdapter) SupportsApprovals() bool {
	return true
}

func hasClaudeStreamInit(screen string) bool {
	for _, event := range parseClaudeStreamEvents(screen) {
		if strings.EqualFold(event.Type, "system") && strings.EqualFold(event.Subtype, "init") {
			return true
		}
	}
	return false
}

func detectClaudeStreamState(screen string) (models.AgentState, StateReason, bool) {
	events := parseClaudeStreamEvents(screen)
	if len(events) == 0 {
		return "", StateReason{}, false
	}

	for _, event := range events {
		if strings.EqualFold(event.Type, "error") || strings.EqualFold(event.Subtype, "error") {
			return models.AgentStateError, StateReason{
				Reason:     "stream-json error event",
				Confidence: models.StateConfidenceMedium,
				Evidence:   claudeEventEvidence(event, "error"),
			}, true
		}

		if hasClaudePermissionSignal(event) {
			return models.AgentStateAwaitingApproval, StateReason{
				Reason:     "stream-json permission event",
				Confidence: models.StateConfidenceMedium,
				Evidence:   claudeEventEvidence(event, "permission"),
			}, true
		}
	}

	for _, event := range events {
		if strings.EqualFold(event.Type, "system") && strings.EqualFold(event.Subtype, "init") {
			evidence := []string{"system/init"}
			if event.PermissionMode != "" {
				evidence = append(evidence, "permissionMode="+event.PermissionMode)
			}
			return models.AgentStateIdle, StateReason{
				Reason:     "stream-json init event",
				Confidence: models.StateConfidenceMedium,
				Evidence:   evidence,
			}, true
		}
	}

	return "", StateReason{}, false
}

func hasClaudePermissionSignal(event claudeStreamEvent) bool {
	lowerType := strings.ToLower(event.Type)
	lowerSubtype := strings.ToLower(event.Subtype)
	return strings.Contains(lowerType, "permission") ||
		strings.Contains(lowerSubtype, "permission") ||
		strings.Contains(lowerType, "approval") ||
		strings.Contains(lowerSubtype, "approval")
}

func claudeEventEvidence(event claudeStreamEvent, label string) []string {
	evidence := []string{label}
	if event.Type != "" {
		evidence = append(evidence, "type="+event.Type)
	}
	if event.Subtype != "" {
		evidence = append(evidence, "subtype="+event.Subtype)
	}
	if event.PermissionMode != "" {
		evidence = append(evidence, "permissionMode="+event.PermissionMode)
	}
	return evidence
}

func parseClaudeStreamEvents(screen string) []claudeStreamEvent {
	lines := strings.Split(screen, "\n")
	events := make([]claudeStreamEvent, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 2 || !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
			continue
		}

		var event claudeStreamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type == "" && event.Subtype == "" && event.PermissionMode == "" {
			continue
		}
		events = append(events, event)
	}
	return events
}
