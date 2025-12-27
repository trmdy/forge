package db

import (
	"context"
	"testing"

	"github.com/opencode-ai/swarm/internal/models"
)

func createTestNode(t *testing.T, db *DB) *models.Node {
	t.Helper()
	repo := NewNodeRepository(db)
	node := &models.Node{
		Name:       "test-node",
		SSHBackend: models.SSHBackendAuto,
		Status:     models.NodeStatusUnknown,
		IsLocal:    true,
	}
	if err := repo.Create(context.Background(), node); err != nil {
		t.Fatalf("create node: %v", err)
	}
	return node
}

func createTestAgentForPort(t *testing.T, db *DB, ws *models.Workspace) *models.Agent {
	t.Helper()
	repo := NewAgentRepository(db)
	agent := &models.Agent{
		WorkspaceID: ws.ID,
		Type:        models.AgentTypeOpenCode,
		TmuxPane:    "swarm-test:0.1",
		State:       models.AgentStateIdle,
	}
	if err := repo.Create(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func TestPortRepository_Allocate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	node := createTestNode(t, db)
	ws := createTestWorkspace(t, db)
	agent := createTestAgentForPort(t, db, ws)
	repo := NewPortRepository(db)
	ctx := context.Background()

	// Allocate a port
	port, err := repo.Allocate(ctx, node.ID, agent.ID, "test allocation")
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	if port < DefaultPortRangeStart || port > DefaultPortRangeEnd {
		t.Errorf("port %d outside expected range %d-%d", port, DefaultPortRangeStart, DefaultPortRangeEnd)
	}

	// First allocation should get the first port in range
	if port != DefaultPortRangeStart {
		t.Errorf("expected first port %d, got %d", DefaultPortRangeStart, port)
	}

	// Verify allocation can be retrieved
	alloc, err := repo.GetByAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByAgent failed: %v", err)
	}
	if alloc.Port != port {
		t.Errorf("expected port %d, got %d", port, alloc.Port)
	}
	if *alloc.AgentID != agent.ID {
		t.Errorf("expected agent ID %s, got %s", agent.ID, *alloc.AgentID)
	}
}

func TestPortRepository_AllocateMultiple(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	node := createTestNode(t, db)
	repo := NewPortRepository(db)
	ctx := context.Background()

	// Allocate multiple ports
	port1, err := repo.Allocate(ctx, node.ID, "", "first")
	if err != nil {
		t.Fatalf("first Allocate failed: %v", err)
	}

	port2, err := repo.Allocate(ctx, node.ID, "", "second")
	if err != nil {
		t.Fatalf("second Allocate failed: %v", err)
	}

	port3, err := repo.Allocate(ctx, node.ID, "", "third")
	if err != nil {
		t.Fatalf("third Allocate failed: %v", err)
	}

	// Ports should be sequential
	if port2 != port1+1 {
		t.Errorf("expected sequential ports: %d, %d", port1, port2)
	}
	if port3 != port2+1 {
		t.Errorf("expected sequential ports: %d, %d", port2, port3)
	}
}

func TestPortRepository_AllocateSpecific(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	node := createTestNode(t, db)
	repo := NewPortRepository(db)
	ctx := context.Background()

	specificPort := 17500

	// Allocate specific port
	err := repo.AllocateSpecific(ctx, node.ID, specificPort, "", "specific test")
	if err != nil {
		t.Fatalf("AllocateSpecific failed: %v", err)
	}

	// Try to allocate same port again - should fail
	err = repo.AllocateSpecific(ctx, node.ID, specificPort, "", "duplicate")
	if err != ErrPortAlreadyAllocated {
		t.Errorf("expected ErrPortAlreadyAllocated, got: %v", err)
	}

	// Try to allocate port outside range - should fail
	err = repo.AllocateSpecific(ctx, node.ID, 10000, "", "out of range")
	if err == nil {
		t.Error("expected error for out-of-range port")
	}
}

func TestPortRepository_Release(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	node := createTestNode(t, db)
	repo := NewPortRepository(db)
	ctx := context.Background()

	// Allocate a port
	port, err := repo.Allocate(ctx, node.ID, "", "to release")
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	// Release it
	err = repo.Release(ctx, node.ID, port)
	if err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Releasing again should fail
	err = repo.Release(ctx, node.ID, port)
	if err != ErrPortNotAllocated {
		t.Errorf("expected ErrPortNotAllocated, got: %v", err)
	}

	// Port should be available for reallocation
	port2, err := repo.Allocate(ctx, node.ID, "", "reuse")
	if err != nil {
		t.Fatalf("Allocate after release failed: %v", err)
	}
	if port2 != port {
		t.Errorf("expected to reuse port %d, got %d", port, port2)
	}
}

func TestPortRepository_ReleaseByAgent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	node := createTestNode(t, db)
	ws := createTestWorkspace(t, db)
	agent := createTestAgentForPort(t, db, ws)
	repo := NewPortRepository(db)
	ctx := context.Background()

	// Allocate ports for agent
	_, err := repo.Allocate(ctx, node.ID, agent.ID, "agent port 1")
	if err != nil {
		t.Fatalf("first Allocate failed: %v", err)
	}

	// Release all ports for agent
	count, err := repo.ReleaseByAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("ReleaseByAgent failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 released, got %d", count)
	}

	// Verify no active allocation for agent
	_, err = repo.GetByAgent(ctx, agent.ID)
	if err != ErrPortNotAllocated {
		t.Errorf("expected ErrPortNotAllocated, got: %v", err)
	}
}

func TestPortRepository_ListActiveByNode(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	node := createTestNode(t, db)
	repo := NewPortRepository(db)
	ctx := context.Background()

	// Allocate some ports
	port1, _ := repo.Allocate(ctx, node.ID, "", "first")
	port2, _ := repo.Allocate(ctx, node.ID, "", "second")
	port3, _ := repo.Allocate(ctx, node.ID, "", "third")

	// Release one
	_ = repo.Release(ctx, node.ID, port2)

	// List active
	active, err := repo.ListActiveByNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("ListActiveByNode failed: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("expected 2 active allocations, got %d", len(active))
	}

	// Verify the right ports are active
	ports := make(map[int]bool)
	for _, a := range active {
		ports[a.Port] = true
	}
	if !ports[port1] || !ports[port3] {
		t.Errorf("expected ports %d and %d to be active", port1, port3)
	}
	if ports[port2] {
		t.Error("port2 should not be active")
	}
}

func TestPortRepository_IsPortAvailable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	node := createTestNode(t, db)
	repo := NewPortRepository(db)
	ctx := context.Background()

	testPort := 17100

	// Port should be available initially
	available, err := repo.IsPortAvailable(ctx, node.ID, testPort)
	if err != nil {
		t.Fatalf("IsPortAvailable failed: %v", err)
	}
	if !available {
		t.Error("expected port to be available")
	}

	// Allocate it
	_ = repo.AllocateSpecific(ctx, node.ID, testPort, "", "test")

	// Port should not be available
	available, err = repo.IsPortAvailable(ctx, node.ID, testPort)
	if err != nil {
		t.Fatalf("IsPortAvailable failed: %v", err)
	}
	if available {
		t.Error("expected port to be unavailable")
	}

	// Release it
	_ = repo.Release(ctx, node.ID, testPort)

	// Port should be available again
	available, err = repo.IsPortAvailable(ctx, node.ID, testPort)
	if err != nil {
		t.Fatalf("IsPortAvailable failed: %v", err)
	}
	if !available {
		t.Error("expected port to be available after release")
	}
}

func TestPortRepository_CountActiveByNode(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	node := createTestNode(t, db)
	repo := NewPortRepository(db)
	ctx := context.Background()

	// Initially zero
	count, err := repo.CountActiveByNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("CountActiveByNode failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	// Allocate some ports
	_, _ = repo.Allocate(ctx, node.ID, "", "first")
	port2, _ := repo.Allocate(ctx, node.ID, "", "second")
	_, _ = repo.Allocate(ctx, node.ID, "", "third")

	count, err = repo.CountActiveByNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("CountActiveByNode failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}

	// Release one
	_ = repo.Release(ctx, node.ID, port2)

	count, err = repo.CountActiveByNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("CountActiveByNode failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestPortRepository_CleanupExpired(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	node := createTestNode(t, db)
	ws := createTestWorkspace(t, db)
	agent := createTestAgentForPort(t, db, ws)
	repo := NewPortRepository(db)
	agentRepo := NewAgentRepository(db)
	ctx := context.Background()

	// Allocate a port to agent
	port, err := repo.Allocate(ctx, node.ID, agent.ID, "agent port")
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	// Also allocate a port without agent
	_, err = repo.Allocate(ctx, node.ID, "", "no agent")
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	// Cleanup should not release anything yet
	cleaned, err := repo.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("CleanupExpired failed: %v", err)
	}
	if cleaned != 0 {
		t.Errorf("expected 0 cleaned, got %d", cleaned)
	}

	// Delete the agent
	if err := agentRepo.Delete(ctx, agent.ID); err != nil {
		t.Fatalf("delete agent: %v", err)
	}

	// Cleanup should now release the orphaned allocation
	cleaned, err = repo.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("CleanupExpired failed: %v", err)
	}
	if cleaned != 1 {
		t.Errorf("expected 1 cleaned, got %d", cleaned)
	}

	// Port should be available again
	available, _ := repo.IsPortAvailable(ctx, node.ID, port)
	if !available {
		t.Error("expected port to be available after cleanup")
	}
}

func TestPortRepository_CustomRange(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	node := createTestNode(t, db)
	repo := NewPortRepositoryWithRange(db, 20000, 20010)
	ctx := context.Background()

	// Allocate a port
	port, err := repo.Allocate(ctx, node.ID, "", "custom range")
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	if port != 20000 {
		t.Errorf("expected port 20000, got %d", port)
	}

	// Fill up the range
	for i := 1; i <= 10; i++ {
		_, err := repo.Allocate(ctx, node.ID, "", "fill")
		if err != nil {
			t.Fatalf("Allocate %d failed: %v", i, err)
		}
	}

	// Next allocation should fail - range exhausted
	_, err = repo.Allocate(ctx, node.ID, "", "overflow")
	if err != ErrNoAvailablePorts {
		t.Errorf("expected ErrNoAvailablePorts, got: %v", err)
	}
}

func TestPortRepository_NodeIsolation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create two nodes
	nodeRepo := NewNodeRepository(db)
	node1 := &models.Node{
		Name:       "node-1",
		SSHBackend: models.SSHBackendAuto,
		Status:     models.NodeStatusUnknown,
		IsLocal:    true,
	}
	node2 := &models.Node{
		Name:       "node-2",
		SSHBackend: models.SSHBackendAuto,
		Status:     models.NodeStatusUnknown,
		IsLocal:    false,
	}
	if err := nodeRepo.Create(context.Background(), node1); err != nil {
		t.Fatalf("create node1: %v", err)
	}
	if err := nodeRepo.Create(context.Background(), node2); err != nil {
		t.Fatalf("create node2: %v", err)
	}

	repo := NewPortRepository(db)
	ctx := context.Background()

	// Allocate same port on both nodes (should succeed)
	specificPort := 17500

	err := repo.AllocateSpecific(ctx, node1.ID, specificPort, "", "node1")
	if err != nil {
		t.Fatalf("AllocateSpecific on node1 failed: %v", err)
	}

	err = repo.AllocateSpecific(ctx, node2.ID, specificPort, "", "node2")
	if err != nil {
		t.Fatalf("AllocateSpecific on node2 failed: %v", err)
	}

	// Both should have the allocation
	count1, _ := repo.CountActiveByNode(ctx, node1.ID)
	count2, _ := repo.CountActiveByNode(ctx, node2.ID)

	if count1 != 1 || count2 != 1 {
		t.Errorf("expected 1 allocation per node, got node1=%d node2=%d", count1, count2)
	}
}
