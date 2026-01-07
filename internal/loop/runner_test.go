package loop

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/tOgg1/forge/internal/config"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/testutil"
)

func TestRunnerRunOnceConsumesQueue(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = t.TempDir()
	cfg.Global.ConfigDir = t.TempDir()

	profileRepo := db.NewProfileRepository(database)
	loopRepo := db.NewLoopRepository(database)
	queueRepo := db.NewLoopQueueRepository(database)
	runRepo := db.NewLoopRunRepository(database)

	profile := &models.Profile{
		Name:            "pi-default",
		Harness:         models.HarnessPi,
		PromptMode:      models.PromptModeEnv,
		CommandTemplate: "pi -p \"$FORGE_PROMPT_CONTENT\"",
		MaxConcurrency:  1,
	}
	if err := profileRepo.Create(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	loopEntry := &models.Loop{
		Name:            "loop-a",
		RepoPath:        repoDir,
		BasePromptMsg:   "base",
		IntervalSeconds: 1,
		ProfileID:       profile.ID,
		State:           models.LoopStateStopped,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	overridePayload := mustJSON(models.NextPromptOverridePayload{Prompt: "override", IsPath: false})
	messagePayload := mustJSON(models.MessageAppendPayload{Text: "hello"})
	if err := queueRepo.Enqueue(context.Background(), loopEntry.ID,
		&models.LoopQueueItem{Type: models.LoopQueueItemNextPromptOverride, Payload: overridePayload},
		&models.LoopQueueItem{Type: models.LoopQueueItemMessageAppend, Payload: messagePayload},
	); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	var capturedPrompt string
	runner := NewRunner(database, cfg)
	runner.Exec = func(ctx context.Context, profile models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
		capturedPrompt = promptContent
		return 0, "ok", nil
	}

	if err := runner.RunOnce(context.Background(), loopEntry.ID); err != nil {
		t.Fatalf("run once: %v", err)
	}

	if !strings.Contains(capturedPrompt, "override") {
		t.Fatalf("expected override prompt in content")
	}
	if !strings.Contains(capturedPrompt, "hello") {
		t.Fatalf("expected operator message in content")
	}

	items, err := queueRepo.List(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	for _, item := range items {
		if item.Status != models.LoopQueueStatusCompleted {
			t.Fatalf("expected queue item completed, got %s", item.Status)
		}
	}

	runs, err := runRepo.ListByLoop(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].PromptSource != "override" || !runs[0].PromptOverride {
		t.Fatalf("expected override prompt source")
	}
}

func TestRunnerStopQueueStopsLoop(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	repoDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Global.DataDir = t.TempDir()
	cfg.Global.ConfigDir = t.TempDir()

	loopRepo := db.NewLoopRepository(database)
	queueRepo := db.NewLoopQueueRepository(database)
	runRepo := db.NewLoopRunRepository(database)

	loopEntry := &models.Loop{
		Name:            "loop-stop",
		RepoPath:        repoDir,
		IntervalSeconds: 1,
		State:           models.LoopStateStopped,
	}
	if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	stopPayload := mustJSON(models.StopPayload{Reason: "test"})
	if err := queueRepo.Enqueue(context.Background(), loopEntry.ID,
		&models.LoopQueueItem{Type: models.LoopQueueItemStopGraceful, Payload: stopPayload},
	); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	runner := NewRunner(database, cfg)
	runner.Exec = func(ctx context.Context, profile models.Profile, promptPath, promptContent, workDir string, output io.Writer) (int, string, error) {
		return 0, "", nil
	}

	if err := runner.RunOnce(context.Background(), loopEntry.ID); err != nil {
		t.Fatalf("run once: %v", err)
	}

	items, err := queueRepo.List(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	for _, item := range items {
		if item.Status != models.LoopQueueStatusCompleted {
			t.Fatalf("expected queue item completed, got %s", item.Status)
		}
	}

	runs, err := runRepo.ListByLoop(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no runs, got %d", len(runs))
	}

	updated, err := loopRepo.Get(context.Background(), loopEntry.ID)
	if err != nil {
		t.Fatalf("get loop: %v", err)
	}
	if updated.State != models.LoopStateStopped {
		t.Fatalf("expected loop stopped, got %s", updated.State)
	}
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
