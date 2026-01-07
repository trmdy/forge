package db

import (
	"context"
	"testing"

	"github.com/tOgg1/forge/internal/models"
)

func TestPoolRepository_CreateDefaultMembers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	poolRepo := NewPoolRepository(db)
	profileRepo := NewProfileRepository(db)
	ctx := context.Background()

	profile := &models.Profile{
		Name:            "pi-default",
		Harness:         models.HarnessPi,
		CommandTemplate: "pi -p \"{prompt}\"",
		MaxConcurrency:  1,
		PromptMode:      models.PromptModePath,
	}
	if err := profileRepo.Create(ctx, profile); err != nil {
		t.Fatalf("Create profile failed: %v", err)
	}

	pool := &models.Pool{
		Name:     "default",
		Strategy: models.PoolStrategyRoundRobin,
	}
	if err := poolRepo.Create(ctx, pool); err != nil {
		t.Fatalf("Create pool failed: %v", err)
	}

	member := &models.PoolMember{
		PoolID:    pool.ID,
		ProfileID: profile.ID,
		Weight:    2,
	}
	if err := poolRepo.AddMember(ctx, member); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	members, err := poolRepo.ListMembers(ctx, pool.ID)
	if err != nil {
		t.Fatalf("ListMembers failed: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}

	if err := poolRepo.SetDefault(ctx, pool.ID); err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}

	updated, err := poolRepo.Get(ctx, pool.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !updated.IsDefault {
		t.Fatalf("expected pool to be default")
	}
}
