package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/opencode-ai/swarm/internal/models"
)

func TestFileLockRepository_CleanupExpired(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	ws := createTestWorkspace(t, db)

	agentRepo := NewAgentRepository(db)
	agent := &models.Agent{
		WorkspaceID: ws.ID,
		Type:        models.AgentTypeOpenCode,
		TmuxPane:    "swarm-test:0.1",
		State:       models.AgentStateIdle,
	}
	if err := agentRepo.Create(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	now := time.Now().UTC()
	expired := now.Add(-1 * time.Hour)
	active := now.Add(1 * time.Hour)
	createdAt := now.Add(-2 * time.Hour)

	insert := `
		INSERT INTO file_locks (
			id, workspace_id, agent_id, path_pattern, exclusive, reason,
			ttl_seconds, expires_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	if _, err := db.ExecContext(ctx, insert,
		"lock-expired",
		ws.ID,
		agent.ID,
		"src/*.go",
		1,
		"test",
		3600,
		expired.Format(time.RFC3339),
		createdAt.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("insert expired lock: %v", err)
	}
	if _, err := db.ExecContext(ctx, insert,
		"lock-active",
		ws.ID,
		agent.ID,
		"README.md",
		1,
		"test",
		3600,
		active.Format(time.RFC3339),
		createdAt.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("insert active lock: %v", err)
	}

	repo := NewFileLockRepository(db)
	updated, err := repo.CleanupExpired(ctx, now)
	if err != nil {
		t.Fatalf("CleanupExpired failed: %v", err)
	}
	if updated != 1 {
		t.Errorf("expected 1 lock released, got %d", updated)
	}

	var expiredReleased sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT released_at FROM file_locks WHERE id = ?`, "lock-expired").Scan(&expiredReleased); err != nil {
		t.Fatalf("query expired lock: %v", err)
	}
	if !expiredReleased.Valid {
		t.Fatalf("expected expired lock to be released")
	}

	var activeReleased sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT released_at FROM file_locks WHERE id = ?`, "lock-active").Scan(&activeReleased); err != nil {
		t.Fatalf("query active lock: %v", err)
	}
	if activeReleased.Valid {
		t.Fatalf("expected active lock to remain unreleased")
	}
}
