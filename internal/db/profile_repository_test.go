package db

import (
	"context"
	"testing"
	"time"

	"github.com/tOgg1/forge/internal/models"
)

func TestProfileRepository_CreateGetUpdate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewProfileRepository(db)
	ctx := context.Background()

	profile := &models.Profile{
		Name:            "pi-test",
		Harness:         models.HarnessPi,
		CommandTemplate: "pi -p \"{prompt}\"",
		MaxConcurrency:  2,
		PromptMode:      models.PromptModePath,
	}

	if err := repo.Create(ctx, profile); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	fetched, err := repo.Get(ctx, profile.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if fetched.Name != profile.Name {
		t.Fatalf("expected name %q, got %q", profile.Name, fetched.Name)
	}
	if fetched.Harness != profile.Harness {
		t.Fatalf("expected harness %q, got %q", profile.Harness, fetched.Harness)
	}

	cooldown := time.Now().UTC().Add(5 * time.Minute)
	fetched.Model = "claude-opus"
	fetched.CooldownUntil = &cooldown

	if err := repo.Update(ctx, fetched); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, err := repo.GetByName(ctx, profile.Name)
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if updated.Model != "claude-opus" {
		t.Fatalf("expected model to update")
	}
	if updated.CooldownUntil == nil {
		t.Fatalf("expected cooldown to be set")
	}
}
