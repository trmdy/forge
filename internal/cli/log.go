// Package cli provides the log command for tailing agent transcripts.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/opencode-ai/swarm/internal/db"
	"github.com/opencode-ai/swarm/internal/node"
	"github.com/opencode-ai/swarm/internal/tmux"
	"github.com/opencode-ai/swarm/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	logFollow   bool
	logLast     int
	logSince    string
	logNoFollow bool
	logRaw      bool
)

func init() {
	rootCmd.AddCommand(logCmd)

	logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "follow log output (continuous)")
	logCmd.Flags().IntVarP(&logLast, "last", "n", 0, "show last N lines (0 = all visible)")
	logCmd.Flags().StringVar(&logSince, "since", "", "show logs since duration (e.g., 1h, 30m)")
	logCmd.Flags().BoolVar(&logNoFollow, "no-follow", false, "don't follow, just show current content")
	logCmd.Flags().BoolVar(&logRaw, "raw", false, "show raw output without formatting")
}

var logCmd = &cobra.Command{
	Use:   "log [agent]",
	Short: "Tail agent transcript/output",
	Long: `View an agent's terminal output (transcript).

By default, shows the current visible content. Use --follow to
continuously stream new output, or --last N to show the last N lines.

Sources:
- Tmux pane capture (primary)
- Event log history (when available)`,
	Example: `  # Show current output for context agent
  swarm log

  # Show last 50 lines
  swarm log abc123 --last 50

  # Follow output continuously
  swarm log abc123 --follow

  # Show output from last hour
  swarm log abc123 --since 1h

  # Raw output (no formatting)
  swarm log abc123 --raw`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		agentRepo := db.NewAgentRepository(database)
		nodeRepo := db.NewNodeRepository(database)
		nodeService := node.NewService(nodeRepo)
		wsRepo := db.NewWorkspaceRepository(database)
		wsService := workspace.NewService(wsRepo, nodeService, agentRepo)
		_ = wsService

		// Resolve agent
		var agentID string
		if len(args) > 0 {
			agent, err := findAgent(ctx, agentRepo, args[0])
			if err != nil {
				return err
			}
			agentID = agent.ID
		} else {
			// Try context
			agentCtx, err := RequireAgentContext(ctx, agentRepo, "", "")
			if err != nil {
				return fmt.Errorf("no agent specified and no context set")
			}
			agentID = agentCtx.AgentID
		}

		agent, err := agentRepo.Get(ctx, agentID)
		if err != nil {
			return fmt.Errorf("agent not found: %w", err)
		}

		if agent.TmuxPane == "" {
			return fmt.Errorf("agent %s has no tmux pane", shortID(agent.ID))
		}

		tmuxClient := tmux.NewLocalClient()

		// Determine mode
		if logFollow {
			return followLog(ctx, tmuxClient, agent.TmuxPane, agent.ID)
		}

		// Single capture
		includeHistory := logLast > 0 || logSince != ""
		content, err := tmuxClient.CapturePane(ctx, agent.TmuxPane, includeHistory)
		if err != nil {
			return fmt.Errorf("failed to capture pane: %w", err)
		}

		// Apply filters
		lines := strings.Split(content, "\n")

		if logLast > 0 && len(lines) > logLast {
			lines = lines[len(lines)-logLast:]
		}

		// Output
		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{
				"agent_id":    agent.ID,
				"tmux_pane":   agent.TmuxPane,
				"lines":       len(lines),
				"content":     strings.Join(lines, "\n"),
				"captured_at": time.Now().Format(time.RFC3339),
			})
		}

		if logRaw {
			fmt.Println(strings.Join(lines, "\n"))
		} else {
			// Add header
			fmt.Printf("=== Agent %s (%s) ===\n", shortID(agent.ID), agent.TmuxPane)
			fmt.Printf("=== Captured at %s ===\n\n", time.Now().Format("15:04:05"))
			fmt.Println(strings.Join(lines, "\n"))
		}

		return nil
	},
}

// followLog continuously captures and displays new content from the pane.
func followLog(ctx context.Context, tmuxClient *tmux.Client, paneID, agentID string) error {
	if !IsJSONOutput() && !IsJSONLOutput() {
		fmt.Printf("=== Following agent %s (%s) - Ctrl+C to stop ===\n\n", shortID(agentID), paneID)
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastContent string
	var lastHash string

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			content, err := tmuxClient.CapturePane(ctx, paneID, false)
			if err != nil {
				// Pane might be gone
				if strings.Contains(err.Error(), "can't find pane") {
					fmt.Println("\n=== Pane closed ===")
					return nil
				}
				continue
			}

			// Simple change detection using content comparison
			// (could use hash for efficiency, but content comparison is fine for now)
			currentHash := fmt.Sprintf("%d", len(content))
			if content != lastContent || currentHash != lastHash {
				// Find new lines
				if lastContent == "" {
					// First capture - show all
					if logRaw {
						fmt.Print(content)
					} else {
						fmt.Print(content)
					}
				} else {
					// Find delta - show only new content
					// Simple approach: if content changed, show the difference
					if len(content) > len(lastContent) && strings.HasPrefix(content, lastContent) {
						// Content was appended
						newContent := content[len(lastContent):]
						fmt.Print(newContent)
					} else if content != lastContent {
						// Content changed significantly - show all
						// (this happens when terminal scrolls)
						if !logRaw {
							fmt.Print("\033[2J\033[H") // Clear screen
							fmt.Printf("=== Refreshed at %s ===\n\n", time.Now().Format("15:04:05"))
						}
						fmt.Print(content)
					}
				}

				lastContent = content
				lastHash = currentHash
			}
		}
	}
}
