// Package state provides agent state management and change notifications.
package state

import (
	"strings"

	"github.com/opencode-ai/swarm/internal/models"
)

// ParseTranscript inspects transcript text and returns a detected state if a pattern matches.
func ParseTranscript(text string) *models.StateInfo {
	lower := strings.ToLower(text)

	if containsAny(lower, "error", "exception", "panic", "failed") {
		return &models.StateInfo{
			State:      models.AgentStateError,
			Confidence: models.StateConfidenceMedium,
			Reason:     "error indicator detected in transcript",
		}
	}

	if containsAny(lower, "rate limit", "too many requests", "quota exceeded", "429") {
		return &models.StateInfo{
			State:      models.AgentStateRateLimited,
			Confidence: models.StateConfidenceMedium,
			Reason:     "rate limit indicator detected in transcript",
		}
	}

	if containsAny(lower, "approve", "confirm", "allow", "proceed?", "[y/n]", "(y/n)") {
		return &models.StateInfo{
			State:      models.AgentStateAwaitingApproval,
			Confidence: models.StateConfidenceLow,
			Reason:     "approval prompt detected in transcript",
		}
	}

	return nil
}
