package state

import (
	"testing"

	"github.com/opencode-ai/swarm/internal/models"
)

func TestApplyRuleBasedInference(t *testing.T) {
	result := &DetectionResult{
		State:      models.AgentStateWorking,
		Confidence: models.StateConfidenceLow,
		Reason:     "adapter",
	}

	ApplyRuleBasedInference(result, "HTTP 429 rate limit")
	if result.State != models.AgentStateRateLimited {
		t.Fatalf("expected rate limited state, got %q", result.State)
	}
	if result.Reason == "adapter" {
		t.Fatalf("expected reason to be updated")
	}
}

func TestCombineResults(t *testing.T) {
	primary := &DetectionResult{
		State:      models.AgentStateIdle,
		Confidence: models.StateConfidenceLow,
		Reason:     "primary",
		Evidence:   []string{"a"},
	}
	secondary := &DetectionResult{
		State:      models.AgentStateError,
		Confidence: models.StateConfidenceHigh,
		Reason:     "secondary",
		Evidence:   []string{"b"},
	}

	combined := CombineResults(primary, secondary)
	if combined.State != models.AgentStateError {
		t.Fatalf("expected secondary state to win, got %q", combined.State)
	}
	if combined.Confidence != models.StateConfidenceHigh {
		t.Fatalf("expected high confidence, got %q", combined.Confidence)
	}
	if len(combined.Evidence) != 2 {
		t.Fatalf("expected evidence to be combined")
	}
	if combined.Reason == "primary" {
		t.Fatalf("expected reason to be updated")
	}
}
