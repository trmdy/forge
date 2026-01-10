// Package db provides SQLite database access for Forge.
package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tOgg1/forge/internal/models"
)

// Loop repository errors.
var (
	ErrLoopNotFound      = errors.New("loop not found")
	ErrLoopAlreadyExists = errors.New("loop already exists")
)

// LoopRepository handles loop persistence.
type LoopRepository struct {
	db *DB
}

// NewLoopRepository creates a new LoopRepository.
func NewLoopRepository(db *DB) *LoopRepository {
	return &LoopRepository{db: db}
}

// Create adds a new loop to the database.
func (r *LoopRepository) Create(ctx context.Context, loop *models.Loop) error {
	if loop.ID == "" {
		loop.ID = uuid.New().String()
	}
	if err := r.ensureLoopShortID(ctx, loop); err != nil {
		return err
	}
	if err := loop.Validate(); err != nil {
		return fmt.Errorf("invalid loop: %w", err)
	}

	if loop.State == "" {
		loop.State = models.DefaultLoopState()
	}

	now := time.Now().UTC()
	loop.CreatedAt = now
	loop.UpdatedAt = now

	var tagsJSON *string
	if len(loop.Tags) > 0 {
		data, err := json.Marshal(loop.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags: %w", err)
		}
		value := string(data)
		tagsJSON = &value
	}

	var metadataJSON *string
	if loop.Metadata != nil {
		data, err := json.Marshal(loop.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		value := string(data)
		metadataJSON = &value
	}

	lastRunAt := stringTimePtr(loop.LastRunAt)

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO loops (
			id, short_id, name, repo_path, base_prompt_path, base_prompt_msg,
			interval_seconds, pool_id, profile_id, state,
			last_run_at, last_exit_code, last_error,
			log_path, ledger_path, tags_json, metadata_json,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		loop.ID,
		loop.ShortID,
		loop.Name,
		loop.RepoPath,
		nullableString(loop.BasePromptPath),
		nullableString(loop.BasePromptMsg),
		loop.IntervalSeconds,
		nullableString(loop.PoolID),
		nullableString(loop.ProfileID),
		string(loop.State),
		lastRunAt,
		loop.LastExitCode,
		nullableString(loop.LastError),
		nullableString(loop.LogPath),
		nullableString(loop.LedgerPath),
		tagsJSON,
		metadataJSON,
		loop.CreatedAt.Format(time.RFC3339),
		loop.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return ErrLoopAlreadyExists
		}
		return fmt.Errorf("failed to insert loop: %w", err)
	}

	return nil
}

// Get retrieves a loop by ID.
func (r *LoopRepository) Get(ctx context.Context, id string) (*models.Loop, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			id, short_id, name, repo_path, base_prompt_path, base_prompt_msg,
			interval_seconds, pool_id, profile_id, state,
			last_run_at, last_exit_code, last_error,
			log_path, ledger_path, tags_json, metadata_json,
			created_at, updated_at
		FROM loops WHERE id = ?
	`, id)

	return r.scanLoop(row)
}

// GetByName retrieves a loop by name.
func (r *LoopRepository) GetByName(ctx context.Context, name string) (*models.Loop, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			id, short_id, name, repo_path, base_prompt_path, base_prompt_msg,
			interval_seconds, pool_id, profile_id, state,
			last_run_at, last_exit_code, last_error,
			log_path, ledger_path, tags_json, metadata_json,
			created_at, updated_at
		FROM loops WHERE name = ?
	`, name)

	return r.scanLoop(row)
}

// GetByShortID retrieves a loop by short ID.
func (r *LoopRepository) GetByShortID(ctx context.Context, shortID string) (*models.Loop, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			id, short_id, name, repo_path, base_prompt_path, base_prompt_msg,
			interval_seconds, pool_id, profile_id, state,
			last_run_at, last_exit_code, last_error,
			log_path, ledger_path, tags_json, metadata_json,
			created_at, updated_at
		FROM loops WHERE short_id = ?
	`, shortID)

	return r.scanLoop(row)
}

// List retrieves all loops.
func (r *LoopRepository) List(ctx context.Context) ([]*models.Loop, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			id, short_id, name, repo_path, base_prompt_path, base_prompt_msg,
			interval_seconds, pool_id, profile_id, state,
			last_run_at, last_exit_code, last_error,
			log_path, ledger_path, tags_json, metadata_json,
			created_at, updated_at
		FROM loops
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query loops: %w", err)
	}
	defer rows.Close()

	loops := make([]*models.Loop, 0)
	for rows.Next() {
		loop, err := r.scanLoop(rows)
		if err != nil {
			return nil, err
		}
		loops = append(loops, loop)
	}

	return loops, nil
}

// Update updates a loop.
func (r *LoopRepository) Update(ctx context.Context, loop *models.Loop) error {
	if err := r.ensureLoopShortID(ctx, loop); err != nil {
		return err
	}
	if err := loop.Validate(); err != nil {
		return fmt.Errorf("invalid loop: %w", err)
	}

	loop.UpdatedAt = time.Now().UTC()

	var tagsJSON *string
	if len(loop.Tags) > 0 {
		data, err := json.Marshal(loop.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags: %w", err)
		}
		value := string(data)
		tagsJSON = &value
	}

	var metadataJSON *string
	if loop.Metadata != nil {
		data, err := json.Marshal(loop.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		value := string(data)
		metadataJSON = &value
	}

	lastRunAt := stringTimePtr(loop.LastRunAt)

	result, err := r.db.ExecContext(ctx, `
		UPDATE loops
		SET short_id = ?, name = ?, repo_path = ?, base_prompt_path = ?, base_prompt_msg = ?,
			interval_seconds = ?, pool_id = ?, profile_id = ?, state = ?,
			last_run_at = ?, last_exit_code = ?, last_error = ?,
			log_path = ?, ledger_path = ?, tags_json = ?, metadata_json = ?,
			updated_at = ?
		WHERE id = ?
	`,
		loop.ShortID,
		loop.Name,
		loop.RepoPath,
		nullableString(loop.BasePromptPath),
		nullableString(loop.BasePromptMsg),
		loop.IntervalSeconds,
		nullableString(loop.PoolID),
		nullableString(loop.ProfileID),
		string(loop.State),
		lastRunAt,
		loop.LastExitCode,
		nullableString(loop.LastError),
		nullableString(loop.LogPath),
		nullableString(loop.LedgerPath),
		tagsJSON,
		metadataJSON,
		loop.UpdatedAt.Format(time.RFC3339),
		loop.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update loop: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrLoopNotFound
	}
	return nil
}

// Delete removes a loop.
func (r *LoopRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM loops WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete loop: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrLoopNotFound
	}
	return nil
}

func (r *LoopRepository) scanLoop(scanner interface{ Scan(...any) error }) (*models.Loop, error) {
	var (
		id              string
		shortID         sql.NullString
		name            string
		repoPath        string
		basePromptPath  sql.NullString
		basePromptMsg   sql.NullString
		intervalSeconds int
		poolID          sql.NullString
		profileID       sql.NullString
		state           string
		lastRunAt       sql.NullString
		lastExitCode    sql.NullInt64
		lastError       sql.NullString
		logPath         sql.NullString
		ledgerPath      sql.NullString
		tagsJSON        sql.NullString
		metadataJSON    sql.NullString
		createdAt       string
		updatedAt       string
	)

	if err := scanner.Scan(
		&id,
		&shortID,
		&name,
		&repoPath,
		&basePromptPath,
		&basePromptMsg,
		&intervalSeconds,
		&poolID,
		&profileID,
		&state,
		&lastRunAt,
		&lastExitCode,
		&lastError,
		&logPath,
		&ledgerPath,
		&tagsJSON,
		&metadataJSON,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrLoopNotFound
		}
		return nil, fmt.Errorf("failed to scan loop: %w", err)
	}

	loop := &models.Loop{
		ID:              id,
		ShortID:         shortID.String,
		Name:            name,
		RepoPath:        repoPath,
		BasePromptPath:  basePromptPath.String,
		BasePromptMsg:   basePromptMsg.String,
		IntervalSeconds: intervalSeconds,
		PoolID:          poolID.String,
		ProfileID:       profileID.String,
		State:           models.LoopState(state),
		LastError:       lastError.String,
		LogPath:         logPath.String,
		LedgerPath:      ledgerPath.String,
	}

	if lastRunAt.Valid && lastRunAt.String != "" {
		if t, err := time.Parse(time.RFC3339, lastRunAt.String); err == nil {
			loop.LastRunAt = &t
		}
	}
	if lastExitCode.Valid {
		exitCode := int(lastExitCode.Int64)
		loop.LastExitCode = &exitCode
	}
	if tagsJSON.Valid && tagsJSON.String != "" {
		_ = json.Unmarshal([]byte(tagsJSON.String), &loop.Tags)
	}
	if metadataJSON.Valid && metadataJSON.String != "" {
		_ = json.Unmarshal([]byte(metadataJSON.String), &loop.Metadata)
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		loop.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		loop.UpdatedAt = t
	}

	return loop, nil
}

const loopShortIDLength = 8

var loopShortIDAlphabet = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

func (r *LoopRepository) ensureLoopShortID(ctx context.Context, loop *models.Loop) error {
	if loop.ShortID != "" {
		loop.ShortID = strings.ToLower(loop.ShortID)
		return nil
	}

	for i := 0; i < 10; i++ {
		candidate, err := generateLoopShortID(loopShortIDLength)
		if err != nil {
			return fmt.Errorf("failed to generate loop short ID: %w", err)
		}
		exists, err := r.loopShortIDExists(ctx, candidate)
		if err != nil {
			return err
		}
		if !exists {
			loop.ShortID = candidate
			return nil
		}
	}

	return fmt.Errorf("failed to allocate unique loop short ID")
}

func (r *LoopRepository) loopShortIDExists(ctx context.Context, shortID string) (bool, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM loops WHERE short_id = ?`, shortID).Scan(&count); err != nil {
		return false, fmt.Errorf("failed to check loop short ID: %w", err)
	}
	return count > 0, nil
}

func generateLoopShortID(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("short ID length must be positive")
	}

	var builder strings.Builder
	builder.Grow(length)
	for i := 0; i < length; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(loopShortIDAlphabet))))
		if err != nil {
			return "", err
		}
		builder.WriteRune(loopShortIDAlphabet[idx.Int64()])
	}
	return builder.String(), nil
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
