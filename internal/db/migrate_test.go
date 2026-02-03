package db

import (
	"context"
	"testing"
)

func TestMigrateUp(t *testing.T) {
	ctx := context.Background()

	database, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer database.Close()

	// Apply all migrations
	applied, err := database.MigrateUp(ctx)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	if applied == 0 {
		t.Error("expected at least one migration to be applied")
	}

	// Check version
	version, err := database.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations failed: %v", err)
	}
	maxVersion := 0
	for _, migration := range migrations {
		if migration.Version > maxVersion {
			maxVersion = migration.Version
		}
	}
	if version != maxVersion {
		t.Errorf("expected version %d, got %d", maxVersion, version)
	}

	// Run again - should be idempotent
	applied, err = database.MigrateUp(ctx)
	if err != nil {
		t.Fatalf("second MigrateUp failed: %v", err)
	}

	if applied != 0 {
		t.Errorf("expected 0 migrations on second run, got %d", applied)
	}
}

func TestMigrateDown(t *testing.T) {
	ctx := context.Background()

	database, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer database.Close()

	// Apply migrations first
	_, err = database.MigrateUp(ctx)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Roll back
	rolledBack, err := database.MigrateDown(ctx, 1)
	if err != nil {
		t.Fatalf("MigrateDown failed: %v", err)
	}

	if rolledBack != 1 {
		t.Errorf("expected 1 migration rolled back, got %d", rolledBack)
	}

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations failed: %v", err)
	}
	maxVersion := 0
	for _, migration := range migrations {
		if migration.Version > maxVersion {
			maxVersion = migration.Version
		}
	}

	// Check version
	version, err := database.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}

	expectedVersion := maxVersion - 1
	if expectedVersion < 0 {
		expectedVersion = 0
	}
	if version != expectedVersion {
		t.Errorf("expected version %d after rollback, got %d", expectedVersion, version)
	}
}

func TestMigrateTo(t *testing.T) {
	ctx := context.Background()

	database, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer database.Close()

	// Migrate to version 1
	err = database.MigrateTo(ctx, 1)
	if err != nil {
		t.Fatalf("MigrateTo(1) failed: %v", err)
	}

	version, err := database.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}

	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}

	// Migrate back to 0
	err = database.MigrateTo(ctx, 0)
	if err != nil {
		t.Fatalf("MigrateTo(0) failed: %v", err)
	}

	version, err = database.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}

	if version != 0 {
		t.Errorf("expected version 0, got %d", version)
	}
}

func TestMigrationStatus(t *testing.T) {
	ctx := context.Background()

	database, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer database.Close()

	// Get status before any migrations
	status, err := database.MigrationStatus(ctx)
	if err != nil {
		t.Fatalf("MigrationStatus failed: %v", err)
	}

	if len(status) == 0 {
		t.Error("expected at least one migration in status")
	}

	// All should be pending
	for _, s := range status {
		if s.Applied {
			t.Errorf("migration %d should not be applied yet", s.Version)
		}
	}

	// Apply migrations
	_, err = database.MigrateUp(ctx)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Get status after migration
	status, err = database.MigrationStatus(ctx)
	if err != nil {
		t.Fatalf("MigrationStatus failed: %v", err)
	}

	// All should be applied
	for _, s := range status {
		if !s.Applied {
			t.Errorf("migration %d should be applied", s.Version)
		}
		if s.AppliedAt == "" {
			t.Errorf("migration %d should have applied_at timestamp", s.Version)
		}
	}
}

func TestMigrateCreatesSchemaVersionTable(t *testing.T) {
	ctx := context.Background()

	database, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer database.Close()

	// Calling SchemaVersion should create the table (may fail pre-migration).
	_, _ = database.SchemaVersion(ctx)

	// After MigrateUp, table should definitely exist
	_, err = database.MigrateUp(ctx)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	version, err := database.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion failed after migration: %v", err)
	}

	if version < 1 {
		t.Errorf("expected version >= 1, got %d", version)
	}
}

func TestMigrateTablesCreated(t *testing.T) {
	ctx := context.Background()

	database, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer database.Close()

	// Apply migrations
	_, err = database.MigrateUp(ctx)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Check that core tables exist
	tables := []string{"nodes", "workspaces", "agents", "accounts", "queue_items", "events", "alerts", "transcripts", "approvals"}

	for _, table := range tables {
		var count int
		err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Fatalf("failed to check table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s should exist", table)
		}
	}
}
