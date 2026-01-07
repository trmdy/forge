package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/tOgg1/forge/internal/models"
)

func createTestLoop(t *testing.T, db *DB) *models.Loop {
	t.Helper()

	repo := NewLoopRepository(db)
	loop := &models.Loop{
		Name:            "Beefy Flanders",
		RepoPath:        "/repo",
		IntervalSeconds: 10,
		State:           models.LoopStateStopped,
	}
	if err := repo.Create(context.Background(), loop); err != nil {
		t.Fatalf("create loop: %v", err)
	}
	return loop
}

func newLoopMessageItem(t *testing.T, text string) *models.LoopQueueItem {
	t.Helper()

	payload, err := json.Marshal(models.MessageAppendPayload{Text: text})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	return &models.LoopQueueItem{
		ID:      uuid.New().String(),
		Type:    models.LoopQueueItemMessageAppend,
		Payload: payload,
	}
}

func TestLoopQueueRepository_EnqueuePeekDequeue(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	loop := createTestLoop(t, db)
	repo := NewLoopQueueRepository(db)
	ctx := context.Background()

	item1 := newLoopMessageItem(t, "first")
	item2 := newLoopMessageItem(t, "second")

	if err := repo.Enqueue(ctx, loop.ID, item1, item2); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	peeked, err := repo.Peek(ctx, loop.ID)
	if err != nil {
		t.Fatalf("Peek failed: %v", err)
	}
	if peeked.Position != 1 {
		t.Fatalf("expected position 1, got %d", peeked.Position)
	}

	dequeued, err := repo.Dequeue(ctx, loop.ID)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if dequeued.Status != models.LoopQueueStatusDispatched {
		t.Fatalf("expected dispatched status, got %q", dequeued.Status)
	}

	items, err := repo.List(ctx, loop.ID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}
