package state

import (
	"strings"
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

func TestApplyRuleBasedInferenceConflictAddsEvidence(t *testing.T) {
	result := &DetectionResult{
		State:      models.AgentStateIdle,
		Confidence: models.StateConfidenceHigh,
		Reason:     "adapter",
	}

	ApplyRuleBasedInference(result, "HTTP 429 rate limit")
	if result.State != models.AgentStateRateLimited {
		t.Fatalf("expected rate limited state, got %q", result.State)
	}
	found := false
	for _, evidence := range result.Evidence {
		if strings.Contains(evidence, "conflict:") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected conflict evidence to be recorded")
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

func TestResolveConflictPrefersBlocking(t *testing.T) {
	adapter := &DetectionResult{
		State:      models.AgentStateIdle,
		Confidence: models.StateConfidenceHigh,
		Reason:     "adapter",
	}
	transcript := &DetectionResult{
		State:      models.AgentStateRateLimited,
		Confidence: models.StateConfidenceLow,
		Reason:     "transcript",
	}

	resolved := resolveConflict(adapter, transcript)
	if resolved.State != models.AgentStateRateLimited {
		t.Fatalf("expected rate limited to win, got %q", resolved.State)
	}
}

func TestResolveConflictNonBlockingUsesConfidence(t *testing.T) {
	adapter := &DetectionResult{
		State:      models.AgentStateWorking,
		Confidence: models.StateConfidenceLow,
	}
	transcript := &DetectionResult{
		State:      models.AgentStateIdle,
		Confidence: models.StateConfidenceHigh,
	}

	resolved := resolveConflict(adapter, transcript)
	if resolved.State != models.AgentStateIdle {
		t.Fatalf("expected idle to win, got %q", resolved.State)
	}
}
