// Package db provides SQLite database access for Forge.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite" // Pure Go SQLite driver

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/logging"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps the SQLite database connection.
type DB struct {
	*sql.DB
	mu     sync.RWMutex
	logger zerolog.Logger
}

// Config contains database configuration.
type Config struct {
	// Path is the database file path.
	Path string

	// MaxOpenConns is the maximum number of open connections.
	MaxOpenConns int

	// BusyTimeoutMs is the busy timeout in milliseconds.
	BusyTimeoutMs int
}

// DefaultConfig returns the default database configuration.
func DefaultConfig() Config {
	return Config{
		MaxOpenConns:  10,
		BusyTimeoutMs: 5000,
	}
}

// Open opens a connection to the SQLite database.
func Open(cfg Config) (*DB, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("database path is required")
	}

	// Ensure directory exists
	_ = filepath.Dir(cfg.Path)

	// Build connection string with pragmas
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)",
		cfg.Path, cfg.BusyTimeoutMs)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{
		DB:     db,
		logger: logging.Component("db"),
	}, nil
}

// OpenInMemory opens an in-memory SQLite database (for testing).
func OpenInMemory() (*DB, error) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("failed to open in-memory database: %w", err)
	}

	// Keep a single connection open so the in-memory DB stays consistent.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	return &DB{
		DB:     db,
		logger: logging.Component("db"),
	}, nil
}

// Migrate runs all pending database migrations.
func (db *DB) Migrate(ctx context.Context) error {
	_, err := db.MigrateUp(ctx)
	return err
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}

// Transaction executes a function within a database transaction.
func (db *DB) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// HealthCheck verifies the database is accessible.
func (db *DB) HealthCheck(ctx context.Context) error {
	return db.PingContext(ctx)
}

// SchemaVersion returns the current schema version.
func (db *DB) SchemaVersion(ctx context.Context) (int, error) {
	var version int
	err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	return version, err
}
