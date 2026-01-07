package db

import (
	"context"
	"testing"

	"github.com/tOgg1/forge/internal/models"
)

func TestLoopRunRepository_CreateFinish(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	loop := createTestLoop(t, db)
	profileRepo := NewProfileRepository(db)
	ctx := context.Background()

	profile := &models.Profile{
		Name:            "pi-runner",
		Harness:         models.HarnessPi,
		CommandTemplate: "pi -p \"{prompt}\"",
		MaxConcurrency:  1,
		PromptMode:      models.PromptModePath,
	}
	if err := profileRepo.Create(ctx, profile); err != nil {
		t.Fatalf("Create profile failed: %v", err)
	}

	repo := NewLoopRunRepository(db)
	run := &models.LoopRun{
		LoopID:       loop.ID,
		ProfileID:    profile.ID,
		PromptSource: "base",
		Status:       models.LoopRunStatusRunning,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create run failed: %v", err)
	}

	exitCode := 0
	run.Status = models.LoopRunStatusSuccess
	run.ExitCode = &exitCode
	run.OutputTail = "ok"

	if err := repo.Finish(ctx, run); err != nil {
		t.Fatalf("Finish failed: %v", err)
	}

	stored, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if stored.Status != models.LoopRunStatusSuccess {
		t.Fatalf("expected status success, got %q", stored.Status)
	}
	if stored.ExitCode == nil || *stored.ExitCode != 0 {
		t.Fatalf("expected exit code 0")
	}
}
