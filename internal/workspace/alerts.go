// Package workspace provides helpers for workspace lifecycle management.
package workspace

import (
	"fmt"
	"time"

	"github.com/opencode-ai/swarm/internal/models"
)

// BuildAlerts derives alert entries from agent states.
func BuildAlerts(agents []*models.Agent) []models.Alert {
	alerts := make([]models.Alert, 0)
	now := time.Now().UTC()

	for _, agent := range agents {
		switch agent.State {
		case models.AgentStateAwaitingApproval:
			alerts = append(alerts, models.Alert{
				Type:      models.AlertTypeApprovalNeeded,
				Severity:  models.AlertSeverityWarning,
				Message:   "Approval needed",
				AgentID:   agent.ID,
				CreatedAt: now,
			})
		case models.AgentStateRateLimited:
			alerts = append(alerts, models.Alert{
				Type:      models.AlertTypeRateLimit,
				Severity:  models.AlertSeverityWarning,
				Message:   "Agent rate limited",
				AgentID:   agent.ID,
				CreatedAt: now,
			})
		case models.AgentStateError:
			alerts = append(alerts, models.Alert{
				Type:      models.AlertTypeError,
				Severity:  models.AlertSeverityError,
				Message:   "Agent error",
				AgentID:   agent.ID,
				CreatedAt: now,
			})
		case models.AgentStatePaused:
			alerts = append(alerts, models.Alert{
				Type:      models.AlertTypeCooldown,
				Severity:  models.AlertSeverityInfo,
				Message:   "Agent paused",
				AgentID:   agent.ID,
				CreatedAt: now,
			})
		}
	}

	return alerts
}

// BuildUsageLimitAlerts creates alerts from usage limit status.
func BuildUsageLimitAlerts(accountID string, status *models.UsageLimitStatus) []models.Alert {
	if status == nil {
		return nil
	}

	alerts := make([]models.Alert, 0)
	now := time.Now().UTC()

	for _, warning := range status.Warnings {
		severity := models.AlertSeverityWarning
		if status.IsOverLimit {
			severity = models.AlertSeverityError
		}

		alerts = append(alerts, models.Alert{
			Type:      models.AlertTypeUsageLimit,
			Severity:  severity,
			Message:   fmt.Sprintf("Account %s: %s", accountID, warning),
			CreatedAt: now,
		})
	}

	return alerts
}

// MergeAlerts combines multiple alert slices, deduplicating by type+agent.
func MergeAlerts(alertSets ...[]models.Alert) []models.Alert {
	seen := make(map[string]bool)
	result := make([]models.Alert, 0)

	for _, alerts := range alertSets {
		for _, alert := range alerts {
			key := fmt.Sprintf("%s:%s", alert.Type, alert.AgentID)
			if !seen[key] {
				seen[key] = true
				result = append(result, alert)
			}
		}
	}

	return result
}
