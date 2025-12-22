// Package state provides agent state management and change notifications.
package state

import (
	"fmt"
	"strings"

	"github.com/opencode-ai/swarm/internal/logging"
	"github.com/opencode-ai/swarm/internal/models"
)

var conflictLogger = logging.Component("state-rules")

// ApplyRuleBasedInference adjusts detection results using transcript rules.
func ApplyRuleBasedInference(result *DetectionResult, screen string) {
	if result == nil {
		return
	}

	info := ParseTranscript(screen)
	if info == nil {
		return
	}

	transcript := &DetectionResult{
		State:      info.State,
		Confidence: info.Confidence,
		Reason:     info.Reason,
		Evidence:   info.Evidence,
	}

	if result.State == transcript.State {
		result.Confidence = maxConfidence(result.Confidence, transcript.Confidence)
		result.Reason = appendReason(result.Reason, transcript.Reason)
		result.Evidence = append(result.Evidence, transcript.Evidence...)
		return
	}

	adapter := *result
	resolved := resolveConflict(result, transcript)
	*result = *resolved

	conflict := fmt.Sprintf(
		"conflict: adapter=%s(%s) transcript=%s(%s)",
		adapter.State,
		adapter.Confidence,
		transcript.State,
		transcript.Confidence,
	)
	result.Evidence = append(result.Evidence, adapter.Evidence...)
	result.Evidence = append(result.Evidence, transcript.Evidence...)
	result.Evidence = append(result.Evidence, conflict)
	result.Reason = appendReason(result.Reason, fmt.Sprintf("conflict: adapter_reason=%s", strings.TrimSpace(adapter.Reason)))

	conflictLogger.Warn().
		Str("adapter_state", string(adapter.State)).
		Str("adapter_confidence", string(adapter.Confidence)).
		Str("transcript_state", string(transcript.State)).
		Str("transcript_confidence", string(transcript.Confidence)).
		Msg("conflicting state evidence resolved")
}

// CombineResults merges two detection results, favoring higher confidence.
func CombineResults(primary, secondary *DetectionResult) *DetectionResult {
	if primary == nil {
		return secondary
	}
	if secondary == nil {
		return primary
	}

	if confidenceRank(primary.Confidence) < confidenceRank(secondary.Confidence) {
		primary, secondary = secondary, primary
	}

	combined := *primary
	if combined.Evidence == nil {
		combined.Evidence = []string{}
	}
	combined.Evidence = append(combined.Evidence, secondary.Evidence...)
	if combined.Reason != secondary.Reason && secondary.Reason != "" {
		combined.Reason = combined.Reason + "; " + secondary.Reason
	}

	return &combined
}

func confidenceRank(conf models.StateConfidence) int {
	switch conf {
	case models.StateConfidenceHigh:
		return 3
	case models.StateConfidenceMedium:
		return 2
	case models.StateConfidenceLow:
		return 1
	default:
		return 0
	}
}

func resolveConflict(adapter, transcript *DetectionResult) *DetectionResult {
	if adapter == nil {
		return transcript
	}
	if transcript == nil {
		return adapter
	}

	adapterBlocking := isBlockingState(adapter.State)
	transcriptBlocking := isBlockingState(transcript.State)

	if adapterBlocking || transcriptBlocking {
		if adapterBlocking && !transcriptBlocking {
			return adapter
		}
		if transcriptBlocking && !adapterBlocking {
			return transcript
		}
		if stateSeverityRank(adapter.State) != stateSeverityRank(transcript.State) {
			if stateSeverityRank(adapter.State) > stateSeverityRank(transcript.State) {
				return adapter
			}
			return transcript
		}
		if confidenceRank(adapter.Confidence) >= confidenceRank(transcript.Confidence) {
			return adapter
		}
		return transcript
	}

	if confidenceRank(adapter.Confidence) != confidenceRank(transcript.Confidence) {
		if confidenceRank(adapter.Confidence) > confidenceRank(transcript.Confidence) {
			return adapter
		}
		return transcript
	}

	if stateSeverityRank(adapter.State) >= stateSeverityRank(transcript.State) {
		return adapter
	}
	return transcript
}

func stateSeverityRank(state models.AgentState) int {
	switch state {
	case models.AgentStateError:
		return 6
	case models.AgentStateRateLimited:
		return 5
	case models.AgentStateAwaitingApproval:
		return 4
	case models.AgentStateWorking:
		return 3
	case models.AgentStateIdle:
		return 2
	case models.AgentStateStarting, models.AgentStatePaused:
		return 1
	case models.AgentStateStopped:
		return 0
	default:
		return 0
	}
}

func isBlockingState(state models.AgentState) bool {
	switch state {
	case models.AgentStateAwaitingApproval, models.AgentStateRateLimited, models.AgentStateError:
		return true
	default:
		return false
	}
}

func appendReason(base, extra string) string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return extra
	}
	return base + "; " + extra
}

func maxConfidence(a, b models.StateConfidence) models.StateConfidence {
	if confidenceRank(a) >= confidenceRank(b) {
		return a
	}
	return b
}
