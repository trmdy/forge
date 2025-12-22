// Package services implements the business logic layer for Swarm.
package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/opencode-ai/swarm/internal/db"
	"github.com/opencode-ai/swarm/internal/logging"
	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/ssh"
	"github.com/rs/zerolog"
)

// NodeService errors.
var (
	ErrNodeNotFound      = errors.New("node not found")
	ErrNodeAlreadyExists = errors.New("node already exists")
	ErrConnectionFailed  = errors.New("connection test failed")
)

// NodeService manages node operations.
type NodeService struct {
	repo   *db.NodeRepository
	logger zerolog.Logger
}

// NewNodeService creates a new NodeService.
func NewNodeService(repo *db.NodeRepository, logger zerolog.Logger) *NodeService {
	return &NodeService{
		repo:   repo,
		logger: logger,
	}
}

// NewNodeServiceDefault creates a NodeService with the default component logger.
func NewNodeServiceDefault(repo *db.NodeRepository) *NodeService {
	return &NodeService{
		repo:   repo,
		logger: logging.Component("node-service"),
	}
}

// AddNodeInput contains the parameters for adding a node.
type AddNodeInput struct {
	Name       string
	SSHTarget  string // user@host:port format
	SSHBackend models.SSHBackend
	SSHKeyPath string
	IsLocal    bool
}

// AddNode adds a new node to Swarm.
func (s *NodeService) AddNode(ctx context.Context, input AddNodeInput) (*models.Node, error) {
	s.logger.Debug().
		Str("name", input.Name).
		Str("ssh_target", input.SSHTarget).
		Bool("is_local", input.IsLocal).
		Msg("adding node")

	node := &models.Node{
		Name:       input.Name,
		SSHTarget:  input.SSHTarget,
		SSHBackend: input.SSHBackend,
		SSHKeyPath: input.SSHKeyPath,
		IsLocal:    input.IsLocal,
		Status:     models.NodeStatusUnknown,
	}

	// Default SSH backend to auto
	if node.SSHBackend == "" {
		node.SSHBackend = models.SSHBackendAuto
	}

	if err := node.Validate(); err != nil {
		return nil, fmt.Errorf("validate node: %w", err)
	}

	if err := s.repo.Create(ctx, node); err != nil {
		if errors.Is(err, db.ErrNodeAlreadyExists) {
			return nil, ErrNodeAlreadyExists
		}
		return nil, fmt.Errorf("create node: %w", err)
	}

	s.logger.Info().
		Str("node_id", node.ID).
		Str("name", node.Name).
		Msg("node added")

	return node, nil
}

// RemoveNode removes a node from Swarm.
func (s *NodeService) RemoveNode(ctx context.Context, id string) error {
	s.logger.Debug().Str("node_id", id).Msg("removing node")

	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, db.ErrNodeNotFound) {
			return ErrNodeNotFound
		}
		return fmt.Errorf("delete node: %w", err)
	}

	s.logger.Info().Str("node_id", id).Msg("node removed")
	return nil
}

// ListNodes returns all nodes, optionally filtered by status.
func (s *NodeService) ListNodes(ctx context.Context, status *models.NodeStatus) ([]*models.Node, error) {
	nodes, err := s.repo.List(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	// Populate agent counts
	for _, node := range nodes {
		count, err := s.repo.GetAgentCount(ctx, node.ID)
		if err != nil {
			s.logger.Warn().Err(err).Str("node_id", node.ID).Msg("failed to get agent count")
			continue
		}
		node.AgentCount = count
	}

	return nodes, nil
}

// GetNode retrieves a node by ID or name.
func (s *NodeService) GetNode(ctx context.Context, idOrName string) (*models.Node, error) {
	// Try by ID first
	node, err := s.repo.Get(ctx, idOrName)
	if err == nil {
		count, _ := s.repo.GetAgentCount(ctx, node.ID)
		node.AgentCount = count
		return node, nil
	}

	if !errors.Is(err, db.ErrNodeNotFound) {
		return nil, fmt.Errorf("get node by id: %w", err)
	}

	// Try by name
	node, err = s.repo.GetByName(ctx, idOrName)
	if err != nil {
		if errors.Is(err, db.ErrNodeNotFound) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("get node by name: %w", err)
	}

	count, _ := s.repo.GetAgentCount(ctx, node.ID)
	node.AgentCount = count
	return node, nil
}

// ConnectionTestResult contains the results of a connection test.
type ConnectionTestResult struct {
	Success     bool
	Latency     time.Duration
	TmuxVersion string
	Platform    string
	Hostname    string
	Error       string
}

// TestConnection tests SSH connectivity to a node.
func (s *NodeService) TestConnection(ctx context.Context, idOrName string) (*ConnectionTestResult, error) {
	node, err := s.GetNode(ctx, idOrName)
	if err != nil {
		return nil, err
	}

	result := &ConnectionTestResult{}
	start := time.Now()

	if node.IsLocal {
		result = s.testLocalConnection(ctx)
	} else {
		result = s.testRemoteConnection(ctx, node)
	}

	result.Latency = time.Since(start)

	// Update node status based on test result
	newStatus := models.NodeStatusOffline
	if result.Success {
		newStatus = models.NodeStatusOnline

		// Update metadata
		node.Metadata.TmuxVersion = result.TmuxVersion
		node.Metadata.Platform = result.Platform
		node.Metadata.Hostname = result.Hostname
		now := time.Now().UTC()
		node.LastSeen = &now

		if err := s.repo.Update(ctx, node); err != nil {
			s.logger.Warn().Err(err).Str("node_id", node.ID).Msg("failed to update node metadata")
		}
	}

	if err := s.repo.UpdateStatus(ctx, node.ID, newStatus); err != nil {
		s.logger.Warn().Err(err).Str("node_id", node.ID).Msg("failed to update node status")
	}

	return result, nil
}

// testLocalConnection tests the local node's connectivity.
func (s *NodeService) testLocalConnection(ctx context.Context) *ConnectionTestResult {
	result := &ConnectionTestResult{
		Success:  true,
		Platform: runtime.GOOS,
	}

	// Get hostname
	if hostname, err := os.Hostname(); err == nil {
		result.Hostname = hostname
	}

	// Check tmux availability
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		result.Success = false
		result.Error = "tmux not found in PATH"
		return result
	}

	// Get tmux version
	cmd := exec.CommandContext(ctx, tmuxPath, "-V")
	out, err := cmd.Output()
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to get tmux version: %v", err)
		return result
	}

	result.TmuxVersion = strings.TrimSpace(string(out))
	return result
}

// testRemoteConnection tests SSH connectivity to a remote node.
func (s *NodeService) testRemoteConnection(ctx context.Context, node *models.Node) *ConnectionTestResult {
	result := &ConnectionTestResult{}

	// Parse SSH target
	opts, err := ssh.ParseSSHTarget(node.SSHTarget)
	if err != nil {
		result.Error = fmt.Sprintf("invalid SSH target: %v", err)
		return result
	}

	opts.KeyPath = node.SSHKeyPath
	opts.Timeout = 10 * time.Second

	// Determine which executor to use
	var executor ssh.Executor
	switch node.SSHBackend {
	case models.SSHBackendNative:
		executor, err = ssh.NewNativeExecutor(*opts, nil)
	case models.SSHBackendSystem:
		executor = ssh.NewSystemExecutor(*opts)
	case models.SSHBackendAuto:
		// Try native first, fall back to system
		executor, err = ssh.NewNativeExecutor(*opts, nil)
		if err != nil {
			s.logger.Debug().Err(err).Msg("native SSH failed, falling back to system")
			executor = ssh.NewSystemExecutor(*opts)
			err = nil
		}
	default:
		executor = ssh.NewSystemExecutor(*opts)
	}

	if err != nil {
		result.Error = fmt.Sprintf("failed to create SSH executor: %v", err)
		return result
	}
	defer executor.Close()

	// Test connection with a simple command
	stdout, stderr, err := executor.Exec(ctx, "echo ok && uname -s && hostname && tmux -V 2>/dev/null || echo 'tmux not found'")
	if err != nil {
		result.Error = fmt.Sprintf("SSH command failed: %v, stderr: %s", err, string(stderr))
		return result
	}

	lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	if len(lines) < 1 || lines[0] != "ok" {
		result.Error = fmt.Sprintf("unexpected output: %s", string(stdout))
		return result
	}

	result.Success = true

	if len(lines) >= 2 {
		result.Platform = strings.ToLower(strings.TrimSpace(lines[1]))
	}
	if len(lines) >= 3 {
		result.Hostname = strings.TrimSpace(lines[2])
	}
	if len(lines) >= 4 {
		tmuxVer := strings.TrimSpace(lines[3])
		if tmuxVer != "tmux not found" {
			result.TmuxVersion = tmuxVer
		} else {
			result.Success = false
			result.Error = "tmux not installed on remote node"
		}
	}

	return result
}

// UpdateNode updates an existing node's configuration.
func (s *NodeService) UpdateNode(ctx context.Context, node *models.Node) error {
	s.logger.Debug().Str("node_id", node.ID).Msg("updating node")

	if err := s.repo.Update(ctx, node); err != nil {
		if errors.Is(err, db.ErrNodeNotFound) {
			return ErrNodeNotFound
		}
		return fmt.Errorf("update node: %w", err)
	}

	s.logger.Info().Str("node_id", node.ID).Msg("node updated")
	return nil
}

// EnsureLocalNode ensures a local node exists and returns it.
// If it doesn't exist, it creates one automatically.
func (s *NodeService) EnsureLocalNode(ctx context.Context) (*models.Node, error) {
	const localNodeName = "local"

	node, err := s.repo.GetByName(ctx, localNodeName)
	if err == nil {
		return node, nil
	}

	if !errors.Is(err, db.ErrNodeNotFound) {
		return nil, fmt.Errorf("get local node: %w", err)
	}

	// Create the local node
	s.logger.Info().Msg("creating local node")

	hostname, _ := os.Hostname()

	node = &models.Node{
		Name:       localNodeName,
		IsLocal:    true,
		Status:     models.NodeStatusUnknown,
		SSHBackend: models.SSHBackendAuto,
		Metadata: models.NodeMetadata{
			Platform: runtime.GOOS,
			Hostname: hostname,
		},
	}

	if err := s.repo.Create(ctx, node); err != nil {
		// Handle race condition where another process created it
		if errors.Is(err, db.ErrNodeAlreadyExists) {
			return s.repo.GetByName(ctx, localNodeName)
		}
		return nil, fmt.Errorf("create local node: %w", err)
	}

	// Test connection to populate metadata
	if _, testErr := s.TestConnection(ctx, node.ID); testErr != nil {
		s.logger.Warn().Err(testErr).Msg("failed to test local node connection")
	}

	// Re-fetch to get updated metadata
	return s.repo.Get(ctx, node.ID)
}
