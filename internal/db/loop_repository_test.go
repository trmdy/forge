package db

import (
	"context"
	"testing"

	"github.com/tOgg1/forge/internal/models"
)

func TestLoopRepository_CreateGetUpdate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewLoopRepository(db)
	ctx := context.Background()

	loop := &models.Loop{
		Name:            "Smart Homer",
		RepoPath:        "/repo",
		IntervalSeconds: 15,
		State:           models.LoopStateRunning,
		LogPath:         "/tmp/loop.log",
		LedgerPath:      "/repo/.forge/ledgers/Smart-Homer.md",
		Tags:            []string{"review"},
	}

	if err := repo.Create(ctx, loop); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	fetched, err := repo.GetByName(ctx, loop.Name)
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if fetched.RepoPath != loop.RepoPath {
		t.Fatalf("expected repo path %q, got %q", loop.RepoPath, fetched.RepoPath)
	}
	if fetched.ShortID == "" {
		t.Fatalf("expected short ID to be set")
	}
	if loop.ShortID == "" {
		t.Fatalf("expected loop short ID to be set on create")
	}

	fetched.State = models.LoopStateSleeping
	if err := repo.Update(ctx, fetched); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, err := repo.Get(ctx, fetched.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if updated.State != models.LoopStateSleeping {
		t.Fatalf("expected state to update")
	}
}
