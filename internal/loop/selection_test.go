package loop

import (
	"context"
	"testing"
	"time"

	"github.com/tOgg1/forge/internal/config"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/testutil"
)

func TestSelectProfilePinnedUnavailable(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	ctx := context.Background()
	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)
	runRepo := db.NewLoopRunRepository(database)
	poolRepo := db.NewPoolRepository(database)

	profile := &models.Profile{
		Name:            "p1",
		Harness:         models.HarnessPi,
		CommandTemplate: "pi -p \"{prompt}\"",
		MaxConcurrency:  1,
	}
	if err := profileRepo.Create(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	loop := &models.Loop{
		Name:            "loop-a",
		RepoPath:        "/tmp/repo",
		IntervalSeconds: 10,
		ProfileID:       profile.ID,
	}
	if err := loopRepo.Create(ctx, loop); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	run := &models.LoopRun{
		LoopID:    loop.ID,
		ProfileID: profile.ID,
		Status:    models.LoopRunStatusRunning,
		StartedAt: time.Now().UTC(),
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	runner := NewRunner(database, config.DefaultConfig())
	selected, waitUntil, err := runner.selectProfile(ctx, loop, profileRepo, poolRepo, runRepo)
	if err == nil {
		t.Fatalf("expected error for unavailable pinned profile")
	}
	if selected != nil || waitUntil != nil {
		t.Fatalf("expected no selection for pinned profile")
	}
}

func TestSelectProfilePoolSkipsCooldown(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	ctx := context.Background()
	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)
	runRepo := db.NewLoopRunRepository(database)
	poolRepo := db.NewPoolRepository(database)

	now := time.Now().UTC()
	cooldownUntil := now.Add(10 * time.Minute)

	profile1 := &models.Profile{
		Name:            "cooldown",
		Harness:         models.HarnessPi,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
		CooldownUntil:   &cooldownUntil,
	}
	profile2 := &models.Profile{
		Name:            "ready",
		Harness:         models.HarnessPi,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
	}
	if err := profileRepo.Create(ctx, profile1); err != nil {
		t.Fatalf("create profile1: %v", err)
	}
	if err := profileRepo.Create(ctx, profile2); err != nil {
		t.Fatalf("create profile2: %v", err)
	}

	pool := &models.Pool{Name: "pool-a", Strategy: models.PoolStrategyRoundRobin, IsDefault: true}
	if err := poolRepo.Create(ctx, pool); err != nil {
		t.Fatalf("create pool: %v", err)
	}
	if err := poolRepo.AddMember(ctx, &models.PoolMember{PoolID: pool.ID, ProfileID: profile1.ID, Position: 1}); err != nil {
		t.Fatalf("add member1: %v", err)
	}
	if err := poolRepo.AddMember(ctx, &models.PoolMember{PoolID: pool.ID, ProfileID: profile2.ID, Position: 2}); err != nil {
		t.Fatalf("add member2: %v", err)
	}

	loop := &models.Loop{Name: "loop-a", RepoPath: "/tmp/repo", IntervalSeconds: 1, PoolID: pool.ID}
	if err := loopRepo.Create(ctx, loop); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	runner := NewRunner(database, config.DefaultConfig())
	selected, waitUntil, err := runner.selectProfile(ctx, loop, profileRepo, poolRepo, runRepo)
	if err != nil {
		t.Fatalf("select profile: %v", err)
	}
	if waitUntil != nil {
		t.Fatalf("expected no wait, got %v", waitUntil)
	}
	if selected == nil || selected.ID != profile2.ID {
		t.Fatalf("expected profile2, got %#v", selected)
	}
}

func TestSelectProfilePoolWaitsForCooldown(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	ctx := context.Background()
	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)
	runRepo := db.NewLoopRunRepository(database)
	poolRepo := db.NewPoolRepository(database)

	now := time.Now().UTC()
	early := now.Add(5 * time.Minute)
	late := now.Add(10 * time.Minute)

	profile1 := &models.Profile{
		Name:            "cooldown-early",
		Harness:         models.HarnessPi,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
		CooldownUntil:   &early,
	}
	profile2 := &models.Profile{
		Name:            "cooldown-late",
		Harness:         models.HarnessPi,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
		CooldownUntil:   &late,
	}
	if err := profileRepo.Create(ctx, profile1); err != nil {
		t.Fatalf("create profile1: %v", err)
	}
	if err := profileRepo.Create(ctx, profile2); err != nil {
		t.Fatalf("create profile2: %v", err)
	}

	pool := &models.Pool{Name: "pool-b", Strategy: models.PoolStrategyRoundRobin, IsDefault: true}
	if err := poolRepo.Create(ctx, pool); err != nil {
		t.Fatalf("create pool: %v", err)
	}
	if err := poolRepo.AddMember(ctx, &models.PoolMember{PoolID: pool.ID, ProfileID: profile1.ID, Position: 1}); err != nil {
		t.Fatalf("add member1: %v", err)
	}
	if err := poolRepo.AddMember(ctx, &models.PoolMember{PoolID: pool.ID, ProfileID: profile2.ID, Position: 2}); err != nil {
		t.Fatalf("add member2: %v", err)
	}

	loop := &models.Loop{Name: "loop-b", RepoPath: "/tmp/repo", IntervalSeconds: 1, PoolID: pool.ID}
	if err := loopRepo.Create(ctx, loop); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	runner := NewRunner(database, config.DefaultConfig())
	selected, waitUntil, err := runner.selectProfile(ctx, loop, profileRepo, poolRepo, runRepo)
	if err != nil {
		t.Fatalf("select profile: %v", err)
	}
	if selected != nil {
		t.Fatalf("expected no profile, got %v", selected)
	}
	if waitUntil == nil {
		t.Fatalf("expected waitUntil")
	}
	if waitUntil.Before(early.Add(-time.Second)) || waitUntil.After(early.Add(time.Second)) {
		t.Fatalf("expected waitUntil near %s, got %s", early, waitUntil)
	}
}
