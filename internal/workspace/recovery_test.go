package workspace

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/opencode-ai/swarm/internal/db"
	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/node"
	"github.com/opencode-ai/swarm/internal/tmux"
)

type recoveryExecutor struct {
	panePath string
	commands []string
}

func (e *recoveryExecutor) Exec(ctx context.Context, cmd string) ([]byte, []byte, error) {
	e.commands = append(e.commands, cmd)

	switch {
	case strings.HasPrefix(cmd, "tmux list-sessions"):
		return []byte("swarm-demo-1234|1\nmisc|1\n"), nil, nil
	case strings.Contains(cmd, "tmux list-panes -t swarm-demo-1234"):
		return []byte(fmt.Sprintf("%%1|%s\n", e.panePath)), nil, nil
	default:
		return []byte(""), nil, nil
	}
}

func setupWorkspaceTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	ctx := context.Background()
	statements := []string{
		"PRAGMA foreign_keys = ON;",
		`CREATE TABLE nodes (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			ssh_target TEXT,
			ssh_backend TEXT NOT NULL DEFAULT 'auto' CHECK (ssh_backend IN ('native', 'system', 'auto')),
			ssh_key_path TEXT,
			ssh_agent_forwarding INTEGER NOT NULL DEFAULT 0,
			ssh_proxy_jump TEXT,
			ssh_control_master TEXT,
			ssh_control_path TEXT,
			ssh_control_persist TEXT,
			ssh_timeout_seconds INTEGER,
			status TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN ('online', 'offline', 'unknown')),
			is_local INTEGER NOT NULL DEFAULT 0,
			last_seen_at TEXT,
			metadata_json TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);`,
		`CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			repo_path TEXT NOT NULL,
			tmux_session TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'error')),
			git_info_json TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE(node_id, repo_path),
			UNIQUE(node_id, tmux_session)
		);`,
	}
	for _, stmt := range statements {
		if _, err := database.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("failed to set up schema: %v", err)
		}
	}
	return database
}

func TestRecoverOrphanedSessions(t *testing.T) {
	ctx := context.Background()
	database := setupWorkspaceTestDB(t)
	defer database.Close()

	nodeRepo := db.NewNodeRepository(database)
	localNode := &models.Node{
		Name:       "local",
		IsLocal:    true,
		Status:     models.NodeStatusUnknown,
		SSHBackend: models.SSHBackendAuto,
	}
	if err := nodeRepo.Create(ctx, localNode); err != nil {
		t.Fatalf("failed to create local node: %v", err)
	}

	nodeService := node.NewService(nodeRepo)
	wsRepo := db.NewWorkspaceRepository(database)

	paneDir := t.TempDir()
	exec := &recoveryExecutor{panePath: paneDir}
	tmuxFactory := func() *tmux.Client {
		return tmux.NewClient(exec)
	}

	service := NewService(wsRepo, nodeService, nil, WithTmuxClientFactory(tmuxFactory))
	report, err := service.RecoverOrphanedSessions(ctx, localNode.ID, "swarm")
	if err != nil {
		t.Fatalf("RecoverOrphanedSessions failed: %v", err)
	}
	if report == nil || len(report.Imported) != 1 {
		t.Fatalf("expected 1 imported session, got %+v", report)
	}

	if _, err := wsRepo.GetByTmuxSession(ctx, localNode.ID, "swarm-demo-1234"); err != nil {
		t.Fatalf("expected workspace for swarm-demo-1234, got error: %v", err)
	}
	if _, err := wsRepo.GetByTmuxSession(ctx, localNode.ID, "misc"); err == nil {
		t.Fatalf("expected misc session to remain unimported")
	}
}
