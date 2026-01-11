package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tOgg1/forge/internal/db"
)

// NewTestDB creates an in-memory SQLite database for testing.
// It runs migrations and returns a cleanup function.
func NewTestDB(t *testing.T) (*db.DB, func()) {
	t.Helper()

	database, err := db.OpenInMemory()
	require.NoError(t, err, "failed to open test database")

	ctx := context.Background()
	err = database.Migrate(ctx)
	require.NoError(t, err, "failed to run migrations")

	cleanup := func() {
		_ = database.Close()
	}

	return database, cleanup
}

// TestDBEnv provides a complete database test environment with all repositories.
type TestDBEnv struct {
	DB            *db.DB
	AgentRepo     *db.AgentRepository
	EventRepo     *db.EventRepository
	AccountRepo   *db.AccountRepository
	WorkspaceRepo *db.WorkspaceRepository
	NodeRepo      *db.NodeRepository
	QueueRepo     *db.QueueRepository
	ApprovalRepo  *db.ApprovalRepository
	UsageRepo     *db.UsageRepository
	cleanup       func()
	t             *testing.T
}

// NewTestDBEnv creates a complete test database environment.
func NewTestDBEnv(t *testing.T) *TestDBEnv {
	t.Helper()
	database, cleanup := NewTestDB(t)

	return &TestDBEnv{
		DB:            database,
		AgentRepo:     db.NewAgentRepository(database),
		EventRepo:     db.NewEventRepository(database),
		AccountRepo:   db.NewAccountRepository(database),
		WorkspaceRepo: db.NewWorkspaceRepository(database),
		NodeRepo:      db.NewNodeRepository(database),
		QueueRepo:     db.NewQueueRepository(database),
		ApprovalRepo:  db.NewApprovalRepository(database),
		UsageRepo:     db.NewUsageRepository(database),
		cleanup:       cleanup,
		t:             t,
	}
}

// Close cleans up the test environment.
func (e *TestDBEnv) Close() {
	if e.cleanup != nil {
		e.cleanup()
	}
}
