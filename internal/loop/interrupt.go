package loop

import (
	"context"
	"time"

	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

type interruptResult struct {
	killOnly     bool
	steerMessage string
	reason       string
}

func watchInterrupts(ctx context.Context, database *db.DB, loopID string, startedAt time.Time, pollInterval time.Duration) (*interruptResult, error) {
	queueRepo := db.NewLoopQueueRepository(database)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			items, err := queueRepo.List(ctx, loopID)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				if item.Status != models.LoopQueueStatusPending {
					continue
				}
				if !item.CreatedAt.After(startedAt) {
					continue
				}
				switch item.Type {
				case models.LoopQueueItemKillNow:
					payload, err := decodePayload[models.KillPayload](item.Payload)
					if err != nil {
						return nil, err
					}
					_ = queueRepo.UpdateStatus(ctx, item.ID, models.LoopQueueStatusCompleted, "")
					return &interruptResult{killOnly: true, reason: payload.Reason}, nil
				case models.LoopQueueItemSteerMessage:
					payload, err := decodePayload[models.SteerPayload](item.Payload)
					if err != nil {
						return nil, err
					}
					_ = queueRepo.UpdateStatus(ctx, item.ID, models.LoopQueueStatusCompleted, "")
					return &interruptResult{steerMessage: payload.Message, reason: "steer interrupt"}, nil
				}
			}
		}
	}
}
