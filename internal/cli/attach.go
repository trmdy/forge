// Package cli provides the attach command for quick tmux access.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/node"
	"github.com/tOgg1/forge/internal/workspace"
)

var (
	attachSelect bool
)

func init() {
	addLegacyCommand(attachCmd)

	attachCmd.Flags().BoolVar(&attachSelect, "select", false, "interactive selection")
}

var attachCmd = &cobra.Command{
	Use:   "attach [target]",
	Short: "Attach to agent pane or workspace session",
	Long: `Attach to a tmux session or pane.

If a target is specified, it can be:
- An agent ID/prefix: Focus that agent's tmux pane
- A workspace name/ID: Attach to the workspace's tmux session

If no target is specified:
- Uses current context (agent or workspace)
- Falls back to interactive selection with --select`,
	Example: `  # Attach using context
  forge attach

  # Attach to specific agent's pane
  forge attach abc123

  # Attach to workspace session
  forge attach my-project

  # Interactive selection
  forge attach --select`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		nodeRepo := db.NewNodeRepository(database)
		nodeService := node.NewService(nodeRepo, node.WithPublisher(newEventPublisher(database)))
		wsRepo := db.NewWorkspaceRepository(database)
		agentRepo := db.NewAgentRepository(database)
		wsService := workspace.NewService(wsRepo, nodeService, agentRepo, workspace.WithPublisher(newEventPublisher(database)))

		var target string
		if len(args) > 0 {
			target = args[0]
		}

		// If no target and not selecting, try context
		if target == "" && !attachSelect {
			// Try agent context first
			agentCtx, err := ResolveAgentContext(ctx, agentRepo, "", "")
			if err == nil && agentCtx != nil {
				target = agentCtx.AgentID
			} else {
				// Try workspace context
				wsCtx, err := ResolveWorkspaceContext(ctx, wsRepo, "")
				if err == nil && wsCtx != nil {
					target = wsCtx.WorkspaceID
				}
			}
		}

		// Interactive selection
		if target == "" || attachSelect {
			return interactiveAttach(ctx, wsRepo, agentRepo, wsService)
		}

		// Try to resolve as agent first
		agent, err := findAgent(ctx, agentRepo, target)
		if err == nil {
			return attachToAgent(ctx, wsService, wsRepo, agent.WorkspaceID, agent.TmuxPane)
		}

		// Try to resolve as workspace
		ws, err := findWorkspace(ctx, wsRepo, target)
		if err == nil {
			return attachToWorkspace(ctx, wsService, ws.ID)
		}

		return fmt.Errorf("'%s' not found as agent or workspace", target)
	},
}

// attachToAgent focuses on a specific agent's pane.
func attachToAgent(ctx context.Context, wsService *workspace.Service, wsRepo *db.WorkspaceRepository, workspaceID, paneID string) error {
	if paneID == "" {
		return fmt.Errorf("agent has no tmux pane")
	}

	// Get the workspace to find the session
	ws, err := wsRepo.Get(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	if ws.TmuxSession == "" {
		return fmt.Errorf("workspace has no tmux session")
	}

	// Build attach command that focuses on the specific pane
	attachCmd := fmt.Sprintf("tmux select-pane -t %s && tmux attach-session -t %s",
		paneID, ws.TmuxSession)

	if IsJSONOutput() || IsJSONLOutput() {
		return WriteOutput(os.Stdout, map[string]string{
			"workspace_id": workspaceID,
			"pane_id":      paneID,
			"session":      ws.TmuxSession,
			"command":      attachCmd,
		})
	}

	// Execute tmux attach
	tmuxCmd := exec.CommandContext(ctx, "sh", "-c", attachCmd)
	tmuxCmd.Stdin = os.Stdin
	tmuxCmd.Stdout = os.Stdout
	tmuxCmd.Stderr = os.Stderr

	return tmuxCmd.Run()
}

// attachToWorkspace attaches to a workspace's tmux session.
func attachToWorkspace(ctx context.Context, wsService *workspace.Service, workspaceID string) error {
	attachCmd, err := wsService.AttachWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get attach command: %w", err)
	}

	if IsJSONOutput() || IsJSONLOutput() {
		return WriteOutput(os.Stdout, map[string]string{
			"workspace_id": workspaceID,
			"command":      attachCmd,
		})
	}

	// Execute tmux attach
	tmuxCmd := exec.CommandContext(ctx, "sh", "-c", attachCmd)
	tmuxCmd.Stdin = os.Stdin
	tmuxCmd.Stdout = os.Stdout
	tmuxCmd.Stderr = os.Stderr

	return tmuxCmd.Run()
}

// interactiveAttach prompts for selection when no target is specified.
func interactiveAttach(ctx context.Context, wsRepo *db.WorkspaceRepository, agentRepo *db.AgentRepository, wsService *workspace.Service) error {
	// List available workspaces and agents
	workspaces, err := wsRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list workspaces: %w", err)
	}

	if len(workspaces) == 0 {
		return fmt.Errorf("no workspaces available")
	}

	// For now, just list options - full interactive selection would need a TUI library
	fmt.Println("Available targets:")
	fmt.Println()
	fmt.Println("Workspaces:")
	for _, ws := range workspaces {
		fmt.Printf("  forge attach %s  # %s\n", shortID(ws.ID), ws.Name)
	}

	// List agents
	fmt.Println()
	fmt.Println("Agents (by workspace):")
	for _, ws := range workspaces {
		agents, err := agentRepo.ListByWorkspace(ctx, ws.ID)
		if err != nil {
			continue
		}
		if len(agents) > 0 {
			fmt.Printf("  [%s]\n", ws.Name)
			for _, a := range agents {
				stateStr := formatAgentState(a.State)
				fmt.Printf("    forge attach %s  # %s %s\n", shortID(a.ID), a.Type, stateStr)
			}
		}
	}

	fmt.Println()
	fmt.Println("Run one of the above commands to attach.")

	return nil
}
