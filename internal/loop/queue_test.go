package loop

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/testutil"
)

func TestBuildQueuePlan(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	ctx := context.Background()
	loopRepo := db.NewLoopRepository(database)
	queueRepo := db.NewLoopQueueRepository(database)

	loop := &models.Loop{
		Name:            "loop-a",
		RepoPath:        "/tmp/repo",
		IntervalSeconds: 10,
	}
	if err := loopRepo.Create(ctx, loop); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	messagePayload, _ := json.Marshal(models.MessageAppendPayload{Text: "hello"})
	overridePayload, _ := json.Marshal(models.NextPromptOverridePayload{Prompt: "override", IsPath: false})
	pausePayload, _ := json.Marshal(models.LoopPausePayload{DurationSeconds: 5})

	items := []*models.LoopQueueItem{
		{Type: models.LoopQueueItemMessageAppend, Payload: messagePayload},
		{Type: models.LoopQueueItemNextPromptOverride, Payload: overridePayload},
		{Type: models.LoopQueueItemPause, Payload: pausePayload},
		{Type: models.LoopQueueItemMessageAppend, Payload: messagePayload},
	}

	if err := queueRepo.Enqueue(ctx, loop.ID, items...); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	plan, err := buildQueuePlan(ctx, queueRepo, loop.ID, nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	if plan.OverridePrompt == nil || plan.OverridePrompt.Prompt != "override" {
		t.Fatalf("expected override prompt")
	}
	if len(plan.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(plan.Messages))
	}
	if plan.PauseDuration == 0 {
		t.Fatalf("expected pause duration")
	}
	if plan.PauseBeforeRun {
		t.Fatalf("expected pause after run when messages are queued")
	}
	if len(plan.ConsumeItemIDs) != 2 {
		t.Fatalf("expected 2 consumed items, got %d", len(plan.ConsumeItemIDs))
	}
}

func TestBuildQueuePlanPauseBeforeRun(t *testing.T) {
	database, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	ctx := context.Background()
	loopRepo := db.NewLoopRepository(database)
	queueRepo := db.NewLoopQueueRepository(database)

	loop := &models.Loop{
		Name:            "loop-b",
		RepoPath:        "/tmp/repo",
		IntervalSeconds: 10,
	}
	if err := loopRepo.Create(ctx, loop); err != nil {
		t.Fatalf("create loop: %v", err)
	}

	pausePayload, _ := json.Marshal(models.LoopPausePayload{DurationSeconds: 5})
	if err := queueRepo.Enqueue(ctx, loop.ID, &models.LoopQueueItem{Type: models.LoopQueueItemPause, Payload: pausePayload}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	plan, err := buildQueuePlan(ctx, queueRepo, loop.ID, nil)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	if plan.PauseDuration == 0 {
		t.Fatalf("expected pause duration")
	}
	if !plan.PauseBeforeRun {
		t.Fatalf("expected pause before run when no messages are queued")
	}
	if len(plan.ConsumeItemIDs) != 0 {
		t.Fatalf("expected no consumed items, got %d", len(plan.ConsumeItemIDs))
	}
}
