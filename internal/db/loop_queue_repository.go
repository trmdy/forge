// Package db provides SQLite database access for Forge.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tOgg1/forge/internal/models"
)

// LoopQueueRepository handles loop queue persistence.
type LoopQueueRepository struct {
	db *DB
}

// NewLoopQueueRepository creates a new LoopQueueRepository.
func NewLoopQueueRepository(db *DB) *LoopQueueRepository {
	return &LoopQueueRepository{db: db}
}

// Enqueue adds items to a loop's queue.
func (r *LoopQueueRepository) Enqueue(ctx context.Context, loopID string, items ...*models.LoopQueueItem) error {
	if len(items) == 0 {
		return nil
	}

	maxPos, err := r.getMaxPosition(ctx, loopID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	for i, item := range items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("invalid queue item at index %d: %w", i, err)
		}
		if item.ID == "" {
			item.ID = uuid.New().String()
		}
		item.LoopID = loopID
		item.CreatedAt = now
		item.Position = maxPos + i + 1
		if item.Status == "" {
			item.Status = models.LoopQueueStatusPending
		}

		_, err := r.db.ExecContext(ctx, `
			INSERT INTO loop_queue_items (
				id, loop_id, type, position, status, attempts, payload_json,
				error_message, created_at, dispatched_at, completed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			item.ID,
			item.LoopID,
			string(item.Type),
			item.Position,
			string(item.Status),
			item.Attempts,
			string(item.Payload),
			item.Error,
			item.CreatedAt.Format(time.RFC3339),
			stringTimePtr(item.DispatchedAt),
			stringTimePtr(item.CompletedAt),
		)
		if err != nil {
			return fmt.Errorf("failed to insert loop queue item: %w", err)
		}
	}

	return nil
}

// Peek returns the next pending item without removing it.
func (r *LoopQueueRepository) Peek(ctx context.Context, loopID string) (*models.LoopQueueItem, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, loop_id, type, position, status, attempts, payload_json,
			error_message, created_at, dispatched_at, completed_at
		FROM loop_queue_items
		WHERE loop_id = ? AND status = ?
		ORDER BY position ASC
		LIMIT 1
	`, loopID, string(models.LoopQueueStatusPending))

	item, err := r.scanLoopQueueItem(row)
	if err != nil {
		if errors.Is(err, ErrQueueItemNotFound) {
			return nil, ErrQueueEmpty
		}
		return nil, err
	}

	return item, nil
}

// Dequeue removes and returns the next pending item.
func (r *LoopQueueRepository) Dequeue(ctx context.Context, loopID string) (*models.LoopQueueItem, error) {
	item, err := r.Peek(ctx, loopID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `
		UPDATE loop_queue_items
		SET status = ?, dispatched_at = ?
		WHERE id = ?
	`, string(models.LoopQueueStatusDispatched), now.Format(time.RFC3339), item.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update loop queue status: %w", err)
	}

	item.Status = models.LoopQueueStatusDispatched
	item.DispatchedAt = &now
	return item, nil
}

// List returns all queue items for a loop.
func (r *LoopQueueRepository) List(ctx context.Context, loopID string) ([]*models.LoopQueueItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, loop_id, type, position, status, attempts, payload_json,
			error_message, created_at, dispatched_at, completed_at
		FROM loop_queue_items
		WHERE loop_id = ?
		ORDER BY position ASC
	`, loopID)
	if err != nil {
		return nil, fmt.Errorf("failed to query loop queue items: %w", err)
	}
	defer rows.Close()

	items := make([]*models.LoopQueueItem, 0)
	for rows.Next() {
		item, err := r.scanLoopQueueItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// Clear removes all pending items from a loop queue.
func (r *LoopQueueRepository) Clear(ctx context.Context, loopID string) (int, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM loop_queue_items
		WHERE loop_id = ? AND status = ?
	`, loopID, string(models.LoopQueueStatusPending))
	if err != nil {
		return 0, fmt.Errorf("failed to clear loop queue: %w", err)
	}
	count, _ := result.RowsAffected()
	return int(count), nil
}

// Remove deletes a queue item by ID.
func (r *LoopQueueRepository) Remove(ctx context.Context, itemID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM loop_queue_items WHERE id = ?`, itemID)
	if err != nil {
		return fmt.Errorf("failed to remove loop queue item: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrQueueItemNotFound
	}
	return nil
}

// UpdateStatus updates the status of a queue item.
func (r *LoopQueueRepository) UpdateStatus(ctx context.Context, itemID string, status models.LoopQueueItemStatus, errorMsg string) error {
	completedAt := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE loop_queue_items
		SET status = ?, error_message = ?, completed_at = ?
		WHERE id = ?
	`, string(status), nullableString(errorMsg), stringTimePtr(&completedAt), itemID)
	if err != nil {
		return fmt.Errorf("failed to update loop queue status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrQueueItemNotFound
	}
	return nil
}

// Reorder updates queue item positions based on the provided ordered IDs.
func (r *LoopQueueRepository) Reorder(ctx context.Context, loopID string, orderedIDs []string) error {
	if len(orderedIDs) == 0 {
		return nil
	}

	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		for i, id := range orderedIDs {
			position := i + 1
			_, err := tx.ExecContext(ctx, `
				UPDATE loop_queue_items
				SET position = ?
				WHERE id = ? AND loop_id = ?
			`, position, id, loopID)
			if err != nil {
				return fmt.Errorf("failed to update queue position: %w", err)
			}
		}
		return nil
	})
}

func (r *LoopQueueRepository) getMaxPosition(ctx context.Context, loopID string) (int, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(position), 0) FROM loop_queue_items WHERE loop_id = ?
	`, loopID)

	var maxPos int
	if err := row.Scan(&maxPos); err != nil {
		return 0, fmt.Errorf("failed to get max queue position: %w", err)
	}
	return maxPos, nil
}

func (r *LoopQueueRepository) scanLoopQueueItem(scanner interface{ Scan(...any) error }) (*models.LoopQueueItem, error) {
	var (
		id           string
		loopID       string
		typeValue    string
		position     int
		status       string
		attempts     int
		payload      string
		errorMsg     sql.NullString
		createdAt    string
		dispatchedAt sql.NullString
		completedAt  sql.NullString
	)

	if err := scanner.Scan(
		&id,
		&loopID,
		&typeValue,
		&position,
		&status,
		&attempts,
		&payload,
		&errorMsg,
		&createdAt,
		&dispatchedAt,
		&completedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrQueueItemNotFound
		}
		return nil, fmt.Errorf("failed to scan loop queue item: %w", err)
	}

	item := &models.LoopQueueItem{
		ID:       id,
		LoopID:   loopID,
		Type:     models.LoopQueueItemType(typeValue),
		Position: position,
		Status:   models.LoopQueueItemStatus(status),
		Attempts: attempts,
		Payload:  []byte(payload),
		Error:    errorMsg.String,
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		item.CreatedAt = t
	}
	if dispatchedAt.Valid && dispatchedAt.String != "" {
		if t, err := time.Parse(time.RFC3339, dispatchedAt.String); err == nil {
			item.DispatchedAt = &t
		}
	}
	if completedAt.Valid && completedAt.String != "" {
		if t, err := time.Parse(time.RFC3339, completedAt.String); err == nil {
			item.CompletedAt = &t
		}
	}

	return item, nil
}
