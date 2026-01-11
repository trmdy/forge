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

// Profile repository errors.
var (
	ErrProfileNotFound      = errors.New("profile not found")
	ErrProfileAlreadyExists = errors.New("profile already exists")
)

// ProfileRepository handles profile persistence.
type ProfileRepository struct {
	db *DB
}

// NewProfileRepository creates a new ProfileRepository.
func NewProfileRepository(db *DB) *ProfileRepository {
	return &ProfileRepository{db: db}
}

// Create adds a new profile to the database.
func (r *ProfileRepository) Create(ctx context.Context, profile *models.Profile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("invalid profile: %w", err)
	}

	if profile.ID == "" {
		profile.ID = uuid.New().String()
	}

	if profile.PromptMode == "" {
		profile.PromptMode = models.DefaultPromptMode()
	}

	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now

	var extraArgsJSON *string
	if len(profile.ExtraArgs) > 0 {
		data, err := json.Marshal(profile.ExtraArgs)
		if err != nil {
			return fmt.Errorf("failed to marshal extra args: %w", err)
		}
		value := string(data)
		extraArgsJSON = &value
	}

	var envJSON *string
	if len(profile.Env) > 0 {
		data, err := json.Marshal(profile.Env)
		if err != nil {
			return fmt.Errorf("failed to marshal env: %w", err)
		}
		value := string(data)
		envJSON = &value
	}

	cooldownUntil := stringTimePtr(profile.CooldownUntil)

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO profiles (
			id, name, harness, auth_kind, auth_home,
			prompt_mode, command_template, model,
			extra_args_json, env_json, max_concurrency,
			cooldown_until, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		profile.ID,
		profile.Name,
		string(profile.Harness),
		profile.AuthKind,
		profile.AuthHome,
		string(profile.PromptMode),
		profile.CommandTemplate,
		profile.Model,
		extraArgsJSON,
		envJSON,
		profile.MaxConcurrency,
		cooldownUntil,
		profile.CreatedAt.Format(time.RFC3339),
		profile.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return ErrProfileAlreadyExists
		}
		return fmt.Errorf("failed to insert profile: %w", err)
	}

	return nil
}

// Get retrieves a profile by ID.
func (r *ProfileRepository) Get(ctx context.Context, id string) (*models.Profile, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			id, name, harness, auth_kind, auth_home,
			prompt_mode, command_template, model,
			extra_args_json, env_json, max_concurrency,
			cooldown_until, created_at, updated_at
		FROM profiles WHERE id = ?
	`, id)

	return r.scanProfile(row)
}

// GetByName retrieves a profile by name.
func (r *ProfileRepository) GetByName(ctx context.Context, name string) (*models.Profile, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			id, name, harness, auth_kind, auth_home,
			prompt_mode, command_template, model,
			extra_args_json, env_json, max_concurrency,
			cooldown_until, created_at, updated_at
		FROM profiles WHERE name = ?
	`, name)

	return r.scanProfile(row)
}

// List retrieves all profiles.
func (r *ProfileRepository) List(ctx context.Context) ([]*models.Profile, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			id, name, harness, auth_kind, auth_home,
			prompt_mode, command_template, model,
			extra_args_json, env_json, max_concurrency,
			cooldown_until, created_at, updated_at
		FROM profiles
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query profiles: %w", err)
	}
	defer rows.Close()

	profiles := make([]*models.Profile, 0)
	for rows.Next() {
		profile, err := r.scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}

	return profiles, nil
}

// Update updates a profile.
func (r *ProfileRepository) Update(ctx context.Context, profile *models.Profile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("invalid profile: %w", err)
	}

	profile.UpdatedAt = time.Now().UTC()

	var extraArgsJSON *string
	if len(profile.ExtraArgs) > 0 {
		data, err := json.Marshal(profile.ExtraArgs)
		if err != nil {
			return fmt.Errorf("failed to marshal extra args: %w", err)
		}
		value := string(data)
		extraArgsJSON = &value
	}

	var envJSON *string
	if len(profile.Env) > 0 {
		data, err := json.Marshal(profile.Env)
		if err != nil {
			return fmt.Errorf("failed to marshal env: %w", err)
		}
		value := string(data)
		envJSON = &value
	}

	cooldownUntil := stringTimePtr(profile.CooldownUntil)

	result, err := r.db.ExecContext(ctx, `
		UPDATE profiles
		SET name = ?, harness = ?, auth_kind = ?, auth_home = ?,
			prompt_mode = ?, command_template = ?, model = ?,
			extra_args_json = ?, env_json = ?, max_concurrency = ?,
			cooldown_until = ?, updated_at = ?
		WHERE id = ?
	`,
		profile.Name,
		string(profile.Harness),
		profile.AuthKind,
		profile.AuthHome,
		string(profile.PromptMode),
		profile.CommandTemplate,
		profile.Model,
		extraArgsJSON,
		envJSON,
		profile.MaxConcurrency,
		cooldownUntil,
		profile.UpdatedAt.Format(time.RFC3339),
		profile.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrProfileNotFound
	}

	return nil
}

// Delete removes a profile.
func (r *ProfileRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM profiles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrProfileNotFound
	}
	return nil
}

// SetCooldown sets cooldown_until for a profile.
func (r *ProfileRepository) SetCooldown(ctx context.Context, id string, until *time.Time) error {
	updatedAt := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE profiles
		SET cooldown_until = ?, updated_at = ?
		WHERE id = ?
	`, stringTimePtr(until), updatedAt.Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("failed to set cooldown: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrProfileNotFound
	}
	return nil
}

func (r *ProfileRepository) scanProfile(scanner interface {
	Scan(...any) error
}) (*models.Profile, error) {
	var (
		id              string
		name            string
		harness         string
		authKind        sql.NullString
		authHome        sql.NullString
		promptMode      string
		commandTemplate string
		model           sql.NullString
		extraArgsJSON   sql.NullString
		envJSON         sql.NullString
		maxConcurrency  int
		cooldownUntil   sql.NullString
		createdAt       string
		updatedAt       string
	)

	if err := scanner.Scan(
		&id,
		&name,
		&harness,
		&authKind,
		&authHome,
		&promptMode,
		&commandTemplate,
		&model,
		&extraArgsJSON,
		&envJSON,
		&maxConcurrency,
		&cooldownUntil,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProfileNotFound
		}
		return nil, fmt.Errorf("failed to scan profile: %w", err)
	}

	profile := &models.Profile{
		ID:              id,
		Name:            name,
		Harness:         models.Harness(harness),
		AuthKind:        authKind.String,
		AuthHome:        authHome.String,
		PromptMode:      models.PromptMode(promptMode),
		CommandTemplate: commandTemplate,
		Model:           model.String,
		MaxConcurrency:  maxConcurrency,
	}

	if extraArgsJSON.Valid && extraArgsJSON.String != "" {
		_ = json.Unmarshal([]byte(extraArgsJSON.String), &profile.ExtraArgs)
	}
	if envJSON.Valid && envJSON.String != "" {
		_ = json.Unmarshal([]byte(envJSON.String), &profile.Env)
	}
	if cooldownUntil.Valid && cooldownUntil.String != "" {
		if t, err := time.Parse(time.RFC3339, cooldownUntil.String); err == nil {
			profile.CooldownUntil = &t
		}
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		profile.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		profile.UpdatedAt = t
	}

	return profile, nil
}
