// Package db provides SQLite database access for Swarm.
package db

import (
	"context"
	"fmt"
	"time"
)

// FileLockRepository handles file lock persistence.
type FileLockRepository struct {
	db *DB
}

// NewFileLockRepository creates a new FileLockRepository.
func NewFileLockRepository(db *DB) *FileLockRepository {
	return &FileLockRepository{db: db}
}

// CleanupExpired marks expired locks as released.
// Returns the number of locks updated.
func (r *FileLockRepository) CleanupExpired(ctx context.Context, now time.Time) (int, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("file lock repository not initialized")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	timestamp := now.Format(time.RFC3339)
	result, err := r.db.ExecContext(ctx, `
		UPDATE file_locks
		SET released_at = ?
		WHERE released_at IS NULL
		  AND expires_at <= ?
	`, timestamp, timestamp)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired file locks: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rowsAffected), nil
}
