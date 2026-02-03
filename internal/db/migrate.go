// Package db provides database migration functionality.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Migration represents a single database migration.
type Migration struct {
	Version     int
	Description string
	UpSQL       string
	DownSQL     string
}

// MigrationStatus represents the status of a migration.
type MigrationStatus struct {
	Version     int
	Description string
	Applied     bool
	AppliedAt   string
}

// migrationFilePattern matches migration filenames like "001_initial_schema.up.sql"
var migrationFilePattern = regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.sql$`)

// loadMigrations reads all migrations from the embedded filesystem.
func loadMigrations() ([]Migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Group files by version
	type migrationFiles struct {
		version     int
		description string
		upFile      string
		downFile    string
	}
	filesByVersion := make(map[int]*migrationFiles)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := migrationFilePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}

		version, _ := strconv.Atoi(matches[1])
		description := strings.ReplaceAll(matches[2], "_", " ")
		direction := matches[3]

		if filesByVersion[version] == nil {
			filesByVersion[version] = &migrationFiles{
				version:     version,
				description: description,
			}
		}

		if direction == "up" {
			filesByVersion[version].upFile = entry.Name()
		} else {
			filesByVersion[version].downFile = entry.Name()
		}
	}

	// Convert to sorted slice
	var migrations []Migration
	for _, files := range filesByVersion {
		m := Migration{
			Version:     files.version,
			Description: files.description,
		}

		if files.upFile != "" {
			content, err := fs.ReadFile(migrationsFS, "migrations/"+files.upFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read %s: %w", files.upFile, err)
			}
			m.UpSQL = string(content)
		}

		if files.downFile != "" {
			content, err := fs.ReadFile(migrationsFS, "migrations/"+files.downFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read %s: %w", files.downFile, err)
			}
			m.DownSQL = string(content)
		}

		migrations = append(migrations, m)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// MigrateUp applies all pending migrations.
func (db *DB) MigrateUp(ctx context.Context) (int, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if err := db.ensureSchemaVersionTable(ctx); err != nil {
		return 0, err
	}

	migrations, err := loadMigrations()
	if err != nil {
		return 0, err
	}

	currentVersion, err := db.getCurrentVersion(ctx)
	if err != nil {
		return 0, err
	}

	applied := 0
	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}

		if m.UpSQL == "" {
			return applied, fmt.Errorf("migration %d has no up SQL", m.Version)
		}

		if err := db.applyMigrationTx(ctx, m.Version, m.Description, m.UpSQL); err != nil {
			return applied, fmt.Errorf("migration %d failed: %w", m.Version, err)
		}

		db.logger.Info().
			Int("version", m.Version).
			Str("description", m.Description).
			Msg("applied migration")
		applied++
	}

	return applied, nil
}

// MigrateDown rolls back the last n migrations.
func (db *DB) MigrateDown(ctx context.Context, steps int) (int, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if err := db.ensureSchemaVersionTable(ctx); err != nil {
		return 0, err
	}

	migrations, err := loadMigrations()
	if err != nil {
		return 0, err
	}

	currentVersion, err := db.getCurrentVersion(ctx)
	if err != nil {
		return 0, err
	}

	if currentVersion == 0 {
		return 0, nil // Nothing to roll back
	}

	// Find migrations to roll back (in reverse order)
	var toRollback []Migration
	for i := len(migrations) - 1; i >= 0 && len(toRollback) < steps; i-- {
		if migrations[i].Version <= currentVersion {
			toRollback = append(toRollback, migrations[i])
		}
	}

	rolledBack := 0
	for _, m := range toRollback {
		if m.DownSQL == "" {
			return rolledBack, fmt.Errorf("migration %d has no down SQL", m.Version)
		}

		if err := db.rollbackMigrationTx(ctx, m.Version, m.DownSQL); err != nil {
			return rolledBack, fmt.Errorf("rollback of migration %d failed: %w", m.Version, err)
		}

		db.logger.Info().
			Int("version", m.Version).
			Str("description", m.Description).
			Msg("rolled back migration")
		rolledBack++
	}

	return rolledBack, nil
}

// MigrateTo migrates to a specific version.
func (db *DB) MigrateTo(ctx context.Context, targetVersion int) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if err := db.ensureSchemaVersionTable(ctx); err != nil {
		return err
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	currentVersion, err := db.getCurrentVersion(ctx)
	if err != nil {
		return err
	}

	if targetVersion == currentVersion {
		return nil
	}

	if targetVersion > currentVersion {
		// Migrate up
		for _, m := range migrations {
			if m.Version <= currentVersion || m.Version > targetVersion {
				continue
			}

			if err := db.applyMigrationTx(ctx, m.Version, m.Description, m.UpSQL); err != nil {
				return fmt.Errorf("migration %d failed: %w", m.Version, err)
			}

			db.logger.Info().
				Int("version", m.Version).
				Str("description", m.Description).
				Msg("applied migration")
		}
	} else {
		// Migrate down
		for i := len(migrations) - 1; i >= 0; i-- {
			m := migrations[i]
			if m.Version <= targetVersion || m.Version > currentVersion {
				continue
			}

			if err := db.rollbackMigrationTx(ctx, m.Version, m.DownSQL); err != nil {
				return fmt.Errorf("rollback of migration %d failed: %w", m.Version, err)
			}

			db.logger.Info().
				Int("version", m.Version).
				Str("description", m.Description).
				Msg("rolled back migration")
		}
	}

	return nil
}

// MigrationStatus returns the status of all migrations.
func (db *DB) MigrationStatus(ctx context.Context) ([]MigrationStatus, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if err := db.ensureSchemaVersionTable(ctx); err != nil {
		return nil, err
	}

	migrations, err := loadMigrations()
	if err != nil {
		return nil, err
	}

	// Get applied versions
	rows, err := db.QueryContext(ctx, "SELECT version, applied_at FROM schema_version ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("failed to query schema_version: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]string)
	for rows.Next() {
		var version int
		var appliedAt string
		if err := rows.Scan(&version, &appliedAt); err != nil {
			return nil, fmt.Errorf("failed to scan schema_version row: %w", err)
		}
		applied[version] = appliedAt
	}

	var status []MigrationStatus
	for _, m := range migrations {
		s := MigrationStatus{
			Version:     m.Version,
			Description: m.Description,
		}
		if appliedAt, ok := applied[m.Version]; ok {
			s.Applied = true
			s.AppliedAt = appliedAt
		}
		status = append(status, s)
	}

	return status, nil
}

// ensureSchemaVersionTable creates the schema_version table if it doesn't exist.
func (db *DB) ensureSchemaVersionTable(ctx context.Context) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now')),
			description TEXT
		)
	`)
	return err
}

// getCurrentVersion returns the current schema version.
func (db *DB) getCurrentVersion(ctx context.Context) (int, error) {
	var version int
	err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	return version, err
}

// applyMigrationTx applies a migration in a transaction.
func (db *DB) applyMigrationTx(ctx context.Context, version int, description, sql string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Execute migration SQL
	if _, err := tx.ExecContext(ctx, sql); err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record migration
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_version (version, description) VALUES (?, ?)",
		version, description); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return tx.Commit()
}

// rollbackMigrationTx rolls back a migration in a transaction.
func (db *DB) rollbackMigrationTx(ctx context.Context, version int, sql string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Execute rollback SQL
	if _, err := tx.ExecContext(ctx, sql); err != nil {
		return fmt.Errorf("failed to execute rollback SQL: %w", err)
	}

	// Remove migration record
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM schema_version WHERE version = ?", version); err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	return tx.Commit()
}

// CreateMigration generates new migration file templates.
func CreateMigration(name string) (upPath, downPath string, err error) {
	// This would normally write to the filesystem, but since we use embed.FS,
	// this is mainly for documentation. In practice, developers create these manually.
	return "", "", fmt.Errorf("CreateMigration: please create migration files manually in internal/db/migrations/")
}

// Tx is a transaction handle returned by BeginTx.
type Tx struct {
	*sql.Tx
}

// BeginTransaction starts a new database transaction.
func (db *DB) BeginTransaction(ctx context.Context) (*Tx, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Tx{tx}, nil
}
