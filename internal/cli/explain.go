// Package cli provides the explain command for human-readable status explanations.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

func init() {
	rootCmd.AddCommand(explainCmd)
}

var explainCmd = &cobra.Command{
	Use:   "explain [agent-id|queue-item-id]",
	Short: "Explain agent or queue item status",
	Long: `Show a human-readable explanation of why an agent or queue item is in its current state.

If no argument is given, explains the agent from the current context (set with 'forge use').

Examples:
  forge explain abc123        # Explain agent status
  forge explain qi_789        # Explain queue item status
  forge explain               # Explain context agent`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		// Determine target
		var target string
		if len(args) > 0 {
			target = args[0]
		}

		// Try to resolve as agent first, then queue item
		agentRepo := db.NewAgentRepository(database)
		queueRepo := db.NewQueueRepository(database)

		// Check if it's a queue item (starts with "qi_")
		if strings.HasPrefix(target, "qi_") {
			return explainQueueItem(ctx, database, queueRepo, agentRepo, target)
		}

		// Try to find agent
		if target == "" {
			// Use context
			wsRepo := db.NewWorkspaceRepository(database)
			resolved, err := ResolveAgentContext(ctx, agentRepo, "", "")
			if err != nil {
				return err
			}
			if resolved.AgentID == "" {
				// Try workspace context and get first agent
				wsResolved, _ := ResolveWorkspaceContext(ctx, wsRepo, "")
				if wsResolved != nil && wsResolved.WorkspaceID != "" {
					agents, err := agentRepo.ListByWorkspace(ctx, wsResolved.WorkspaceID)
					if err == nil && len(agents) > 0 {
						target = agents[0].ID
					}
				}
				if target == "" {
					return fmt.Errorf("no agent specified and no context set (use 'forge use <agent>' or provide agent ID)")
				}
			} else {
				target = resolved.AgentID
			}
		}

		return explainAgent(ctx, database, agentRepo, queueRepo, target)
	},
}

// AgentExplanation contains the full explanation for an agent.
type AgentExplanation struct {
	AgentID       string              `json:"agent_id"`
	Type          models.AgentType    `json:"type"`
	State         models.AgentState   `json:"state"`
	StateInfo     models.StateInfo    `json:"state_info"`
	IsBlocked     bool                `json:"is_blocked"`
	BlockReasons  []string            `json:"block_reasons,omitempty"`
	Suggestions   []string            `json:"suggestions,omitempty"`
	QueueStatus   QueueExplanation    `json:"queue_status"`
	AccountStatus *AccountExplanation `json:"account_status,omitempty"`
	LastActivity  *time.Time          `json:"last_activity,omitempty"`
	PausedUntil   *time.Time          `json:"paused_until,omitempty"`
}

// QueueExplanation summarizes queue status.
type QueueExplanation struct {
	TotalItems   int `json:"total_items"`
	PendingItems int `json:"pending_items"`
	BlockedItems int `json:"blocked_items"`
}

// AccountExplanation summarizes account/cooldown status.
type AccountExplanation struct {
	ProfileName   string     `json:"profile_name,omitempty"`
	CooldownUntil *time.Time `json:"cooldown_until,omitempty"`
	IsInCooldown  bool       `json:"is_in_cooldown"`
}

// QueueItemExplanation contains the full explanation for a queue item.
type QueueItemExplanation struct {
	ItemID       string                     `json:"item_id"`
	AgentID      string                     `json:"agent_id"`
	Type         models.QueueItemType       `json:"type"`
	Status       models.QueueItemStatus     `json:"status"`
	Position     int                        `json:"position"`
	IsBlocked    bool                       `json:"is_blocked"`
	BlockReasons []string                   `json:"block_reasons,omitempty"`
	Suggestions  []string                   `json:"suggestions,omitempty"`
	AgentState   models.AgentState          `json:"agent_state"`
	CreatedAt    time.Time                  `json:"created_at"`
	Content      string                     `json:"content,omitempty"`
	Condition    *models.ConditionalPayload `json:"condition,omitempty"`
}

func explainAgent(ctx context.Context, database *db.DB, agentRepo *db.AgentRepository, queueRepo *db.QueueRepository, agentID string) error {
	agent, err := findAgent(ctx, agentRepo, agentID)
	if err != nil {
		return err
	}

	// Get queue items
	queueItems, err := queueRepo.List(ctx, agent.ID)
	if err != nil {
		return fmt.Errorf("failed to get queue: %w", err)
	}

	// Build explanation
	explanation := buildAgentExplanation(agent, queueItems)

	// Get account info if available
	if agent.AccountID != "" {
		accountRepo := db.NewAccountRepository(database)
		account, err := accountRepo.Get(ctx, agent.AccountID)
		if err == nil {
			explanation.AccountStatus = &AccountExplanation{
				ProfileName: account.ProfileName,
			}
			// Check cooldown
			if account.CooldownUntil != nil && account.CooldownUntil.After(time.Now()) {
				explanation.AccountStatus.CooldownUntil = account.CooldownUntil
				explanation.AccountStatus.IsInCooldown = true
				explanation.BlockReasons = append(explanation.BlockReasons, "account cooldown active")
			}
		}
	}

	if IsJSONOutput() || IsJSONLOutput() {
		return WriteOutput(os.Stdout, explanation)
	}

	return writeAgentExplanationHuman(explanation)
}

func buildAgentExplanation(agent *models.Agent, queueItems []*models.QueueItem) *AgentExplanation {
	// Consider paused as blocked for explanation purposes
	// (agent.IsBlocked() only covers error/approval/rate-limit states)
	isBlocked := agent.IsBlocked() || agent.State == models.AgentStatePaused

	explanation := &AgentExplanation{
		AgentID:      agent.ID,
		Type:         agent.Type,
		State:        agent.State,
		StateInfo:    agent.StateInfo,
		IsBlocked:    isBlocked,
		LastActivity: agent.LastActivity,
		PausedUntil:  agent.PausedUntil,
	}

	// Count queue items
	for _, item := range queueItems {
		explanation.QueueStatus.TotalItems++
		if item.Status == models.QueueItemStatusPending {
			explanation.QueueStatus.PendingItems++
		}
	}

	// Determine block reasons and suggestions
	switch agent.State {
	case models.AgentStateAwaitingApproval:
		explanation.BlockReasons = append(explanation.BlockReasons, "waiting for user approval")
		explanation.Suggestions = append(explanation.Suggestions,
			fmt.Sprintf("Approve pending request: forge agent approve %s", shortID(agent.ID)))

	case models.AgentStateRateLimited:
		explanation.BlockReasons = append(explanation.BlockReasons, "rate limited by provider")
		explanation.Suggestions = append(explanation.Suggestions,
			"Wait for rate limit to expire",
			fmt.Sprintf("Switch to different account: forge agent rotate %s", shortID(agent.ID)))

	case models.AgentStateError:
		explanation.BlockReasons = append(explanation.BlockReasons, "agent encountered an error")
		if agent.StateInfo.Reason != "" {
			explanation.BlockReasons = append(explanation.BlockReasons, agent.StateInfo.Reason)
		}
		explanation.Suggestions = append(explanation.Suggestions,
			fmt.Sprintf("Check agent status: forge agent status %s", shortID(agent.ID)),
			fmt.Sprintf("Restart agent: forge agent restart %s", shortID(agent.ID)))

	case models.AgentStatePaused:
		explanation.BlockReasons = append(explanation.BlockReasons, "agent is paused")
		if agent.PausedUntil != nil {
			remaining := time.Until(*agent.PausedUntil)
			explanation.BlockReasons = append(explanation.BlockReasons,
				fmt.Sprintf("will resume in %s", remaining.Round(time.Second)))
		}
		explanation.Suggestions = append(explanation.Suggestions,
			fmt.Sprintf("Resume agent: forge agent resume %s", shortID(agent.ID)))

	case models.AgentStateWorking:
		explanation.Suggestions = append(explanation.Suggestions,
			fmt.Sprintf("Wait for completion: forge wait --agent %s --until idle", shortID(agent.ID)),
			fmt.Sprintf("Queue a message: forge send %s \"your message\"", shortID(agent.ID)))

	case models.AgentStateIdle:
		if explanation.QueueStatus.PendingItems > 0 {
			explanation.Suggestions = append(explanation.Suggestions,
				"Queue items are pending - scheduler will dispatch next item")
		} else {
			explanation.Suggestions = append(explanation.Suggestions,
				fmt.Sprintf("Send a message: forge send %s \"your prompt\"", shortID(agent.ID)))
		}
	}

	return explanation
}

func writeAgentExplanationHuman(e *AgentExplanation) error {
	// Header
	stateDisplay := formatAgentState(e.State)
	if e.IsBlocked {
		fmt.Printf("Agent %s is %s (BLOCKED)\n", shortID(e.AgentID), stateDisplay)
	} else {
		fmt.Printf("Agent %s is %s\n", shortID(e.AgentID), stateDisplay)
	}
	fmt.Println()

	// State details
	fmt.Printf("Type: %s\n", e.Type)
	if e.StateInfo.Reason != "" {
		fmt.Printf("Reason: %s\n", e.StateInfo.Reason)
	}
	if e.StateInfo.Confidence != "" {
		fmt.Printf("Confidence: %s\n", e.StateInfo.Confidence)
	}
	fmt.Println()

	// Block reasons
	if len(e.BlockReasons) > 0 {
		fmt.Println("Block Reasons:")
		for _, reason := range e.BlockReasons {
			fmt.Printf("  - %s\n", reason)
		}
		fmt.Println()
	}

	// Queue status
	fmt.Println("Queue Status:")
	fmt.Printf("  Total items: %d\n", e.QueueStatus.TotalItems)
	fmt.Printf("  Pending: %d\n", e.QueueStatus.PendingItems)
	fmt.Println()

	// Account status
	if e.AccountStatus != nil {
		fmt.Println("Account Status:")
		if e.AccountStatus.ProfileName != "" {
			fmt.Printf("  Profile: %s\n", e.AccountStatus.ProfileName)
		}
		if e.AccountStatus.IsInCooldown && e.AccountStatus.CooldownUntil != nil {
			remaining := time.Until(*e.AccountStatus.CooldownUntil)
			fmt.Printf("  Cooldown: active (ends in %s)\n", remaining.Round(time.Second))
		} else {
			fmt.Printf("  Cooldown: none\n")
		}
		fmt.Println()
	}

	// Suggestions
	if len(e.Suggestions) > 0 {
		fmt.Println("Suggestions:")
		for i, suggestion := range e.Suggestions {
			fmt.Printf("  %d. %s\n", i+1, suggestion)
		}
	}

	return nil
}

func explainQueueItem(ctx context.Context, database *db.DB, queueRepo *db.QueueRepository, agentRepo *db.AgentRepository, itemID string) error {
	item, err := queueRepo.Get(ctx, itemID)
	if err != nil {
		return fmt.Errorf("queue item not found: %s", itemID)
	}

	agent, err := agentRepo.Get(ctx, item.AgentID)
	if err != nil {
		return fmt.Errorf("failed to get agent: %w", err)
	}

	explanation := buildQueueItemExplanation(item, agent)

	if IsJSONOutput() || IsJSONLOutput() {
		return WriteOutput(os.Stdout, explanation)
	}

	return writeQueueItemExplanationHuman(explanation)
}

func buildQueueItemExplanation(item *models.QueueItem, agent *models.Agent) *QueueItemExplanation {
	explanation := &QueueItemExplanation{
		ItemID:     item.ID,
		AgentID:    item.AgentID,
		Type:       item.Type,
		Status:     item.Status,
		Position:   item.Position,
		AgentState: agent.State,
		CreatedAt:  item.CreatedAt,
	}

	// Extract content based on type
	switch item.Type {
	case models.QueueItemTypeMessage:
		var payload models.MessagePayload
		if err := json.Unmarshal(item.Payload, &payload); err == nil {
			explanation.Content = truncateString(payload.Text, 100)
		}
	case models.QueueItemTypePause:
		var payload models.PausePayload
		if err := json.Unmarshal(item.Payload, &payload); err == nil {
			explanation.Content = fmt.Sprintf("%ds pause", payload.DurationSeconds)
			if payload.Reason != "" {
				explanation.Content += fmt.Sprintf(" (%s)", payload.Reason)
			}
		}
	case models.QueueItemTypeConditional:
		var payload models.ConditionalPayload
		if err := json.Unmarshal(item.Payload, &payload); err == nil {
			explanation.Condition = &payload
			explanation.Content = truncateString(payload.Message, 100)
		}
	}

	// Determine if blocked and why
	if item.Status == models.QueueItemStatusPending {
		// Check if agent state blocks dispatch
		switch agent.State {
		case models.AgentStateWorking:
			explanation.IsBlocked = true
			explanation.BlockReasons = append(explanation.BlockReasons, "agent is currently working")
			explanation.Suggestions = append(explanation.Suggestions,
				"Wait for agent to become idle",
				fmt.Sprintf("forge wait --agent %s --until idle", shortID(agent.ID)))

		case models.AgentStateAwaitingApproval:
			explanation.IsBlocked = true
			explanation.BlockReasons = append(explanation.BlockReasons, "agent is waiting for approval")
			explanation.Suggestions = append(explanation.Suggestions,
				fmt.Sprintf("Approve pending request: forge agent approve %s", shortID(agent.ID)))

		case models.AgentStatePaused:
			explanation.IsBlocked = true
			explanation.BlockReasons = append(explanation.BlockReasons, "agent is paused")
			if agent.PausedUntil != nil {
				remaining := time.Until(*agent.PausedUntil)
				explanation.BlockReasons = append(explanation.BlockReasons,
					fmt.Sprintf("will resume in %s", remaining.Round(time.Second)))
			}
			explanation.Suggestions = append(explanation.Suggestions,
				fmt.Sprintf("Resume agent: forge agent resume %s", shortID(agent.ID)))

		case models.AgentStateError, models.AgentStateStopped:
			explanation.IsBlocked = true
			explanation.BlockReasons = append(explanation.BlockReasons,
				fmt.Sprintf("agent is in %s state", agent.State))
			explanation.Suggestions = append(explanation.Suggestions,
				fmt.Sprintf("Check agent status: forge agent status %s", shortID(agent.ID)),
				fmt.Sprintf("Restart agent: forge agent restart %s", shortID(agent.ID)))

		case models.AgentStateRateLimited:
			explanation.IsBlocked = true
			explanation.BlockReasons = append(explanation.BlockReasons, "agent is rate limited")
			explanation.Suggestions = append(explanation.Suggestions,
				"Wait for rate limit to expire",
				fmt.Sprintf("Rotate account: forge agent rotate %s", shortID(agent.ID)))
		}

		// Check conditional gates
		if item.Type == models.QueueItemTypeConditional && explanation.Condition != nil {
			switch explanation.Condition.ConditionType {
			case models.ConditionTypeWhenIdle:
				if agent.State != models.AgentStateIdle {
					explanation.IsBlocked = true
					explanation.BlockReasons = append(explanation.BlockReasons,
						"conditional: waiting for agent to be idle")
				}
			case models.ConditionTypeAfterCooldown:
				explanation.IsBlocked = true
				explanation.BlockReasons = append(explanation.BlockReasons,
					"conditional: waiting for cooldown to expire")
			}
		}

		// Position-based blocking
		if item.Position > 1 && !explanation.IsBlocked {
			explanation.BlockReasons = append(explanation.BlockReasons,
				fmt.Sprintf("waiting for %d items ahead in queue", item.Position-1))
		}
	}

	return explanation
}

func writeQueueItemExplanationHuman(e *QueueItemExplanation) error {
	// Header
	statusDisplay := string(e.Status)
	if e.IsBlocked {
		statusDisplay += " (BLOCKED)"
	}
	fmt.Printf("Queue Item %s is %s\n", e.ItemID, statusDisplay)
	fmt.Println()

	// Details
	fmt.Printf("Type: %s\n", e.Type)
	fmt.Printf("Position: %d\n", e.Position)
	fmt.Printf("Agent: %s (%s)\n", shortID(e.AgentID), formatAgentState(e.AgentState))
	fmt.Printf("Created: %s\n", formatRelativeTime(e.CreatedAt))
	if e.Content != "" {
		fmt.Printf("Content: %s\n", e.Content)
	}
	fmt.Println()

	// Condition details
	if e.Condition != nil {
		fmt.Println("Condition:")
		fmt.Printf("  Type: %s\n", e.Condition.ConditionType)
		if e.Condition.Expression != "" {
			fmt.Printf("  Expression: %s\n", e.Condition.Expression)
		}
		fmt.Println()
	}

	// Block reasons
	if len(e.BlockReasons) > 0 {
		fmt.Println("Block Reasons:")
		for _, reason := range e.BlockReasons {
			fmt.Printf("  - %s\n", reason)
		}
		fmt.Println()
	}

	// Suggestions
	if len(e.Suggestions) > 0 {
		fmt.Println("Suggestions:")
		for i, suggestion := range e.Suggestions {
			fmt.Printf("  %d. %s\n", i+1, suggestion)
		}
	}

	return nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// formatRelativeTime is defined in vault.go
