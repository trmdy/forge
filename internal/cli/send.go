// Package cli provides the send command for queueing messages to agents.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/opencode-ai/swarm/internal/db"
	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/node"
	"github.com/opencode-ai/swarm/internal/queue"
	"github.com/opencode-ai/swarm/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	sendPriority string
	sendFront    bool
	sendWhenIdle bool
	sendAll      bool
	sendFile     string
	sendStdin    bool
	sendEditor   bool
)

func init() {
	rootCmd.AddCommand(sendCmd)

	sendCmd.Flags().StringVar(&sendPriority, "priority", "normal", "queue priority (high, normal, low)")
	sendCmd.Flags().BoolVar(&sendFront, "front", false, "insert at front of queue")
	sendCmd.Flags().BoolVar(&sendWhenIdle, "when-idle", false, "only dispatch when agent is idle (conditional)")
	sendCmd.Flags().BoolVar(&sendAll, "all", false, "send to all agents in workspace")
	sendCmd.Flags().StringVarP(&sendFile, "file", "f", "", "read message from file")
	sendCmd.Flags().BoolVar(&sendStdin, "stdin", false, "read message from stdin")
	sendCmd.Flags().BoolVar(&sendEditor, "editor", false, "compose message in $EDITOR")
}

var sendCmd = &cobra.Command{
	Use:   "send [agent] <message>",
	Short: "Queue a message for an agent",
	Long: `Queue a message to be sent to an agent by the scheduler.

Messages are enqueued and dispatched when the agent is ready (idle).
This is the safe, queue-based way to send messages. For immediate
injection (dangerous), use 'swarm inject'.

If no agent is specified, uses the agent from current context.`,
	Example: `  # Queue a message for a specific agent
  swarm send abc123 "Fix the lint errors"

  # Queue using context (no agent specified)
  swarm send "Continue with the task"

  # Queue for all agents in workspace
  swarm send --all "Pause and commit your work"

  # Queue with high priority (dispatched before normal items)
  swarm send --priority high abc123 "Urgent: revert last change"

  # Queue at front (first to be dispatched)
  swarm send --front abc123 "Do this next"

  # Queue conditional message (only dispatch when idle)
  swarm send --when-idle abc123 "Continue when ready"

  # Queue from file
  swarm send abc123 --file prompt.txt

  # Queue from stdin
  cat prompt.txt | swarm send abc123 --stdin

  # Compose in $EDITOR
  swarm send abc123 --editor`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		nodeRepo := db.NewNodeRepository(database)
		nodeService := node.NewService(nodeRepo)
		wsRepo := db.NewWorkspaceRepository(database)
		agentRepo := db.NewAgentRepository(database)
		queueRepo := db.NewQueueRepository(database)
		wsService := workspace.NewService(wsRepo, nodeService, agentRepo)
		_ = wsService // for future use with --all

		queueService := queue.NewService(queueRepo)

		// Resolve agent(s) to send to
		var targetAgents []*models.Agent

		if sendAll {
			// Get workspace context
			wsCtx, err := RequireWorkspaceContext(ctx, wsRepo, "")
			if err != nil {
				return fmt.Errorf("--all requires workspace context: %w", err)
			}

			agents, err := agentRepo.ListByWorkspace(ctx, wsCtx.WorkspaceID)
			if err != nil {
				return fmt.Errorf("failed to list agents: %w", err)
			}
			if len(agents) == 0 {
				return errors.New("no agents in workspace")
			}
			targetAgents = agents
		} else {
			// Single agent - either from args or context
			var agentID string
			var messageArgs []string

			if len(args) == 0 {
				// No args - need context
				agentCtx, err := RequireAgentContext(ctx, agentRepo, "", "")
				if err != nil {
					return err
				}
				agentID = agentCtx.AgentID
				messageArgs = args
			} else {
				// First arg could be agent ID or message
				// Try to resolve as agent first
				agent, err := findAgent(ctx, agentRepo, args[0])
				if err == nil {
					agentID = agent.ID
					messageArgs = args[1:]
				} else {
					// Not a valid agent - try context
					agentCtx, err := RequireAgentContext(ctx, agentRepo, "", "")
					if err != nil {
						return fmt.Errorf("first argument '%s' is not a valid agent and no context set", args[0])
					}
					agentID = agentCtx.AgentID
					messageArgs = args // entire args is the message
				}
			}

			agent, err := agentRepo.Get(ctx, agentID)
			if err != nil {
				return fmt.Errorf("agent not found: %w", err)
			}
			targetAgents = []*models.Agent{agent}

			// Update args for message resolution
			args = messageArgs
		}

		// Resolve message
		message, err := resolveMessage(args, sendFile, sendStdin, sendEditor)
		if err != nil {
			return err
		}

		// Create queue item(s)
		results := make([]sendResult, 0, len(targetAgents))

		for _, agent := range targetAgents {
			var item *models.QueueItem

			if sendWhenIdle {
				// Create conditional item
				payload := models.ConditionalPayload{
					ConditionType: models.ConditionTypeWhenIdle,
					Message:       message,
				}
				payloadBytes, _ := json.Marshal(payload)
				item = &models.QueueItem{
					AgentID: agent.ID,
					Type:    models.QueueItemTypeConditional,
					Status:  models.QueueItemStatusPending,
					Payload: payloadBytes,
				}
			} else {
				// Create regular message item
				payload := models.MessagePayload{Text: message}
				payloadBytes, _ := json.Marshal(payload)
				item = &models.QueueItem{
					AgentID: agent.ID,
					Type:    models.QueueItemTypeMessage,
					Status:  models.QueueItemStatusPending,
					Payload: payloadBytes,
				}
			}

			// Enqueue the item
			if sendFront {
				err = queueService.InsertAt(ctx, agent.ID, 0, item)
			} else {
				err = queueService.Enqueue(ctx, agent.ID, item)
			}

			if err != nil {
				results = append(results, sendResult{
					AgentID: agent.ID,
					Error:   err.Error(),
				})
				continue
			}

			// Get queue position
			queueItems, _ := queueService.List(ctx, agent.ID)
			position := len(queueItems)
			for i, qi := range queueItems {
				if qi.ID == item.ID {
					position = i + 1
					break
				}
			}

			results = append(results, sendResult{
				AgentID:  agent.ID,
				ItemID:   item.ID,
				Position: position,
				ItemType: string(item.Type),
			})
		}

		// Output results
		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{
				"queued":  true,
				"results": results,
				"message": truncateMessage(message, 100),
			})
		}

		// Human-readable output
		for _, r := range results {
			if r.Error != "" {
				fmt.Printf("✗ Failed to queue for agent %s: %s\n", shortID(r.AgentID), r.Error)
			} else {
				positionStr := fmt.Sprintf("#%d", r.Position)
				if sendFront {
					positionStr = "#1 (front)"
				}
				typeStr := ""
				if r.ItemType == string(models.QueueItemTypeConditional) {
					typeStr = " (when idle)"
				}
				fmt.Printf("✓ Queued for agent %s at position %s%s\n", shortID(r.AgentID), positionStr, typeStr)
			}
		}

		if len(results) == 1 && results[0].Error == "" {
			fmt.Printf("  \"%s\"\n", truncateMessage(message, 60))
		}

		return nil
	},
}

type sendResult struct {
	AgentID  string `json:"agent_id"`
	ItemID   string `json:"item_id,omitempty"`
	Position int    `json:"position,omitempty"`
	ItemType string `json:"item_type,omitempty"`
	Error    string `json:"error,omitempty"`
}

func truncateMessage(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen-3] + "..."
}
