// Package cli provides preflight checks for CLI commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/node"
	"github.com/tOgg1/forge/internal/workspace"
)

// PreflightError carries a message and suggested next steps.
type PreflightError struct {
	Message  string
	Hint     string
	NextStep string
	Err      error
}

// Error implements error.
func (e *PreflightError) Error() string {
	if e == nil {
		return ""
	}

	parts := []string{e.Message}
	if e.Err != nil {
		parts[0] = fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	if e.Hint != "" {
		parts = append(parts, "Hint: "+e.Hint)
	}
	if e.NextStep != "" {
		parts = append(parts, "Next: "+e.NextStep)
	}
	return strings.Join(parts, "\n")
}

func runPreflight(cmd *cobra.Command) error {
	if !shouldRunPreflight(cmd) {
		return nil
	}

	ctx := context.Background()

	maybeWarnMissingConfig()

	if err := checkTmux(cmd); err != nil {
		return err
	}
	if err := checkSSH(cmd); err != nil {
		return err
	}
	if err := checkDatabase(ctx); err != nil {
		return err
	}
	maybeAutoRegisterLocalNode(ctx)
	maybeAutoImportWorkspaces(ctx)
	if err := checkWorkspacePath(cmd); err != nil {
		return err
	}

	return nil
}

func maybeAutoImportWorkspaces(ctx context.Context) {
	if appConfig == nil || !appConfig.WorkspaceDefaults.AutoImportExisting {
		return
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return
	}

	database, err := openDatabase()
	if err != nil {
		logger.Warn().Err(err).Msg("failed to open database for auto-import")
		return
	}
	defer database.Close()

	nodeRepo := db.NewNodeRepository(database)
	nodeService := node.NewService(nodeRepo, node.WithPublisher(newEventPublisher(database)))
	wsRepo := db.NewWorkspaceRepository(database)
	agentRepo := db.NewAgentRepository(database)
	wsService := workspace.NewService(wsRepo, nodeService, agentRepo, workspace.WithPublisher(newEventPublisher(database)))

	report, err := wsService.RecoverOrphanedSessions(ctx, "", appConfig.WorkspaceDefaults.TmuxPrefix)
	if err != nil {
		if errors.Is(err, workspace.ErrNodeNotFound) {
			return
		}
		logger.Warn().Err(err).Msg("failed to auto-import existing tmux sessions")
		return
	}
	if report == nil {
		return
	}

	if imported := len(report.Imported); imported > 0 {
		logger.Info().Int("count", imported).Msg("imported existing tmux sessions")
	}
	if failed := len(report.Failures); failed > 0 {
		logger.Warn().Int("count", failed).Msg("some tmux sessions failed to import")
	}
}

func maybeAutoRegisterLocalNode(ctx context.Context) {
	if appConfig == nil || !appConfig.Global.AutoRegisterLocalNode {
		return
	}

	database, err := openDatabase()
	if err != nil {
		return
	}
	defer database.Close()

	nodeRepo := db.NewNodeRepository(database)
	nodeService := node.NewService(nodeRepo, node.WithPublisher(newEventPublisher(database)))

	// Check if a local node already exists
	nodes, err := nodeService.ListNodes(ctx, nil)
	if err != nil {
		return
	}
	for _, n := range nodes {
		if n.IsLocal {
			return // Already have a local node
		}
	}

	// Determine hostname for node name
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "local"
	}

	// Create local node
	localNode := &models.Node{
		Name:    hostname,
		IsLocal: true,
		Status:  models.NodeStatusOnline,
	}

	if err := nodeService.AddNode(ctx, localNode, false); err != nil {
		if !errors.Is(err, node.ErrNodeAlreadyExists) {
			logger.Warn().Err(err).Msg("failed to auto-register local node")
		}
		return
	}

	logger.Info().Str("name", hostname).Msg("auto-registered local node")
}

func shouldRunPreflight(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	if cmd.Name() == "forge" {
		if flag := cmd.Flag("version"); flag != nil && flag.Changed {
			return false
		}
	}

	path := cmd.CommandPath()
	switch {
	case strings.HasPrefix(path, "forge init"):
		return false
	case strings.HasPrefix(path, "forge migrate"):
		return false
	case strings.HasPrefix(path, "forge completion"):
		return false
	case strings.HasPrefix(path, "forge help"):
		return false
	case strings.HasPrefix(path, "forge vault"):
		return false
	}

	return true
}

func maybeWarnMissingConfig() {
	if IsJSONOutput() || IsJSONLOutput() {
		return
	}
	if configLoader == nil {
		return
	}
	if cfgFile != "" {
		return
	}
	if configLoader.ConfigFileUsed() != "" {
		return
	}

	fmt.Fprintln(os.Stderr, "Warning: no config file found.")
	fmt.Fprintln(os.Stderr, "Hint: create ~/.config/forge/config.yaml (see docs/config.example.yaml).")
}

func checkTmux(cmd *cobra.Command) error {
	if !requiresTmux(cmd) {
		return nil
	}
	if _, err := exec.LookPath("tmux"); err == nil {
		return nil
	}

	return &PreflightError{
		Message:  "tmux is required for this command",
		Hint:     "Install tmux and ensure it is in PATH",
		NextStep: "Install tmux",
	}
}

func requiresTmux(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if cmd.Parent() == nil && cmd.Name() == "forge" {
		return true
	}

	parent := cmd.Parent()
	if parent == nil {
		return false
	}

	switch parent.Name() {
	case "ws":
		switch cmd.Name() {
		case "list", "refresh":
			return false
		case "create":
			return !wsCreateNoTmux
		case "remove":
			return wsRemoveDestroy
		default:
			return true
		}
	case "agent":
		switch cmd.Name() {
		case "list", "queue", "pause", "resume":
			return false
		default:
			return true
		}
	}

	return false
}

func checkSSH(cmd *cobra.Command) error {
	if appConfig == nil {
		return nil
	}
	if appConfig.NodeDefaults.SSHBackend != models.SSHBackendSystem {
		return nil
	}
	if cmd == nil || cmd.Parent() == nil || cmd.Parent().Name() != "node" {
		return nil
	}
	if _, err := exec.LookPath("ssh"); err == nil {
		return nil
	}

	return &PreflightError{
		Message:  "ssh binary not found for system backend",
		Hint:     "Install OpenSSH client or switch to native SSH backend",
		NextStep: "Install OpenSSH client",
	}
}

func checkDatabase(ctx context.Context) error {
	db, err := openDatabase()
	if err != nil {
		return &PreflightError{
			Message:  "database unavailable",
			Hint:     "Check database path and permissions",
			NextStep: "Check database path",
			Err:      err,
		}
	}
	defer db.Close()

	version, err := db.SchemaVersion(ctx)
	if err != nil {
		if isMissingSchemaTable(err) {
			return &PreflightError{
				Message:  "database not migrated",
				Hint:     "Run `forge migrate up` to initialize the database",
				NextStep: "forge migrate up",
			}
		}
		return &PreflightError{
			Message:  "failed to read database schema version",
			Hint:     "Ensure the database is reachable and not locked",
			NextStep: "Check database connectivity",
			Err:      err,
		}
	}
	if version == 0 {
		return &PreflightError{
			Message:  "database has no migrations applied",
			Hint:     "Run `forge migrate up` to initialize the database",
			NextStep: "forge migrate up",
		}
	}

	return nil
}

func isMissingSchemaTable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") && strings.Contains(msg, "schema_version")
}

func checkWorkspacePath(cmd *cobra.Command) error {
	if cmd == nil || cmd.Parent() == nil || cmd.Parent().Name() != "ws" {
		return nil
	}
	if cmd.Name() != "create" {
		return nil
	}
	if wsCreatePath == "" {
		return nil
	}

	path := wsCreatePath
	if path == "." {
		if cwd, err := os.Getwd(); err == nil {
			path = cwd
		}
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	return &PreflightError{
		Message:  "workspace path not found",
		Hint:     fmt.Sprintf("Check that %s exists and is readable", filepath.Clean(path)),
		NextStep: "forge ws create --path <repo>",
	}
}
