// Package cli provides queue management commands.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/opencode-ai/swarm/internal/db"
	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/scheduler"
	"github.com/spf13/cobra"
)

var (
	queueAgent  string
	queueStatus string
	queueLimit  int
	queueAll    bool
)

func init() {
	rootCmd.AddCommand(queueCmd)
	queueCmd.AddCommand(queueListCmd)

	queueListCmd.Flags().StringVarP(&queueAgent, "agent", "a", "", "filter by agent ID or prefix")
	queueListCmd.Flags().StringVar(&queueStatus, "status", "", "filter by status (pending, blocked, dispatched, completed, failed, skipped)")
	queueListCmd.Flags().IntVarP(&queueLimit, "limit", "n", 20, "max items to show per agent (0 = unlimited)")
	queueListCmd.Flags().BoolVar(&queueAll, "all", false, "show all items including completed")
}

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage agent queues",
	Long:  "Inspect and manage queued messages for agents.",
}

var queueListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List queue items",
	Long: `List queued items and their dispatch status.

By default, this lists queues for agents in the current workspace context.
Use --agent to target a specific agent.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		statusFilter, err := normalizeQueueStatus(queueStatus)
		if err != nil {
			return err
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		agentRepo := db.NewAgentRepository(database)
		queueRepo := db.NewQueueRepository(database)
		wsRepo := db.NewWorkspaceRepository(database)

		agents, err := resolveQueueAgents(ctx, agentRepo, wsRepo, queueAgent)
		if err != nil {
			return err
		}
		if len(agents) == 0 {
			if IsJSONOutput() || IsJSONLOutput() {
				return WriteOutput(os.Stdout, []queueListAgent{})
			}
			fmt.Println("No agents found")
			return nil
		}

		output := make([]queueListAgent, 0, len(agents))
		anyRows := false

		for _, a := range agents {
			items, err := queueRepo.List(ctx, a.ID)
			if err != nil {
				return fmt.Errorf("failed to list queue for agent %s: %w", a.ID, err)
			}

			view := buildQueueView(a, items, statusFilter, queueAll, queueLimit)
			if len(view) == 0 {
				continue
			}
			anyRows = true

			output = append(output, queueListAgent{
				AgentID:   a.ID,
				AgentType: a.Type,
				Items:     view,
			})
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, output)
		}

		if !anyRows {
			fmt.Println("No queue items found")
			return nil
		}

		for i, agentView := range output {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("QUEUE FOR AGENT %s (%d items)\n\n", shortID(agentView.AgentID), len(agentView.Items))

			rows := make([][]string, 0, len(agentView.Items))
			for _, item := range agentView.Items {
				blockReason := item.BlockReason
				if blockReason == "" {
					blockReason = "-"
				}

				rows = append(rows, []string{
					fmt.Sprintf("%d", item.Item.Position),
					string(item.Item.Type),
					item.DisplayStatus,
					blockReason,
					item.Preview,
					formatRelativeTime(item.Item.CreatedAt),
				})
			}

			if err := writeTable(os.Stdout, []string{"POS", "TYPE", "STATUS", "BLOCK REASON", "CONTENT", "CREATED"}, rows); err != nil {
				return err
			}
		}

		return nil
	},
}

type queueListAgent struct {
	AgentID   string           `json:"agent_id"`
	AgentType models.AgentType `json:"agent_type"`
	Items     []queueListItem  `json:"items"`
}

type queueListItem struct {
	Item          *models.QueueItem `json:"item"`
	DisplayStatus string            `json:"display_status"`
	BlockReason   string            `json:"block_reason,omitempty"`
	Preview       string            `json:"preview"`
}

func resolveQueueAgents(ctx context.Context, agentRepo *db.AgentRepository, wsRepo *db.WorkspaceRepository, agentFilter string) ([]*models.Agent, error) {
	if strings.TrimSpace(agentFilter) != "" {
		agent, err := findAgent(ctx, agentRepo, agentFilter)
		if err != nil {
			return nil, err
		}
		return []*models.Agent{agent}, nil
	}

	resolved, err := ResolveWorkspaceContext(ctx, wsRepo, "")
	if err != nil {
		return nil, err
	}
	if resolved != nil && resolved.WorkspaceID != "" {
		agents, err := agentRepo.ListByWorkspace(ctx, resolved.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to list agents: %w", err)
		}
		sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
		return agents, nil
	}

	agents, err := agentRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	return agents, nil
}

func buildQueueView(agent *models.Agent, items []*models.QueueItem, statusFilter string, showAll bool, limit int) []queueListItem {
	if len(items) == 0 {
		return nil
	}

	pendingCount := 0
	for _, item := range items {
		if item != nil && item.Status == models.QueueItemStatusPending {
			pendingCount++
		}
	}

	view := make([]queueListItem, 0, len(items))
	pendingSeen := false

	for _, item := range items {
		if item == nil {
			continue
		}

		isPending := item.Status == models.QueueItemStatusPending
		isFirstPending := isPending && !pendingSeen
		if isPending && !pendingSeen {
			pendingSeen = true
		}

		displayStatus, blockReason := deriveQueueDisplay(agent, item, isFirstPending, pendingCount)
		if !includeQueueItem(item, displayStatus, statusFilter, showAll) {
			continue
		}

		view = append(view, queueListItem{
			Item:          item,
			DisplayStatus: displayStatus,
			BlockReason:   blockReason,
			Preview:       truncateMessage(queueItemPreview(item), 60),
		})

		if limit > 0 && len(view) >= limit {
			break
		}
	}

	return view
}

func deriveQueueDisplay(agent *models.Agent, item *models.QueueItem, firstPending bool, pendingCount int) (string, string) {
	if item.Status != models.QueueItemStatusPending {
		return string(item.Status), ""
	}

	if !firstPending {
		return "blocked", "dependency"
	}

	if item.Type == models.QueueItemTypeConditional {
		reason, blocked := conditionalBlockReason(agent, item, pendingCount)
		if blocked {
			return "blocked", reason
		}
	}

	stateReason := agentStateBlockReason(agent)
	if stateReason != "" {
		return "blocked", stateReason
	}

	return "pending", ""
}

func conditionalBlockReason(agent *models.Agent, item *models.QueueItem, pendingCount int) (string, bool) {
	var payload models.ConditionalPayload
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return "dependency", true
	}

	evaluator := scheduler.NewConditionEvaluator()
	ctx := scheduler.ConditionContext{
		Agent:       agent,
		QueueLength: pendingCount,
		Now:         time.Now().UTC(),
	}
	result, err := evaluator.Evaluate(context.Background(), ctx, payload)
	if err != nil {
		return "dependency", true
	}
	if result.Met {
		return "", false
	}

	return conditionTypeReason(payload.ConditionType), true
}

func conditionTypeReason(condition models.ConditionType) string {
	switch condition {
	case models.ConditionTypeWhenIdle:
		return "idle_gate"
	case models.ConditionTypeAfterCooldown:
		return "cooldown"
	case models.ConditionTypeAfterPrevious:
		return "dependency"
	case models.ConditionTypeCustomExpression:
		return "dependency"
	default:
		return "dependency"
	}
}

func agentStateBlockReason(agent *models.Agent) string {
	if agent == nil {
		return "busy"
	}
	switch agent.State {
	case models.AgentStatePaused:
		return "paused"
	case models.AgentStateRateLimited:
		return "cooldown"
	case models.AgentStateWorking, models.AgentStateStarting, models.AgentStateAwaitingApproval:
		return "busy"
	case models.AgentStateError, models.AgentStateStopped:
		return "busy"
	default:
		return ""
	}
}

func includeQueueItem(item *models.QueueItem, displayStatus string, statusFilter string, showAll bool) bool {
	if statusFilter != "" {
		if statusFilter == "blocked" {
			return displayStatus == "blocked"
		}
		if statusFilter == "pending" {
			return displayStatus == "pending"
		}
		return strings.EqualFold(string(item.Status), statusFilter)
	}

	if showAll {
		return true
	}

	if displayStatus == "pending" || displayStatus == "blocked" {
		return true
	}
	return item.Status == models.QueueItemStatusDispatched
}

func normalizeQueueStatus(status string) (string, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" {
		return "", nil
	}

	switch status {
	case "pending", "blocked", "dispatched", "completed", "failed", "skipped":
		return status, nil
	default:
		return "", errors.New("invalid status (use pending, blocked, dispatched, completed, failed, or skipped)")
	}
}

func queueItemPreview(item *models.QueueItem) string {
	switch item.Type {
	case models.QueueItemTypeMessage:
		var payload models.MessagePayload
		if err := json.Unmarshal(item.Payload, &payload); err != nil {
			return "invalid message payload"
		}
		return payload.Text
	case models.QueueItemTypePause:
		var payload models.PausePayload
		if err := json.Unmarshal(item.Payload, &payload); err != nil {
			return "invalid pause payload"
		}
		label := fmt.Sprintf("pause %ds", payload.DurationSeconds)
		if strings.TrimSpace(payload.Reason) != "" {
			label += " - " + strings.TrimSpace(payload.Reason)
		}
		return label
	case models.QueueItemTypeConditional:
		var payload models.ConditionalPayload
		if err := json.Unmarshal(item.Payload, &payload); err != nil {
			return "invalid conditional payload"
		}
		return payload.Message
	default:
		return "unknown queue item"
	}
}
