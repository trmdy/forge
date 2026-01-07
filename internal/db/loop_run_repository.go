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

// LoopRun repository errors.
var (
	ErrLoopRunNotFound = errors.New("loop run not found")
)

// LoopRunRepository handles loop run persistence.
type LoopRunRepository struct {
	db *DB
}

// NewLoopRunRepository creates a new LoopRunRepository.
func NewLoopRunRepository(db *DB) *LoopRunRepository {
	return &LoopRunRepository{db: db}
}

// Create adds a new loop run.
func (r *LoopRunRepository) Create(ctx context.Context, run *models.LoopRun) error {
	if run.ID == "" {
		run.ID = uuid.New().String()
	}
	if run.Status == "" {
		run.Status = models.LoopRunStatusRunning
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}

	var metadataJSON *string
	if run.Metadata != nil {
		data, err := json.Marshal(run.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal run metadata: %w", err)
		}
		value := string(data)
		metadataJSON = &value
	}

	promptOverride := 0
	if run.PromptOverride {
		promptOverride = 1
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO loop_runs (
			id, loop_id, profile_id, status,
			prompt_source, prompt_path, prompt_override,
			started_at, finished_at, exit_code, output_tail, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		run.ID,
		run.LoopID,
		nullableString(run.ProfileID),
		string(run.Status),
		nullableString(run.PromptSource),
		nullableString(run.PromptPath),
		promptOverride,
		run.StartedAt.Format(time.RFC3339),
		stringTimePtr(run.FinishedAt),
		run.ExitCode,
		nullableString(run.OutputTail),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to insert loop run: %w", err)
	}

	return nil
}

// Get retrieves a loop run by ID.
func (r *LoopRunRepository) Get(ctx context.Context, id string) (*models.LoopRun, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, loop_id, profile_id, status,
			prompt_source, prompt_path, prompt_override,
			started_at, finished_at, exit_code, output_tail, metadata_json
		FROM loop_runs WHERE id = ?
	`, id)

	return r.scanLoopRun(row)
}

// ListByLoop retrieves runs for a loop.
func (r *LoopRunRepository) ListByLoop(ctx context.Context, loopID string) ([]*models.LoopRun, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, loop_id, profile_id, status,
			prompt_source, prompt_path, prompt_override,
			started_at, finished_at, exit_code, output_tail, metadata_json
		FROM loop_runs
		WHERE loop_id = ?
		ORDER BY started_at DESC
	`, loopID)
	if err != nil {
		return nil, fmt.Errorf("failed to query loop runs: %w", err)
	}
	defer rows.Close()

	runs := make([]*models.LoopRun, 0)
	for rows.Next() {
		run, err := r.scanLoopRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}

	return runs, nil
}

// CountRunningByProfile returns the number of running loop runs for a profile.
func (r *LoopRunRepository) CountRunningByProfile(ctx context.Context, profileID string) (int, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM loop_runs
		WHERE profile_id = ? AND status = ?
	`, profileID, string(models.LoopRunStatusRunning))

	var count int
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count running loop runs: %w", err)
	}
	return count, nil
}

// Finish updates a loop run with completion details.
func (r *LoopRunRepository) Finish(ctx context.Context, run *models.LoopRun) error {
	finishedAt := time.Now().UTC()
	run.FinishedAt = &finishedAt
	result, err := r.db.ExecContext(ctx, `
		UPDATE loop_runs
		SET status = ?, finished_at = ?, exit_code = ?, output_tail = ?
		WHERE id = ?
	`,
		string(run.Status),
		stringTimePtr(run.FinishedAt),
		run.ExitCode,
		nullableString(run.OutputTail),
		run.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update loop run: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrLoopRunNotFound
	}
	return nil
}

func (r *LoopRunRepository) scanLoopRun(scanner interface{ Scan(...any) error }) (*models.LoopRun, error) {
	var (
		id             string
		loopID         string
		profileID      sql.NullString
		status         string
		promptSource   sql.NullString
		promptPath     sql.NullString
		promptOverride int
		startedAt      string
		finishedAt     sql.NullString
		exitCode       sql.NullInt64
		outputTail     sql.NullString
		metadataJSON   sql.NullString
	)

	if err := scanner.Scan(
		&id,
		&loopID,
		&profileID,
		&status,
		&promptSource,
		&promptPath,
		&promptOverride,
		&startedAt,
		&finishedAt,
		&exitCode,
		&outputTail,
		&metadataJSON,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrLoopRunNotFound
		}
		return nil, fmt.Errorf("failed to scan loop run: %w", err)
	}

	run := &models.LoopRun{
		ID:             id,
		LoopID:         loopID,
		ProfileID:      profileID.String,
		Status:         models.LoopRunStatus(status),
		PromptSource:   promptSource.String,
		PromptPath:     promptPath.String,
		PromptOverride: promptOverride == 1,
		OutputTail:     outputTail.String,
	}

	if t, err := time.Parse(time.RFC3339, startedAt); err == nil {
		run.StartedAt = t
	}
	if finishedAt.Valid && finishedAt.String != "" {
		if t, err := time.Parse(time.RFC3339, finishedAt.String); err == nil {
			run.FinishedAt = &t
		}
	}
	if exitCode.Valid {
		code := int(exitCode.Int64)
		run.ExitCode = &code
	}
	if metadataJSON.Valid && metadataJSON.String != "" {
		_ = json.Unmarshal([]byte(metadataJSON.String), &run.Metadata)
	}

	return run, nil
}
