// Package cli provides tests for the explain command.
package cli

import (
	"testing"
	"time"

	"github.com/opencode-ai/swarm/internal/models"
)

func TestBuildAgentExplanation_Idle(t *testing.T) {
	agent := &models.Agent{
		ID:    "agent_123",
		Type:  models.AgentTypeOpenCode,
		State: models.AgentStateIdle,
	}

	explanation := buildAgentExplanation(agent, nil)

	if explanation.AgentID != agent.ID {
		t.Errorf("AgentID = %v, want %v", explanation.AgentID, agent.ID)
	}
	if explanation.State != models.AgentStateIdle {
		t.Errorf("State = %v, want %v", explanation.State, models.AgentStateIdle)
	}
	if explanation.IsBlocked {
		t.Error("IsBlocked should be false for idle agent")
	}
	if len(explanation.Suggestions) == 0 {
		t.Error("Should have suggestions for idle agent")
	}
}

func TestBuildAgentExplanation_AwaitingApproval(t *testing.T) {
	agent := &models.Agent{
		ID:    "agent_123",
		Type:  models.AgentTypeOpenCode,
		State: models.AgentStateAwaitingApproval,
	}

	explanation := buildAgentExplanation(agent, nil)

	if !explanation.IsBlocked {
		t.Error("IsBlocked should be true for awaiting_approval agent")
	}
	if len(explanation.BlockReasons) == 0 {
		t.Error("Should have block reasons")
	}
	if len(explanation.Suggestions) == 0 {
		t.Error("Should have suggestions")
	}
}

func TestBuildAgentExplanation_Paused(t *testing.T) {
	pauseTime := time.Now().Add(5 * time.Minute)
	agent := &models.Agent{
		ID:          "agent_123",
		Type:        models.AgentTypeOpenCode,
		State:       models.AgentStatePaused,
		PausedUntil: &pauseTime,
	}

	explanation := buildAgentExplanation(agent, nil)

	if !explanation.IsBlocked {
		t.Error("IsBlocked should be true for paused agent")
	}
	if len(explanation.BlockReasons) < 2 {
		t.Error("Should have block reasons including time remaining")
	}
}

func TestBuildAgentExplanation_WithQueue(t *testing.T) {
	agent := &models.Agent{
		ID:    "agent_123",
		Type:  models.AgentTypeOpenCode,
		State: models.AgentStateIdle,
	}
	queueItems := []*models.QueueItem{
		{ID: "qi_1", Status: models.QueueItemStatusPending},
		{ID: "qi_2", Status: models.QueueItemStatusPending},
		{ID: "qi_3", Status: models.QueueItemStatusCompleted},
	}

	explanation := buildAgentExplanation(agent, queueItems)

	if explanation.QueueStatus.TotalItems != 3 {
		t.Errorf("TotalItems = %v, want 3", explanation.QueueStatus.TotalItems)
	}
	if explanation.QueueStatus.PendingItems != 2 {
		t.Errorf("PendingItems = %v, want 2", explanation.QueueStatus.PendingItems)
	}
}

func TestBuildQueueItemExplanation_Pending(t *testing.T) {
	item := &models.QueueItem{
		ID:        "qi_123",
		AgentID:   "agent_123",
		Type:      models.QueueItemTypeMessage,
		Status:    models.QueueItemStatusPending,
		Position:  1,
		CreatedAt: time.Now(),
	}
	agent := &models.Agent{
		ID:    "agent_123",
		State: models.AgentStateWorking,
	}

	explanation := buildQueueItemExplanation(item, agent)

	if explanation.ItemID != item.ID {
		t.Errorf("ItemID = %v, want %v", explanation.ItemID, item.ID)
	}
	if !explanation.IsBlocked {
		t.Error("IsBlocked should be true when agent is working")
	}
	if len(explanation.BlockReasons) == 0 {
		t.Error("Should have block reasons")
	}
}

func TestBuildQueueItemExplanation_AgentIdle(t *testing.T) {
	item := &models.QueueItem{
		ID:        "qi_123",
		AgentID:   "agent_123",
		Type:      models.QueueItemTypeMessage,
		Status:    models.QueueItemStatusPending,
		Position:  1,
		CreatedAt: time.Now(),
	}
	agent := &models.Agent{
		ID:    "agent_123",
		State: models.AgentStateIdle,
	}

	explanation := buildQueueItemExplanation(item, agent)

	// First position with idle agent should not be blocked
	if explanation.IsBlocked {
		t.Error("IsBlocked should be false when agent is idle and item is first")
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"this is a longer string", 10, "this is..."},
		{"exact", 5, "exact"},
		{"", 10, ""},
	}

	for _, tt := range tests {
		got := truncateString(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}
