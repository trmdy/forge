// Package cli provides the context command for setting workspace/agent context.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/config"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

var (
	useAgent     string
	useWorkspace string
	useClear     bool
	useShow      bool
)

func init() {
	rootCmd.AddCommand(useCmd)
	rootCmd.AddCommand(contextCmd)

	useCmd.Flags().StringVar(&useAgent, "agent", "", "set agent context (within current workspace)")
	useCmd.Flags().StringVar(&useWorkspace, "workspace", "", "set workspace context")
	useCmd.Flags().BoolVar(&useClear, "clear", false, "clear all context")
	useCmd.Flags().BoolVar(&useShow, "show", false, "show current context")
}

var useCmd = &cobra.Command{
	Use:   "use [workspace|agent]",
	Short: "Set the current workspace or agent context",
	Long: `Set the current context to avoid repeating --workspace and --agent flags.

Context is persisted to ~/.config/forge/context.yaml and used by other commands
when explicit flags are not provided.

Examples:
  forge use my-project           # Set workspace context by name
  forge use ws_abc123            # Set workspace context by ID
  forge use --agent agent_xyz    # Set agent context (keeps workspace)
  forge use --clear              # Clear all context
  forge use --show               # Show current context
  forge use                      # Show current context (same as --show)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store := config.DefaultContextStore()

		// Handle --clear
		if useClear {
			if err := store.Clear(); err != nil {
				return fmt.Errorf("failed to clear context: %w", err)
			}
			fmt.Println("Context cleared.")
			return nil
		}

		// Handle --show or no args
		if useShow || (len(args) == 0 && useAgent == "" && useWorkspace == "") {
			ctx, err := store.Load()
			if err != nil {
				return fmt.Errorf("failed to load context: %w", err)
			}
			if ctx.IsEmpty() {
				fmt.Println("No context set.")
				fmt.Println("")
				fmt.Println("Set context with:")
				fmt.Println("  forge use <workspace>       # Set workspace")
				fmt.Println("  forge use --agent <agent>   # Set agent")
			} else {
				fmt.Printf("Current context: %s\n", ctx.String())
				if ctx.HasWorkspace() {
					fmt.Printf("  Workspace: %s", ctx.WorkspaceID)
					if ctx.WorkspaceName != "" {
						fmt.Printf(" (%s)", ctx.WorkspaceName)
					}
					fmt.Println()
				}
				if ctx.HasAgent() {
					fmt.Printf("  Agent: %s", ctx.AgentID)
					if ctx.AgentName != "" {
						fmt.Printf(" (%s)", ctx.AgentName)
					}
					fmt.Println()
				}
			}
			return nil
		}

		// Load existing context
		ctx, err := store.Load()
		if err != nil {
			return fmt.Errorf("failed to load context: %w", err)
		}

		// Open database for lookups
		database, err := openDatabase()
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer database.Close()

		bgCtx := context.Background()

		// Handle --workspace flag
		if useWorkspace != "" {
			ws, err := resolveWorkspace(bgCtx, database, useWorkspace)
			if err != nil {
				return fmt.Errorf("failed to resolve workspace: %w", err)
			}
			ctx.SetWorkspace(ws.ID, ws.Name)
			fmt.Printf("Workspace set to: %s (%s)\n", ws.Name, shortContextID(ws.ID))
		}

		// Handle --agent flag
		if useAgent != "" {
			agent, err := resolveAgent(bgCtx, database, useAgent, ctx.WorkspaceID)
			if err != nil {
				return fmt.Errorf("failed to resolve agent: %w", err)
			}
			// If agent found, also set its workspace
			if ctx.WorkspaceID == "" || ctx.WorkspaceID != agent.WorkspaceID {
				wsRepo := db.NewWorkspaceRepository(database)
				ws, err := wsRepo.Get(bgCtx, agent.WorkspaceID)
				if err == nil {
					ctx.SetWorkspace(ws.ID, ws.Name)
				}
			}
			ctx.SetAgent(agent.ID, shortContextID(agent.ID))
			fmt.Printf("Agent set to: %s\n", shortContextID(agent.ID))
		}

		// Handle positional argument (workspace or workspace:agent)
		if len(args) > 0 {
			target := args[0]

			// Check for workspace:agent format
			if strings.Contains(target, ":") {
				parts := strings.SplitN(target, ":", 2)
				wsTarget := parts[0]
				agentTarget := parts[1]

				// Resolve workspace first
				ws, err := resolveWorkspace(bgCtx, database, wsTarget)
				if err != nil {
					return fmt.Errorf("failed to resolve workspace '%s': %w", wsTarget, err)
				}
				ctx.SetWorkspace(ws.ID, ws.Name)

				// Then resolve agent within that workspace
				agent, err := resolveAgent(bgCtx, database, agentTarget, ws.ID)
				if err != nil {
					return fmt.Errorf("failed to resolve agent '%s': %w", agentTarget, err)
				}
				ctx.SetAgent(agent.ID, shortContextID(agent.ID))

				fmt.Printf("Context set to: %s:%s\n", ws.Name, shortContextID(agent.ID))
			} else {
				// Try workspace first, then agent
				ws, wsErr := resolveWorkspace(bgCtx, database, target)
				if wsErr == nil {
					ctx.SetWorkspace(ws.ID, ws.Name)
					fmt.Printf("Workspace set to: %s (%s)\n", ws.Name, shortContextID(ws.ID))
				} else {
					// Try as agent
					agent, agentErr := resolveAgent(bgCtx, database, target, ctx.WorkspaceID)
					if agentErr != nil {
						return fmt.Errorf("'%s' is not a valid workspace or agent", target)
					}
					// Set workspace from agent
					if agent.WorkspaceID != "" {
						wsRepo := db.NewWorkspaceRepository(database)
						ws, err := wsRepo.Get(bgCtx, agent.WorkspaceID)
						if err == nil {
							ctx.SetWorkspace(ws.ID, ws.Name)
						}
					}
					ctx.SetAgent(agent.ID, shortContextID(agent.ID))
					fmt.Printf("Agent set to: %s\n", shortContextID(agent.ID))
				}
			}
		}

		// Save context
		if err := store.Save(ctx); err != nil {
			return fmt.Errorf("failed to save context: %w", err)
		}

		return nil
	},
}

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Show current context",
	Long:  `Show the current workspace and agent context. Alias for 'forge use --show'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store := config.DefaultContextStore()
		ctx, err := store.Load()
		if err != nil {
			return fmt.Errorf("failed to load context: %w", err)
		}

		if IsJSONOutput() {
			return WriteOutput(os.Stdout, ctx)
		}

		if ctx.IsEmpty() {
			fmt.Println("No context set.")
			return nil
		}

		fmt.Printf("Context: %s\n", ctx.String())
		return nil
	},
}

// resolveWorkspace finds a workspace by ID or name.
func resolveWorkspace(ctx context.Context, database *db.DB, target string) (*models.Workspace, error) {
	repo := db.NewWorkspaceRepository(database)

	// Try by ID first
	ws, err := repo.Get(ctx, target)
	if err == nil {
		return ws, nil
	}

	// Try by name
	workspaces, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}

	for _, w := range workspaces {
		if w.Name == target {
			return w, nil
		}
		// Also match by prefix of ID
		if strings.HasPrefix(w.ID, target) {
			return w, nil
		}
	}

	return nil, fmt.Errorf("workspace not found: %s", target)
}

// resolveAgent finds an agent by ID (or prefix), optionally within a workspace.
func resolveAgent(ctx context.Context, database *db.DB, target, workspaceID string) (*models.Agent, error) {
	repo := db.NewAgentRepository(database)

	// Try by ID first
	agent, err := repo.Get(ctx, target)
	if err == nil {
		// If workspace is set, verify agent belongs to it
		if workspaceID != "" && agent.WorkspaceID != workspaceID {
			return nil, fmt.Errorf("agent %s does not belong to workspace %s", target, workspaceID)
		}
		return agent, nil
	}

	// Try by prefix
	var agents []*models.Agent
	if workspaceID != "" {
		agents, err = repo.ListByWorkspace(ctx, workspaceID)
	} else {
		agents, err = repo.List(ctx)
	}
	if err != nil {
		return nil, err
	}

	for _, a := range agents {
		if strings.HasPrefix(a.ID, target) {
			return a, nil
		}
	}

	return nil, fmt.Errorf("agent not found: %s", target)
}

func shortContextID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
