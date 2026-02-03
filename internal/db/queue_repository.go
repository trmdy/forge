// Package db provides SQLite database access for Forge.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tOgg1/forge/internal/models"
)

// Queue repository errors.
var (
	ErrQueueItemNotFound = errors.New("queue item not found")
	ErrQueueEmpty        = errors.New("queue is empty")
)

// QueueRepository handles queue item persistence.
type QueueRepository struct {
	db *DB
}

// NewQueueRepository creates a new QueueRepository.
func NewQueueRepository(db *DB) *QueueRepository {
	return &QueueRepository{db: db}
}

// Enqueue adds one or more items to an agent's queue.
func (r *QueueRepository) Enqueue(ctx context.Context, agentID string, items ...*models.QueueItem) error {
	if len(items) == 0 {
		return nil
	}

	// Get the current max position for this agent
	maxPos, err := r.getMaxPosition(ctx, agentID)
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

		item.AgentID = agentID
		item.CreatedAt = now
		item.Position = maxPos + i + 1

		if item.Status == "" {
			item.Status = models.QueueItemStatusPending
		}
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO queue_items (
				id, agent_id, type, position, status, attempts, payload_json,
				error_message, created_at, dispatched_at, completed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			item.ID,
			item.AgentID,
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
			return fmt.Errorf("failed to insert queue item: %w", err)
		}
	}

	return nil
}

// Dequeue removes and returns the next pending item from the queue.
func (r *QueueRepository) Dequeue(ctx context.Context, agentID string) (*models.QueueItem, error) {
	item, err := r.Peek(ctx, agentID)
	if err != nil {
		return nil, err
	}

	// Update the item status to dispatched
	now := time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `
		UPDATE queue_items 
		SET status = ?, dispatched_at = ?
		WHERE id = ?
	`, string(models.QueueItemStatusDispatched), now.Format(time.RFC3339), item.ID)

	if err != nil {
		return nil, fmt.Errorf("failed to update queue item status: %w", err)
	}

	item.Status = models.QueueItemStatusDispatched
	item.DispatchedAt = &now

	return item, nil
}

// Peek returns the next pending item without removing it.
func (r *QueueRepository) Peek(ctx context.Context, agentID string) (*models.QueueItem, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT 
			id, agent_id, type, position, status, attempts, payload_json,
			error_message, created_at, dispatched_at, completed_at
		FROM queue_items
		WHERE agent_id = ? AND status = ?
		ORDER BY position ASC
		LIMIT 1
	`, agentID, string(models.QueueItemStatusPending))

	item, err := r.scanQueueItem(row)
	if err != nil {
		if errors.Is(err, ErrQueueItemNotFound) {
			return nil, ErrQueueEmpty
		}
		return nil, err
	}

	return item, nil
}

// List returns all queue items for an agent.
func (r *QueueRepository) List(ctx context.Context, agentID string) ([]*models.QueueItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT 
			id, agent_id, type, position, status, attempts, payload_json,
			error_message, created_at, dispatched_at, completed_at
		FROM queue_items
		WHERE agent_id = ?
		ORDER BY position ASC
	`, agentID)

	if err != nil {
		return nil, fmt.Errorf("failed to query queue items: %w", err)
	}
	defer rows.Close()

	return r.scanQueueItems(rows)
}

// ListPending returns only pending queue items for an agent.
func (r *QueueRepository) ListPending(ctx context.Context, agentID string) ([]*models.QueueItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT 
			id, agent_id, type, position, status, attempts, payload_json,
			error_message, created_at, dispatched_at, completed_at
		FROM queue_items
		WHERE agent_id = ? AND status = ?
		ORDER BY position ASC
	`, agentID, string(models.QueueItemStatusPending))

	if err != nil {
		return nil, fmt.Errorf("failed to query pending queue items: %w", err)
	}
	defer rows.Close()

	return r.scanQueueItems(rows)
}

// Reorder updates the position of items in the queue.
func (r *QueueRepository) Reorder(ctx context.Context, agentID string, itemIDs []string) error {
	if len(itemIDs) == 0 {
		return nil
	}

	pending, err := r.ListPending(ctx, agentID)
	if err != nil {
		return err
	}

	pendingIDs := make(map[string]struct{}, len(pending))
	for _, item := range pending {
		if item == nil {
			continue
		}
		pendingIDs[item.ID] = struct{}{}
	}

	if len(itemIDs) != len(pendingIDs) {
		return fmt.Errorf("reorder list must include all pending items for agent %s", agentID)
	}

	seen := make(map[string]struct{}, len(itemIDs))
	for _, id := range itemIDs {
		if id == "" {
			return fmt.Errorf("queue item id is required")
		}
		if _, ok := pendingIDs[id]; !ok {
			return fmt.Errorf("queue item %s not found in pending queue for agent %s", id, agentID)
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("duplicate queue item %s in reorder list", id)
		}
		seen[id] = struct{}{}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for i, id := range itemIDs {
		result, err := tx.ExecContext(ctx, `
			UPDATE queue_items SET position = ?
			WHERE id = ? AND agent_id = ? AND status = ?
		`, i+1, id, agentID, string(models.QueueItemStatusPending))

		if err != nil {
			return fmt.Errorf("failed to update position for item %s: %w", id, err)
		}

		rows, _ := result.RowsAffected()
		if rows == 0 {
			return fmt.Errorf("queue item %s not found or doesn't belong to agent %s", id, agentID)
		}
	}

	return tx.Commit()
}

// Clear removes all pending items from an agent's queue.
func (r *QueueRepository) Clear(ctx context.Context, agentID string) (int, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM queue_items 
		WHERE agent_id = ? AND status = ?
	`, agentID, string(models.QueueItemStatusPending))

	if err != nil {
		return 0, fmt.Errorf("failed to clear queue: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rows), nil
}

// InsertAt inserts a queue item at a specific position.
func (r *QueueRepository) InsertAt(ctx context.Context, agentID string, position int, item *models.QueueItem) error {
	if err := item.Validate(); err != nil {
		return fmt.Errorf("invalid queue item: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Shift existing items down
	_, err = tx.ExecContext(ctx, `
		UPDATE queue_items 
		SET position = position + 1
		WHERE agent_id = ? AND position >= ?
	`, agentID, position)

	if err != nil {
		return fmt.Errorf("failed to shift items: %w", err)
	}

	// Insert the new item
	if item.ID == "" {
		item.ID = uuid.New().String()
	}

	now := time.Now().UTC()
	item.AgentID = agentID
	item.Position = position
	item.CreatedAt = now

	if item.Status == "" {
		item.Status = models.QueueItemStatusPending
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO queue_items (
			id, agent_id, type, position, status, attempts, payload_json,
			error_message, created_at, dispatched_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		item.ID,
		item.AgentID,
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
		return fmt.Errorf("failed to insert queue item: %w", err)
	}

	return tx.Commit()
}

// Remove deletes a specific queue item.
func (r *QueueRepository) Remove(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM queue_items WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete queue item: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrQueueItemNotFound
	}

	return nil
}

// Get retrieves a specific queue item by ID.
func (r *QueueRepository) Get(ctx context.Context, id string) (*models.QueueItem, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT 
			id, agent_id, type, position, status, attempts, payload_json,
			error_message, created_at, dispatched_at, completed_at
		FROM queue_items WHERE id = ?
	`, id)

	return r.scanQueueItem(row)
}

// UpdateStatus updates the status of a queue item.
func (r *QueueRepository) UpdateStatus(ctx context.Context, id string, status models.QueueItemStatus, errorMsg string) error {
	now := time.Now().UTC()

	var completedAt *string
	if status == models.QueueItemStatusCompleted || status == models.QueueItemStatusFailed {
		s := now.Format(time.RFC3339)
		completedAt = &s
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE queue_items 
		SET status = ?, error_message = ?, completed_at = ?
		WHERE id = ?
	`, string(status), errorMsg, completedAt, id)

	if err != nil {
		return fmt.Errorf("failed to update queue item status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrQueueItemNotFound
	}

	return nil
}

// UpdateAttempts updates the dispatch attempt count for a queue item.
func (r *QueueRepository) UpdateAttempts(ctx context.Context, id string, attempts int) error {
	if attempts < 0 {
		attempts = 0
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE queue_items
		SET attempts = ?
		WHERE id = ?
	`, attempts, id)
	if err != nil {
		return fmt.Errorf("failed to update queue item attempts: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrQueueItemNotFound
	}

	return nil
}

// Count returns the number of pending items in an agent's queue.
func (r *QueueRepository) Count(ctx context.Context, agentID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM queue_items
		WHERE agent_id = ? AND status = ?
	`, agentID, string(models.QueueItemStatusPending)).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("failed to count queue items: %w", err)
	}

	return count, nil
}

// getMaxPosition returns the current maximum position for an agent's queue.
func (r *QueueRepository) getMaxPosition(ctx context.Context, agentID string) (int, error) {
	var maxPos sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT MAX(position) FROM queue_items WHERE agent_id = ?
	`, agentID).Scan(&maxPos)

	if err != nil {
		return 0, fmt.Errorf("failed to get max position: %w", err)
	}

	if !maxPos.Valid {
		return 0, nil
	}

	return int(maxPos.Int64), nil
}

// scanQueueItem scans a single queue item from a row.
func (r *QueueRepository) scanQueueItem(row *sql.Row) (*models.QueueItem, error) {
	var item models.QueueItem
	var itemType, status string
	var attempts int
	var payloadJSON string
	var errorMsg sql.NullString
	var createdAt string
	var dispatchedAt, completedAt sql.NullString

	err := row.Scan(
		&item.ID,
		&item.AgentID,
		&itemType,
		&item.Position,
		&status,
		&attempts,
		&payloadJSON,
		&errorMsg,
		&createdAt,
		&dispatchedAt,
		&completedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrQueueItemNotFound
		}
		return nil, fmt.Errorf("failed to scan queue item: %w", err)
	}

	item.Type = models.QueueItemType(itemType)
	item.Status = models.QueueItemStatus(status)
	item.Attempts = attempts
	item.Payload = json.RawMessage(payloadJSON)

	if errorMsg.Valid {
		item.Error = errorMsg.String
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		item.CreatedAt = t
	}
	if dispatchedAt.Valid {
		if t, err := time.Parse(time.RFC3339, dispatchedAt.String); err == nil {
			item.DispatchedAt = &t
		}
	}
	if completedAt.Valid {
		if t, err := time.Parse(time.RFC3339, completedAt.String); err == nil {
			item.CompletedAt = &t
		}
	}

	return &item, nil
}

// scanQueueItems scans multiple queue items from rows.
func (r *QueueRepository) scanQueueItems(rows *sql.Rows) ([]*models.QueueItem, error) {
	var items []*models.QueueItem

	for rows.Next() {
		var item models.QueueItem
		var itemType, status string
		var attempts int
		var payloadJSON string
		var errorMsg sql.NullString
		var createdAt string
		var dispatchedAt, completedAt sql.NullString

		err := rows.Scan(
			&item.ID,
			&item.AgentID,
			&itemType,
			&item.Position,
			&status,
			&attempts,
			&payloadJSON,
			&errorMsg,
			&createdAt,
			&dispatchedAt,
			&completedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan queue item: %w", err)
		}

		item.Type = models.QueueItemType(itemType)
		item.Status = models.QueueItemStatus(status)
		item.Attempts = attempts
		item.Payload = json.RawMessage(payloadJSON)

		if errorMsg.Valid {
			item.Error = errorMsg.String
		}

		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			item.CreatedAt = t
		}
		if dispatchedAt.Valid {
			if t, err := time.Parse(time.RFC3339, dispatchedAt.String); err == nil {
				item.DispatchedAt = &t
			}
		}
		if completedAt.Valid {
			if t, err := time.Parse(time.RFC3339, completedAt.String); err == nil {
				item.CompletedAt = &t
			}
		}

		items = append(items, &item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating queue items: %w", err)
	}

	return items, nil
}
