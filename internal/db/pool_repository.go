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

// Pool repository errors.
var (
	ErrPoolNotFound      = errors.New("pool not found")
	ErrPoolAlreadyExists = errors.New("pool already exists")
)

// PoolRepository handles pool persistence.
type PoolRepository struct {
	db *DB
}

// NewPoolRepository creates a new PoolRepository.
func NewPoolRepository(db *DB) *PoolRepository {
	return &PoolRepository{db: db}
}

// Create adds a new pool to the database.
func (r *PoolRepository) Create(ctx context.Context, pool *models.Pool) error {
	if err := pool.Validate(); err != nil {
		return fmt.Errorf("invalid pool: %w", err)
	}

	if pool.ID == "" {
		pool.ID = uuid.New().String()
	}

	now := time.Now().UTC()
	pool.CreatedAt = now
	pool.UpdatedAt = now

	var metadataJSON *string
	if pool.Metadata != nil {
		data, err := json.Marshal(pool.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal pool metadata: %w", err)
		}
		value := string(data)
		metadataJSON = &value
	}

	isDefault := 0
	if pool.IsDefault {
		isDefault = 1
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO pools (id, name, strategy, is_default, metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		pool.ID,
		pool.Name,
		string(pool.Strategy),
		isDefault,
		metadataJSON,
		pool.CreatedAt.Format(time.RFC3339),
		pool.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return ErrPoolAlreadyExists
		}
		return fmt.Errorf("failed to insert pool: %w", err)
	}

	return nil
}

// Get retrieves a pool by ID.
func (r *PoolRepository) Get(ctx context.Context, id string) (*models.Pool, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, strategy, is_default, metadata_json, created_at, updated_at
		FROM pools WHERE id = ?
	`, id)
	return r.scanPool(row)
}

// GetByName retrieves a pool by name.
func (r *PoolRepository) GetByName(ctx context.Context, name string) (*models.Pool, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, strategy, is_default, metadata_json, created_at, updated_at
		FROM pools WHERE name = ?
	`, name)
	return r.scanPool(row)
}

// GetDefault retrieves the default pool.
func (r *PoolRepository) GetDefault(ctx context.Context) (*models.Pool, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, strategy, is_default, metadata_json, created_at, updated_at
		FROM pools WHERE is_default = 1
		ORDER BY created_at
		LIMIT 1
	`)
	return r.scanPool(row)
}

// List retrieves all pools.
func (r *PoolRepository) List(ctx context.Context) ([]*models.Pool, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, strategy, is_default, metadata_json, created_at, updated_at
		FROM pools
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query pools: %w", err)
	}
	defer rows.Close()

	pools := make([]*models.Pool, 0)
	for rows.Next() {
		pool, err := r.scanPool(rows)
		if err != nil {
			return nil, err
		}
		pools = append(pools, pool)
	}

	return pools, nil
}

// Update updates a pool.
func (r *PoolRepository) Update(ctx context.Context, pool *models.Pool) error {
	if err := pool.Validate(); err != nil {
		return fmt.Errorf("invalid pool: %w", err)
	}

	pool.UpdatedAt = time.Now().UTC()

	var metadataJSON *string
	if pool.Metadata != nil {
		data, err := json.Marshal(pool.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal pool metadata: %w", err)
		}
		value := string(data)
		metadataJSON = &value
	}

	isDefault := 0
	if pool.IsDefault {
		isDefault = 1
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE pools
		SET name = ?, strategy = ?, is_default = ?, metadata_json = ?, updated_at = ?
		WHERE id = ?
	`,
		pool.Name,
		string(pool.Strategy),
		isDefault,
		metadataJSON,
		pool.UpdatedAt.Format(time.RFC3339),
		pool.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update pool: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrPoolNotFound
	}

	return nil
}

// Delete removes a pool.
func (r *PoolRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM pools WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete pool: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrPoolNotFound
	}
	return nil
}

// SetDefault marks a pool as default and clears other defaults.
func (r *PoolRepository) SetDefault(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE pools SET is_default = 0`)
	if err != nil {
		return fmt.Errorf("failed to clear defaults: %w", err)
	}

	result, err := r.db.ExecContext(ctx, `UPDATE pools SET is_default = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to set default pool: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrPoolNotFound
	}
	return nil
}

// AddMember adds a profile to a pool.
func (r *PoolRepository) AddMember(ctx context.Context, member *models.PoolMember) error {
	if member.ID == "" {
		member.ID = uuid.New().String()
	}
	if member.Weight == 0 {
		member.Weight = 1
	}
	if member.CreatedAt.IsZero() {
		member.CreatedAt = time.Now().UTC()
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO pool_members (id, pool_id, profile_id, weight, position, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		member.ID,
		member.PoolID,
		member.ProfileID,
		member.Weight,
		member.Position,
		member.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return ErrPoolAlreadyExists
		}
		return fmt.Errorf("failed to insert pool member: %w", err)
	}
	return nil
}

// RemoveMember removes a profile from a pool.
func (r *PoolRepository) RemoveMember(ctx context.Context, poolID, profileID string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM pool_members WHERE pool_id = ? AND profile_id = ?
	`, poolID, profileID)
	if err != nil {
		return fmt.Errorf("failed to remove pool member: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrPoolNotFound
	}
	return nil
}

// ListMembers returns members for a pool.
func (r *PoolRepository) ListMembers(ctx context.Context, poolID string) ([]*models.PoolMember, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, pool_id, profile_id, weight, position, created_at
		FROM pool_members
		WHERE pool_id = ?
		ORDER BY position, created_at
	`, poolID)
	if err != nil {
		return nil, fmt.Errorf("failed to query pool members: %w", err)
	}
	defer rows.Close()

	members := make([]*models.PoolMember, 0)
	for rows.Next() {
		member, err := r.scanPoolMember(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}

	return members, nil
}

func (r *PoolRepository) scanPool(scanner interface{ Scan(...any) error }) (*models.Pool, error) {
	var (
		id        string
		name      string
		strategy  string
		isDefault int
		metadata  sql.NullString
		createdAt string
		updatedAt string
	)

	if err := scanner.Scan(&id, &name, &strategy, &isDefault, &metadata, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPoolNotFound
		}
		return nil, fmt.Errorf("failed to scan pool: %w", err)
	}

	pool := &models.Pool{
		ID:        id,
		Name:      name,
		Strategy:  models.PoolStrategy(strategy),
		IsDefault: isDefault == 1,
	}

	if metadata.Valid && metadata.String != "" {
		_ = json.Unmarshal([]byte(metadata.String), &pool.Metadata)
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		pool.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		pool.UpdatedAt = t
	}

	return pool, nil
}

func (r *PoolRepository) scanPoolMember(scanner interface{ Scan(...any) error }) (*models.PoolMember, error) {
	var (
		id        string
		poolID    string
		profileID string
		weight    int
		position  int
		createdAt string
	)

	if err := scanner.Scan(&id, &poolID, &profileID, &weight, &position, &createdAt); err != nil {
		return nil, fmt.Errorf("failed to scan pool member: %w", err)
	}

	member := &models.PoolMember{
		ID:        id,
		PoolID:    poolID,
		ProfileID: profileID,
		Weight:    weight,
		Position:  position,
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		member.CreatedAt = t
	}

	return member, nil
}
