// Package cli provides the wait command for automation scripts.
package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/node"
	"github.com/tOgg1/forge/internal/workspace"
)

// Wait condition types
const (
	WaitConditionIdle         = "idle"
	WaitConditionQueueEmpty   = "queue-empty"
	WaitConditionCooldownOver = "cooldown-over"
	WaitConditionAllIdle      = "all-idle"
	WaitConditionAnyIdle      = "any-idle"
	WaitConditionReady        = "ready"
)

var (
	waitUntil        string
	waitAgent        string
	waitWorkspace    string
	waitTimeout      time.Duration
	waitPollInterval time.Duration
	waitQuiet        bool
)

func init() {
	rootCmd.AddCommand(waitCmd)

	waitCmd.Flags().StringVarP(&waitUntil, "until", "u", "", "condition to wait for (required)")
	waitCmd.Flags().StringVarP(&waitAgent, "agent", "a", "", "agent to wait for")
	waitCmd.Flags().StringVarP(&waitWorkspace, "workspace", "w", "", "workspace to wait for")
	waitCmd.Flags().DurationVarP(&waitTimeout, "timeout", "t", 30*time.Minute, "maximum wait time")
	waitCmd.Flags().DurationVar(&waitPollInterval, "poll-interval", 2*time.Second, "check interval")
	waitCmd.Flags().BoolVarP(&waitQuiet, "quiet", "q", false, "no output, just wait")

	if err := waitCmd.MarkFlagRequired("until"); err != nil {
		panic(err)
	}
}

var waitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Wait for a condition to be met",
	Long: `Wait for a condition to be met before continuing.

Useful for automation scripts that need to wait for agents to reach
a certain state before proceeding.

Exit codes:
  0: Condition met
  1: Timeout reached
  2: Agent/workspace not found`,
	Example: `  # Wait for agent to be idle
  forge wait --agent abc123 --until idle

  # Wait for queue to drain
  forge wait --agent abc123 --until queue-empty

  # Wait for account cooldown to expire
  forge wait --agent abc123 --until cooldown-over

  # Wait for all agents in workspace to be idle
  forge wait --workspace myworkspace --until all-idle

  # Wait for agent to be fully ready (idle + no cooldown + queue empty)
  forge wait --agent abc123 --until ready

  # With timeout
  forge wait --agent abc123 --until idle --timeout 5m

  # Quiet mode (no output)
  forge wait --agent abc123 --until idle --quiet`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Validate condition
		validConditions := []string{
			WaitConditionIdle,
			WaitConditionQueueEmpty,
			WaitConditionCooldownOver,
			WaitConditionAllIdle,
			WaitConditionAnyIdle,
			WaitConditionReady,
		}
		isValid := false
		for _, c := range validConditions {
			if waitUntil == c {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid condition '%s'; valid conditions: %v", waitUntil, validConditions)
		}

		// Validate target
		needsAgent := waitUntil == WaitConditionIdle ||
			waitUntil == WaitConditionQueueEmpty ||
			waitUntil == WaitConditionCooldownOver ||
			waitUntil == WaitConditionReady

		needsWorkspace := waitUntil == WaitConditionAllIdle || waitUntil == WaitConditionAnyIdle

		if needsAgent && waitAgent == "" {
			// Try to get from context
			database, err := openDatabase()
			if err != nil {
				return err
			}
			agentRepo := db.NewAgentRepository(database)
			agentCtx, err := RequireAgentContext(ctx, agentRepo, "", "")
			database.Close()
			if err != nil {
				return fmt.Errorf("--agent is required for condition '%s' (no context set)", waitUntil)
			}
			waitAgent = agentCtx.AgentID
		}

		if needsWorkspace && waitWorkspace == "" {
			// Try to get from context
			database, err := openDatabase()
			if err != nil {
				return err
			}
			wsRepo := db.NewWorkspaceRepository(database)
			wsCtx, err := RequireWorkspaceContext(ctx, wsRepo, "")
			database.Close()
			if err != nil {
				return fmt.Errorf("--workspace is required for condition '%s' (no context set)", waitUntil)
			}
			waitWorkspace = wsCtx.WorkspaceID
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		nodeRepo := db.NewNodeRepository(database)
		nodeService := node.NewService(nodeRepo, node.WithPublisher(newEventPublisher(database)))
		wsRepo := db.NewWorkspaceRepository(database)
		agentRepo := db.NewAgentRepository(database)
		queueRepo := db.NewQueueRepository(database)
		accountRepo := db.NewAccountRepository(database)
		wsService := workspace.NewService(wsRepo, nodeService, agentRepo, workspace.WithPublisher(newEventPublisher(database)))
		_ = wsService

		startTime := time.Now()
		deadline := startTime.Add(waitTimeout)

		if !waitQuiet && !IsJSONOutput() && !IsJSONLOutput() {
			fmt.Printf("Waiting for condition '%s'...\n", waitUntil)
		}

		ticker := time.NewTicker(waitPollInterval)
		defer ticker.Stop()

		lastStatus := ""
		for {
			// Check if timeout reached
			if time.Now().After(deadline) {
				if IsJSONOutput() || IsJSONLOutput() {
					return WriteOutput(os.Stdout, map[string]any{
						"success":   false,
						"condition": waitUntil,
						"reason":    "timeout",
						"elapsed":   time.Since(startTime).String(),
					})
				}
				if !waitQuiet {
					fmt.Printf("\nTimeout reached after %s\n", time.Since(startTime).Round(time.Second))
				}
				os.Exit(1)
			}

			// Check condition
			met, status, err := checkWaitCondition(ctx, waitUntil, agentRepo, queueRepo, accountRepo, wsRepo)
			if err != nil {
				if IsJSONOutput() || IsJSONLOutput() {
					return WriteOutput(os.Stdout, map[string]any{
						"success":   false,
						"condition": waitUntil,
						"reason":    "error",
						"error":     err.Error(),
					})
				}
				return err
			}

			if met {
				elapsed := time.Since(startTime).Round(time.Second)
				if IsJSONOutput() || IsJSONLOutput() {
					return WriteOutput(os.Stdout, map[string]any{
						"success":   true,
						"condition": waitUntil,
						"elapsed":   elapsed.String(),
					})
				}
				if !waitQuiet {
					fmt.Printf("\nCondition '%s' met (waited %s)\n", waitUntil, elapsed)
				}
				return nil
			}

			// Print status update if changed
			if !waitQuiet && !IsJSONOutput() && !IsJSONLOutput() && status != lastStatus {
				elapsed := time.Since(startTime).Round(time.Second)
				fmt.Printf("  %s (elapsed: %s)\n", status, elapsed)
				lastStatus = status
			}

			<-ticker.C
		}
	},
}

// checkWaitCondition checks if the wait condition is met.
// Returns (met, status, error).
func checkWaitCondition(
	ctx context.Context,
	condition string,
	agentRepo *db.AgentRepository,
	queueRepo *db.QueueRepository,
	accountRepo *db.AccountRepository,
	wsRepo *db.WorkspaceRepository,
) (bool, string, error) {
	switch condition {
	case WaitConditionIdle:
		agent, err := findAgent(ctx, agentRepo, waitAgent)
		if err != nil {
			return false, "", fmt.Errorf("agent not found: %w", err)
		}
		if agent.State == models.AgentStateIdle {
			return true, "idle", nil
		}
		return false, fmt.Sprintf("state: %s", agent.State), nil

	case WaitConditionQueueEmpty:
		agent, err := findAgent(ctx, agentRepo, waitAgent)
		if err != nil {
			return false, "", fmt.Errorf("agent not found: %w", err)
		}
		items, err := queueRepo.List(ctx, agent.ID)
		if err != nil {
			return false, "", fmt.Errorf("failed to check queue: %w", err)
		}
		pending := 0
		for _, item := range items {
			if item.Status == models.QueueItemStatusPending {
				pending++
			}
		}
		if pending == 0 {
			return true, "queue empty", nil
		}
		return false, fmt.Sprintf("queue: %d pending", pending), nil

	case WaitConditionCooldownOver:
		agent, err := findAgent(ctx, agentRepo, waitAgent)
		if err != nil {
			return false, "", fmt.Errorf("agent not found: %w", err)
		}
		if agent.AccountID == "" {
			return true, "no account", nil
		}
		account, err := accountRepo.Get(ctx, agent.AccountID)
		if err != nil {
			return false, "", fmt.Errorf("failed to get account: %w", err)
		}
		if account.CooldownUntil == nil || account.CooldownUntil.Before(time.Now()) {
			return true, "no cooldown", nil
		}
		remaining := time.Until(*account.CooldownUntil).Round(time.Second)
		return false, fmt.Sprintf("cooldown: %s remaining", remaining), nil

	case WaitConditionReady:
		// Agent must be idle, no cooldown, and queue empty
		agent, err := findAgent(ctx, agentRepo, waitAgent)
		if err != nil {
			return false, "", fmt.Errorf("agent not found: %w", err)
		}

		// Check idle
		if agent.State != models.AgentStateIdle {
			return false, fmt.Sprintf("state: %s", agent.State), nil
		}

		// Check queue
		items, err := queueRepo.List(ctx, agent.ID)
		if err != nil {
			return false, "", fmt.Errorf("failed to check queue: %w", err)
		}
		pending := 0
		for _, item := range items {
			if item.Status == models.QueueItemStatusPending {
				pending++
			}
		}
		if pending > 0 {
			return false, fmt.Sprintf("queue: %d pending", pending), nil
		}

		// Check cooldown
		if agent.AccountID != "" {
			account, err := accountRepo.Get(ctx, agent.AccountID)
			if err == nil && account.CooldownUntil != nil && account.CooldownUntil.After(time.Now()) {
				remaining := time.Until(*account.CooldownUntil).Round(time.Second)
				return false, fmt.Sprintf("cooldown: %s remaining", remaining), nil
			}
		}

		return true, "ready", nil

	case WaitConditionAllIdle:
		ws, err := findWorkspace(ctx, wsRepo, waitWorkspace)
		if err != nil {
			return false, "", fmt.Errorf("workspace not found: %w", err)
		}
		agents, err := agentRepo.ListByWorkspace(ctx, ws.ID)
		if err != nil {
			return false, "", fmt.Errorf("failed to list agents: %w", err)
		}
		if len(agents) == 0 {
			return true, "no agents", nil
		}
		notIdle := 0
		for _, agent := range agents {
			if agent.State != models.AgentStateIdle {
				notIdle++
			}
		}
		if notIdle == 0 {
			return true, "all idle", nil
		}
		return false, fmt.Sprintf("%d/%d agents not idle", notIdle, len(agents)), nil

	case WaitConditionAnyIdle:
		ws, err := findWorkspace(ctx, wsRepo, waitWorkspace)
		if err != nil {
			return false, "", fmt.Errorf("workspace not found: %w", err)
		}
		agents, err := agentRepo.ListByWorkspace(ctx, ws.ID)
		if err != nil {
			return false, "", fmt.Errorf("failed to list agents: %w", err)
		}
		if len(agents) == 0 {
			return true, "no agents", nil
		}
		for _, agent := range agents {
			if agent.State == models.AgentStateIdle {
				return true, fmt.Sprintf("agent %s is idle", shortID(agent.ID)), nil
			}
		}
		return false, fmt.Sprintf("0/%d agents idle", len(agents)), nil

	default:
		return false, "", fmt.Errorf("unknown condition: %s", condition)
	}
}
