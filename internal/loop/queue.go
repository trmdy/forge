package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

type messageEntry struct {
	Text      string
	Timestamp time.Time
	Source    string
}

type queuePlan struct {
	Messages       []messageEntry
	OverridePrompt *models.NextPromptOverridePayload
	StopRequested  bool
	KillRequested  bool
	PauseDuration  time.Duration
	PauseBeforeRun bool
	ConsumeItemIDs []string
	PauseItemIDs   []string
	StopItemIDs    []string
	KillItemIDs    []string
}

func buildQueuePlan(ctx context.Context, repo *db.LoopQueueRepository, loopID string, steerMessages []messageEntry) (*queuePlan, error) {
	items, err := repo.List(ctx, loopID)
	if err != nil {
		return nil, err
	}

	plan := &queuePlan{}
	if len(steerMessages) > 0 {
		plan.Messages = append(plan.Messages, steerMessages...)
	}

	for _, item := range items {
		if item.Status != models.LoopQueueStatusPending {
			continue
		}

		switch item.Type {
		case models.LoopQueueItemMessageAppend:
			payload, err := decodePayload[models.MessageAppendPayload](item.Payload)
			if err != nil {
				return nil, err
			}
			plan.Messages = append(plan.Messages, messageEntry{Text: payload.Text, Timestamp: item.CreatedAt, Source: "queue"})
			plan.ConsumeItemIDs = append(plan.ConsumeItemIDs, item.ID)
		case models.LoopQueueItemNextPromptOverride:
			payload, err := decodePayload[models.NextPromptOverridePayload](item.Payload)
			if err != nil {
				return nil, err
			}
			if plan.OverridePrompt == nil {
				plan.OverridePrompt = &payload
				plan.ConsumeItemIDs = append(plan.ConsumeItemIDs, item.ID)
			}
		case models.LoopQueueItemPause:
			payload, err := decodePayload[models.LoopPausePayload](item.Payload)
			if err != nil {
				return nil, err
			}
			plan.PauseDuration = time.Duration(payload.DurationSeconds) * time.Second
			plan.PauseItemIDs = append(plan.PauseItemIDs, item.ID)
			plan.PauseBeforeRun = plan.OverridePrompt == nil && len(plan.Messages) == 0
			return plan, nil
		case models.LoopQueueItemStopGraceful:
			plan.StopRequested = true
			plan.StopItemIDs = append(plan.StopItemIDs, item.ID)
			return plan, nil
		case models.LoopQueueItemKillNow:
			plan.KillRequested = true
			plan.KillItemIDs = append(plan.KillItemIDs, item.ID)
			return plan, nil
		case models.LoopQueueItemSteerMessage:
			payload, err := decodePayload[models.SteerPayload](item.Payload)
			if err != nil {
				return nil, err
			}
			plan.Messages = append(plan.Messages, messageEntry{Text: payload.Message, Timestamp: item.CreatedAt, Source: "steer"})
			plan.ConsumeItemIDs = append(plan.ConsumeItemIDs, item.ID)
		default:
			return nil, fmt.Errorf("unsupported queue item type %q", item.Type)
		}
	}

	return plan, nil
}

func decodePayload[T any](payload []byte) (T, error) {
	var data T
	if err := json.Unmarshal(payload, &data); err != nil {
		return data, err
	}
	return data, nil
}

func markQueueCompleted(ctx context.Context, repo *db.LoopQueueRepository, ids []string) error {
	for _, id := range ids {
		if err := repo.UpdateStatus(ctx, id, models.LoopQueueStatusCompleted, ""); err != nil {
			return err
		}
	}
	return nil
}

func hasPendingStop(ctx context.Context, repo *db.LoopQueueRepository, loopID string) (bool, error) {
	items, err := repo.List(ctx, loopID)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.Status == models.LoopQueueStatusPending && item.Type == models.LoopQueueItemStopGraceful {
			return true, nil
		}
	}
	return false, nil
}

func consumePendingStop(ctx context.Context, repo *db.LoopQueueRepository, loopID string) error {
	items, err := repo.List(ctx, loopID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Status == models.LoopQueueStatusPending && item.Type == models.LoopQueueItemStopGraceful {
			if err := repo.UpdateStatus(ctx, item.ID, models.LoopQueueStatusCompleted, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasPendingKill(ctx context.Context, repo *db.LoopQueueRepository, loopID string) (bool, error) {
	items, err := repo.List(ctx, loopID)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.Status == models.LoopQueueStatusPending && item.Type == models.LoopQueueItemKillNow {
			return true, nil
		}
	}
	return false, nil
}

func consumePendingKill(ctx context.Context, repo *db.LoopQueueRepository, loopID string) error {
	items, err := repo.List(ctx, loopID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Status == models.LoopQueueStatusPending && item.Type == models.LoopQueueItemKillNow {
			if err := repo.UpdateStatus(ctx, item.ID, models.LoopQueueStatusCompleted, ""); err != nil {
				return err
			}
		}
	}
	return nil
}
