// Package cli provides queue management CLI commands.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/opencode-ai/swarm/internal/db"
	"github.com/opencode-ai/swarm/internal/models"
	"github.com/spf13/cobra"
)

var (
	// queue ls flags
	queueListAgent  string
	queueListStatus string
	queueListLimit  int
	queueListAll    bool
)

func init() {
	rootCmd.AddCommand(queueCmd)
	queueCmd.AddCommand(queueListCmd)

	// List flags
	queueListCmd.Flags().StringVarP(&queueListAgent, "agent", "a", "", "filter by agent ID")
	queueListCmd.Flags().StringVar(&queueListStatus, "status", "", "filter by status (pending, dispatched, completed, failed, skipped)")
	queueListCmd.Flags().IntVarP(&queueListLimit, "limit", "n", 20, "max items to show")
	queueListCmd.Flags().BoolVar(&queueListAll, "all", false, "show all items including completed")
}

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage agent queues",
	Long: `Manage agent message queues.

The queue system controls message dispatch to agents. Messages are queued
and dispatched when agents become available.`,
}

var queueListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List queue items",
	Long: `List queue items for agents.

By default, shows pending and dispatched items. Use --all to include completed items.
Use --agent to filter by a specific agent, or omit to show queues for all agents
in the current workspace.`,
	Example: `  # List queue for current workspace
  swarm queue ls

  # List queue for a specific agent
  swarm queue ls --agent abc123

  # Filter by status
  swarm queue ls --status pending

  # Show all items including completed
  swarm queue ls --all

  # Limit results
  swarm queue ls -n 50`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		wsRepo := db.NewWorkspaceRepository(database)
		agentRepo := db.NewAgentRepository(database)
		queueRepo := db.NewQueueRepository(database)

		// Determine which agents to query
		var agentIDs []string

		if queueListAgent != "" {
			// Specific agent requested
			agent, err := findAgent(ctx, agentRepo, queueListAgent)
			if err != nil {
				return err
			}
			agentIDs = append(agentIDs, agent.ID)
		} else {
			// Use workspace context to find agents
			resolved, err := ResolveWorkspaceContext(ctx, wsRepo, "")
			if err == nil && resolved != nil && resolved.WorkspaceID != "" {
				// Get agents in this workspace
				agents, err := agentRepo.ListByWorkspace(ctx, resolved.WorkspaceID)
				if err != nil {
					return fmt.Errorf("failed to list workspace agents: %w", err)
				}
				for _, a := range agents {
					agentIDs = append(agentIDs, a.ID)
				}
			} else {
				// No workspace context, list all agents
				agents, err := agentRepo.List(ctx)
				if err != nil {
					return fmt.Errorf("failed to list agents: %w", err)
				}
				for _, a := range agents {
					agentIDs = append(agentIDs, a.ID)
				}
			}
		}

		if len(agentIDs) == 0 {
			if IsJSONOutput() || IsJSONLOutput() {
				return WriteOutput(os.Stdout, []any{})
			}
			fmt.Println("No agents found")
			return nil
		}

		// Collect all queue items
		type queueItemWithAgent struct {
			*models.QueueItem
			AgentShortID string `json:"agent_short_id"`
		}

		var allItems []queueItemWithAgent
		for _, agentID := range agentIDs {
			var items []*models.QueueItem
			var err error

			if queueListAll {
				items, err = queueRepo.List(ctx, agentID)
			} else {
				items, err = queueRepo.List(ctx, agentID)
				// Filter out completed items if not --all
				filtered := make([]*models.QueueItem, 0, len(items))
				for _, item := range items {
					if item.Status != models.QueueItemStatusCompleted &&
						item.Status != models.QueueItemStatusFailed &&
						item.Status != models.QueueItemStatusSkipped {
						filtered = append(filtered, item)
					}
				}
				items = filtered
			}

			if err != nil {
				return fmt.Errorf("failed to list queue for agent %s: %w", agentID, err)
			}

			// Apply status filter if specified
			if queueListStatus != "" {
				statusFilter := models.QueueItemStatus(queueListStatus)
				filtered := make([]*models.QueueItem, 0)
				for _, item := range items {
					if item.Status == statusFilter {
						filtered = append(filtered, item)
					}
				}
				items = filtered
			}

			for _, item := range items {
				allItems = append(allItems, queueItemWithAgent{
					QueueItem:    item,
					AgentShortID: shortID(agentID),
				})
			}
		}

		// Apply limit
		if queueListLimit > 0 && len(allItems) > queueListLimit {
			allItems = allItems[:queueListLimit]
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, allItems)
		}

		if len(allItems) == 0 {
			fmt.Println("No queue items found")
			return nil
		}

		// Print header
		if queueListAgent != "" {
			fmt.Printf("QUEUE FOR AGENT %s (%d items)\n\n", shortID(queueListAgent), len(allItems))
		} else {
			fmt.Printf("QUEUE ITEMS (%d total)\n\n", len(allItems))
		}

		// Get agent states for block reason calculation
		agentStates := make(map[string]models.AgentState)
		for _, agentID := range agentIDs {
			agent, err := agentRepo.Get(ctx, agentID)
			if err == nil {
				agentStates[agentID] = agent.State
			}
		}

		// Build table rows
		rows := make([][]string, 0, len(allItems))
		for _, item := range allItems {
			content := formatQueueContent(item.QueueItem)
			created := formatRelativeTime(item.CreatedAt)
			blockReason := formatBlockReason(item.QueueItem, agentStates[item.AgentID])

			row := []string{
				fmt.Sprintf("%d", item.Position),
				string(item.Type),
				formatQueueStatus(item.Status),
				blockReason,
				content,
				created,
			}

			// Add agent column if showing multiple agents
			if queueListAgent == "" {
				row = append([]string{item.AgentShortID}, row...)
			}

			rows = append(rows, row)
		}

		// Build headers
		headers := []string{"POS", "TYPE", "STATUS", "BLOCK", "CONTENT", "CREATED"}
		if queueListAgent == "" {
			headers = append([]string{"AGENT"}, headers...)
		}

		return writeTable(os.Stdout, headers, rows)
	},
}

// formatQueueContent extracts a preview of the queue item content.
func formatQueueContent(item *models.QueueItem) string {
	switch item.Type {
	case models.QueueItemTypeMessage:
		var payload models.MessagePayload
		if err := json.Unmarshal(item.Payload, &payload); err == nil {
			return truncate(payload.Text, 40)
		}
	case models.QueueItemTypePause:
		var payload models.PausePayload
		if err := json.Unmarshal(item.Payload, &payload); err == nil {
			return fmt.Sprintf("%ds pause", payload.DurationSeconds)
		}
	case models.QueueItemTypeConditional:
		var payload models.ConditionalPayload
		if err := json.Unmarshal(item.Payload, &payload); err == nil {
			return fmt.Sprintf("[%s] %s", payload.ConditionType, truncate(payload.Message, 30))
		}
	}
	return "-"
}

// formatQueueStatus formats the queue item status for display.
func formatQueueStatus(status models.QueueItemStatus) string {
	switch status {
	case models.QueueItemStatusPending:
		return "pending"
	case models.QueueItemStatusDispatched:
		return "dispatched"
	case models.QueueItemStatusCompleted:
		return "completed"
	case models.QueueItemStatusFailed:
		return "failed"
	case models.QueueItemStatusSkipped:
		return "skipped"
	default:
		return string(status)
	}
}

// formatBlockReason returns a human-readable reason why a queue item is blocked.
func formatBlockReason(item *models.QueueItem, agentState models.AgentState) string {
	// Only pending items can be blocked
	if item.Status != models.QueueItemStatusPending {
		return "-"
	}

	// Check agent state
	switch agentState {
	case models.AgentStateWorking:
		return "agent_busy"
	case models.AgentStatePaused:
		return "paused"
	case models.AgentStateRateLimited:
		return "cooldown"
	case models.AgentStateAwaitingApproval:
		return "approval"
	case models.AgentStateError:
		return "error"
	case models.AgentStateStopped:
		return "stopped"
	case models.AgentStateStarting:
		return "starting"
	}

	// Check if it's a conditional item with unmet condition
	if item.Type == models.QueueItemTypeConditional {
		var payload models.ConditionalPayload
		if err := json.Unmarshal(item.Payload, &payload); err == nil {
			switch payload.ConditionType {
			case models.ConditionTypeWhenIdle:
				if agentState != models.AgentStateIdle {
					return "idle_gate"
				}
			case models.ConditionTypeAfterCooldown:
				return "cooldown_gate"
			case models.ConditionTypeAfterPrevious:
				return "dependency"
			}
		}
	}

	// If it's position > 1, previous items are blocking
	if item.Position > 1 {
		return "queue_order"
	}

	return "-"
}
