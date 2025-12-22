package db

import (
	"context"
	"testing"
	"time"

	"github.com/opencode-ai/swarm/internal/models"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func TestNodeRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewNodeRepository(db)
	ctx := context.Background()

	node := &models.Node{
		Name:       "test-node",
		SSHTarget:  "user@host.example.com",
		SSHBackend: models.SSHBackendAuto,
		Status:     models.NodeStatusUnknown,
	}

	err := repo.Create(ctx, node)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if node.ID == "" {
		t.Error("expected node ID to be set")
	}
	if node.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestNodeRepository_Get(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewNodeRepository(db)
	ctx := context.Background()

	// Create a node
	node := &models.Node{
		Name:               "test-node",
		SSHTarget:          "user@host.example.com",
		SSHBackend:         models.SSHBackendAuto,
		SSHAgentForwarding: true,
		SSHProxyJump:       "jump.example.com",
		SSHControlMaster:   "auto",
		SSHControlPath:     "/tmp/ssh-%r@%h:%p",
		SSHControlPersist:  "10m",
		SSHTimeoutSeconds:  45,
		Status:             models.NodeStatusOnline,
		Metadata: models.NodeMetadata{
			TmuxVersion: "3.3",
			Platform:    "linux",
		},
	}

	if err := repo.Create(ctx, node); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Retrieve it
	retrieved, err := repo.Get(ctx, node.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Name != node.Name {
		t.Errorf("expected name %q, got %q", node.Name, retrieved.Name)
	}
	if retrieved.SSHTarget != node.SSHTarget {
		t.Errorf("expected ssh_target %q, got %q", node.SSHTarget, retrieved.SSHTarget)
	}
	if retrieved.Status != node.Status {
		t.Errorf("expected status %q, got %q", node.Status, retrieved.Status)
	}
	if retrieved.SSHAgentForwarding != node.SSHAgentForwarding {
		t.Errorf("expected agent forwarding %v, got %v", node.SSHAgentForwarding, retrieved.SSHAgentForwarding)
	}
	if retrieved.SSHProxyJump != node.SSHProxyJump {
		t.Errorf("expected proxy jump %q, got %q", node.SSHProxyJump, retrieved.SSHProxyJump)
	}
	if retrieved.SSHControlMaster != node.SSHControlMaster {
		t.Errorf("expected control master %q, got %q", node.SSHControlMaster, retrieved.SSHControlMaster)
	}
	if retrieved.SSHControlPath != node.SSHControlPath {
		t.Errorf("expected control path %q, got %q", node.SSHControlPath, retrieved.SSHControlPath)
	}
	if retrieved.SSHControlPersist != node.SSHControlPersist {
		t.Errorf("expected control persist %q, got %q", node.SSHControlPersist, retrieved.SSHControlPersist)
	}
	if retrieved.SSHTimeoutSeconds != node.SSHTimeoutSeconds {
		t.Errorf("expected timeout seconds %d, got %d", node.SSHTimeoutSeconds, retrieved.SSHTimeoutSeconds)
	}
	if retrieved.Metadata.TmuxVersion != node.Metadata.TmuxVersion {
		t.Errorf("expected tmux_version %q, got %q", node.Metadata.TmuxVersion, retrieved.Metadata.TmuxVersion)
	}
}

func TestNodeRepository_GetByName(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewNodeRepository(db)
	ctx := context.Background()

	node := &models.Node{
		Name:       "named-node",
		SSHTarget:  "user@host.example.com",
		SSHBackend: models.SSHBackendSystem,
		Status:     models.NodeStatusUnknown,
	}

	if err := repo.Create(ctx, node); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := repo.GetByName(ctx, "named-node")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}

	if retrieved.ID != node.ID {
		t.Errorf("expected ID %q, got %q", node.ID, retrieved.ID)
	}
}

func TestNodeRepository_List(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewNodeRepository(db)
	ctx := context.Background()

	// Create multiple nodes
	nodes := []*models.Node{
		{Name: "node-1", SSHTarget: "user@host1", SSHBackend: models.SSHBackendAuto, Status: models.NodeStatusOnline},
		{Name: "node-2", SSHTarget: "user@host2", SSHBackend: models.SSHBackendAuto, Status: models.NodeStatusOffline},
		{Name: "node-3", SSHTarget: "user@host3", SSHBackend: models.SSHBackendAuto, Status: models.NodeStatusOnline},
	}

	for _, n := range nodes {
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List all
	all, err := repo.List(ctx, nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(all))
	}

	// List by status
	online := models.NodeStatusOnline
	onlineNodes, err := repo.List(ctx, &online)
	if err != nil {
		t.Fatalf("List with status failed: %v", err)
	}
	if len(onlineNodes) != 2 {
		t.Errorf("expected 2 online nodes, got %d", len(onlineNodes))
	}
}

func TestNodeRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewNodeRepository(db)
	ctx := context.Background()

	node := &models.Node{
		Name:       "update-node",
		SSHTarget:  "user@host.example.com",
		SSHBackend: models.SSHBackendAuto,
		Status:     models.NodeStatusUnknown,
	}

	if err := repo.Create(ctx, node); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update the node
	node.Status = models.NodeStatusOnline
	node.Metadata = models.NodeMetadata{
		TmuxVersion: "3.4",
		Platform:    "darwin",
	}

	if err := repo.Update(ctx, node); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update
	retrieved, err := repo.Get(ctx, node.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Status != models.NodeStatusOnline {
		t.Errorf("expected status %q, got %q", models.NodeStatusOnline, retrieved.Status)
	}
	if retrieved.Metadata.TmuxVersion != "3.4" {
		t.Errorf("expected tmux_version %q, got %q", "3.4", retrieved.Metadata.TmuxVersion)
	}
}

func TestNodeRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewNodeRepository(db)
	ctx := context.Background()

	node := &models.Node{
		Name:       "delete-node",
		SSHTarget:  "user@host.example.com",
		SSHBackend: models.SSHBackendAuto,
		Status:     models.NodeStatusUnknown,
	}

	if err := repo.Create(ctx, node); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := repo.Delete(ctx, node.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := repo.Get(ctx, node.ID)
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestNodeRepository_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewNodeRepository(db)
	ctx := context.Background()

	node := &models.Node{
		Name:       "status-node",
		SSHTarget:  "user@host.example.com",
		SSHBackend: models.SSHBackendAuto,
		Status:     models.NodeStatusUnknown,
	}

	if err := repo.Create(ctx, node); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update status
	if err := repo.UpdateStatus(ctx, node.ID, models.NodeStatusOnline); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	retrieved, err := repo.Get(ctx, node.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Status != models.NodeStatusOnline {
		t.Errorf("expected status %q, got %q", models.NodeStatusOnline, retrieved.Status)
	}
	if retrieved.LastSeen == nil {
		t.Error("expected LastSeen to be set")
	}
}

func TestNodeRepository_DuplicateName(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewNodeRepository(db)
	ctx := context.Background()

	node1 := &models.Node{
		Name:       "duplicate-name",
		SSHTarget:  "user@host1",
		SSHBackend: models.SSHBackendAuto,
		Status:     models.NodeStatusUnknown,
	}

	if err := repo.Create(ctx, node1); err != nil {
		t.Fatalf("Create first node failed: %v", err)
	}

	node2 := &models.Node{
		Name:       "duplicate-name",
		SSHTarget:  "user@host2",
		SSHBackend: models.SSHBackendAuto,
		Status:     models.NodeStatusUnknown,
	}

	err := repo.Create(ctx, node2)
	if err != ErrNodeAlreadyExists {
		t.Errorf("expected ErrNodeAlreadyExists, got %v", err)
	}
}

func TestNodeRepository_LocalNode(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewNodeRepository(db)
	ctx := context.Background()

	node := &models.Node{
		Name:       "local-test-node-unique-xyz123",
		IsLocal:    true,
		SSHBackend: models.SSHBackendAuto, // Must set a valid backend
		Status:     models.NodeStatusOnline,
	}

	if err := repo.Create(ctx, node); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := repo.Get(ctx, node.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !retrieved.IsLocal {
		t.Error("expected IsLocal to be true")
	}
}

func TestNodeRepository_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewNodeRepository(db)
	ctx := context.Background()

	_, err := repo.Get(ctx, "nonexistent-id")
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}

	_, err = repo.GetByName(ctx, "nonexistent-name")
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}

	err = repo.Delete(ctx, "nonexistent-id")
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}

	err = repo.UpdateStatus(ctx, "nonexistent-id", models.NodeStatusOnline)
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestNodeRepository_LastSeen(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewNodeRepository(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	node := &models.Node{
		Name:       "lastseen-node",
		SSHTarget:  "user@host",
		SSHBackend: models.SSHBackendAuto,
		Status:     models.NodeStatusOnline,
		LastSeen:   &now,
	}

	if err := repo.Create(ctx, node); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := repo.Get(ctx, node.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.LastSeen == nil {
		t.Fatal("expected LastSeen to be set")
	}

	// Compare timestamps (truncate to second to avoid precision issues)
	if !retrieved.LastSeen.Truncate(time.Second).Equal(now) {
		t.Errorf("expected LastSeen %v, got %v", now, *retrieved.LastSeen)
	}
}
