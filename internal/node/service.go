// Package node provides the NodeService for managing Forge nodes.
package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/events"
	"github.com/tOgg1/forge/internal/logging"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/ssh"
)

// Common service errors
var (
	ErrNodeNotFound      = errors.New("node not found")
	ErrNodeAlreadyExists = errors.New("node already exists")
	ErrInvalidSSHTarget  = errors.New("invalid SSH target format")
	ErrConnectionFailed  = errors.New("connection test failed")
)

// Service manages Forge nodes.
type Service struct {
	repo      *db.NodeRepository
	publisher events.Publisher
	logger    zerolog.Logger

	// DefaultTimeout is the default timeout for SSH operations.
	DefaultTimeout time.Duration
}

// ServiceOption configures a Service.
type ServiceOption func(*Service)

// WithDefaultTimeout sets the default timeout for SSH operations.
func WithDefaultTimeout(timeout time.Duration) ServiceOption {
	return func(s *Service) {
		s.DefaultTimeout = timeout
	}
}

// WithPublisher sets the event publisher for the service.
func WithPublisher(publisher events.Publisher) ServiceOption {
	return func(s *Service) {
		s.publisher = publisher
	}
}

// NewService creates a new NodeService.
func NewService(repo *db.NodeRepository, opts ...ServiceOption) *Service {
	s := &Service{
		repo:           repo,
		logger:         logging.Component("node"),
		DefaultTimeout: 30 * time.Second,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// AddNode creates a new node and optionally tests the connection.
func (s *Service) AddNode(ctx context.Context, node *models.Node, testConnection bool) error {
	// Parse and validate SSH target for remote nodes
	if !node.IsLocal && node.SSHTarget != "" {
		if err := validateSSHTarget(node.SSHTarget); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidSSHTarget, err)
		}
	}

	// Set defaults
	if node.SSHBackend == "" {
		node.SSHBackend = models.SSHBackendAuto
	}
	if node.Status == "" {
		node.Status = models.NodeStatusUnknown
	}

	// Optionally test connection before adding
	if testConnection && !node.IsLocal {
		result, err := s.TestConnection(ctx, node)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrConnectionFailed, err)
		}
		if !result.Success {
			return fmt.Errorf("%w: %s", ErrConnectionFailed, result.Error)
		}
		node.Status = models.NodeStatusOnline
		node.Metadata = result.Metadata
	}

	// Create in database
	if err := s.repo.Create(ctx, node); err != nil {
		if errors.Is(err, db.ErrNodeAlreadyExists) {
			return ErrNodeAlreadyExists
		}
		return fmt.Errorf("failed to create node: %w", err)
	}

	s.logger.Info().
		Str("node_id", node.ID).
		Str("name", node.Name).
		Bool("is_local", node.IsLocal).
		Msg("node added")

	// Emit event
	s.publishEvent(ctx, models.EventTypeNodeAdded, node.ID, nil)

	return nil
}

// publishEvent publishes an event if a publisher is configured.
func (s *Service) publishEvent(ctx context.Context, eventType models.EventType, nodeID string, payload any) {
	if s.publisher == nil {
		return
	}

	event := &models.Event{
		Type:       eventType,
		EntityType: models.EntityTypeNode,
		EntityID:   nodeID,
	}

	if payload != nil {
		if data, err := json.Marshal(payload); err == nil {
			event.Payload = data
		}
	}

	s.publisher.Publish(ctx, event)
}

// RemoveNode deletes a node by ID.
func (s *Service) RemoveNode(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, db.ErrNodeNotFound) {
			return ErrNodeNotFound
		}
		return fmt.Errorf("failed to remove node: %w", err)
	}

	s.logger.Info().Str("node_id", id).Msg("node removed")

	// Emit event
	s.publishEvent(ctx, models.EventTypeNodeRemoved, id, nil)

	return nil
}

// ListNodes returns all nodes, optionally filtered by status.
func (s *Service) ListNodes(ctx context.Context, status *models.NodeStatus) ([]*models.Node, error) {
	nodes, err := s.repo.List(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
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

// GetNode retrieves a node by ID.
func (s *Service) GetNode(ctx context.Context, id string) (*models.Node, error) {
	node, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrNodeNotFound) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	// Populate agent count
	count, err := s.repo.GetAgentCount(ctx, node.ID)
	if err != nil {
		s.logger.Warn().Err(err).Str("node_id", node.ID).Msg("failed to get agent count")
	} else {
		node.AgentCount = count
	}

	return node, nil
}

// GetNodeByName retrieves a node by name.
func (s *Service) GetNodeByName(ctx context.Context, name string) (*models.Node, error) {
	node, err := s.repo.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, db.ErrNodeNotFound) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	// Populate agent count
	count, err := s.repo.GetAgentCount(ctx, node.ID)
	if err != nil {
		s.logger.Warn().Err(err).Str("node_id", node.ID).Msg("failed to get agent count")
	} else {
		node.AgentCount = count
	}

	return node, nil
}

// UpdateNode updates an existing node.
func (s *Service) UpdateNode(ctx context.Context, node *models.Node) error {
	if !node.IsLocal && node.SSHTarget != "" {
		if err := validateSSHTarget(node.SSHTarget); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidSSHTarget, err)
		}
	}

	if err := s.repo.Update(ctx, node); err != nil {
		if errors.Is(err, db.ErrNodeNotFound) {
			return ErrNodeNotFound
		}
		if errors.Is(err, db.ErrNodeAlreadyExists) {
			return ErrNodeAlreadyExists
		}
		return fmt.Errorf("failed to update node: %w", err)
	}

	s.logger.Info().Str("node_id", node.ID).Msg("node updated")
	return nil
}

// ConnectionResult contains the result of a connection test.
type ConnectionResult struct {
	Success  bool
	Latency  time.Duration
	Error    string
	Metadata models.NodeMetadata
}

// TestConnection tests SSH connectivity to a node.
func (s *Service) TestConnection(ctx context.Context, node *models.Node) (*ConnectionResult, error) {
	executor, err := s.executorForNode(node)
	if err != nil {
		return &ConnectionResult{
			Success: false,
			Error:   fmt.Sprintf("failed to create executor: %v", err),
		}, nil
	}
	defer executor.Close()

	// Test connection and gather metadata
	start := time.Now()

	result := &ConnectionResult{
		Success: true,
	}

	// Run a simple command to verify connectivity
	stdout, stderr, err := executor.Exec(ctx, "echo ok")
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("connection test failed: %v (stderr: %s)", err, string(stderr))
		return result, nil
	}

	result.Latency = time.Since(start)

	if strings.TrimSpace(string(stdout)) != "ok" {
		result.Success = false
		result.Error = fmt.Sprintf("unexpected response: %s", string(stdout))
		return result, nil
	}

	// Gather metadata
	result.Metadata = s.gatherNodeMetadata(ctx, executor)
	if node.IsLocal {
		result.Metadata.Platform = "local"
	}

	return result, nil
}

func (s *Service) executorForNode(node *models.Node) (ssh.Executor, error) {
	if node.IsLocal {
		return ssh.NewLocalExecutor(), nil
	}

	// Parse SSH target
	user, host, port := ParseSSHTarget(node.SSHTarget)

	// Build connection options
	opts := ssh.ConnectionOptions{
		Host:    host,
		Port:    port,
		User:    user,
		KeyPath: node.SSHKeyPath,
		Timeout: s.DefaultTimeout,

		AgentForwarding: node.SSHAgentForwarding,
		ProxyJump:       node.SSHProxyJump,
		ControlMaster:   node.SSHControlMaster,
		ControlPath:     node.SSHControlPath,
		ControlPersist:  node.SSHControlPersist,
	}
	if node.SSHTimeoutSeconds > 0 {
		opts.Timeout = time.Duration(node.SSHTimeoutSeconds) * time.Second
	}

	// Create executor based on backend preference
	return s.createExecutor(node.SSHBackend, opts)
}

// RefreshNodeStatus tests connectivity and updates the node's status.
func (s *Service) RefreshNodeStatus(ctx context.Context, id string) (*ConnectionResult, error) {
	node, err := s.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}

	result, err := s.TestConnection(ctx, node)
	if err != nil {
		return nil, err
	}

	// Update status based on result
	var newStatus models.NodeStatus
	if result.Success {
		newStatus = models.NodeStatusOnline
	} else {
		newStatus = models.NodeStatusOffline
	}

	oldStatus := node.Status
	if err := s.repo.UpdateStatus(ctx, id, newStatus); err != nil {
		return nil, fmt.Errorf("failed to update node status: %w", err)
	}

	// Emit status change event
	if oldStatus != newStatus {
		if newStatus == models.NodeStatusOnline {
			s.publishEvent(ctx, models.EventTypeNodeOnline, id, nil)
		} else if newStatus == models.NodeStatusOffline {
			s.publishEvent(ctx, models.EventTypeNodeOffline, id, nil)
		}
	}

	return result, nil
}

// createExecutor creates an SSH executor based on the backend preference.
func (s *Service) createExecutor(backend models.SSHBackend, opts ssh.ConnectionOptions) (ssh.Executor, error) {
	if opts.Host != "" {
		updated, err := ssh.ApplySSHConfig(opts)
		if err != nil {
			s.logger.Warn().Err(err).Str("host", opts.Host).Msg("failed to apply ssh config")
		} else {
			opts = updated
		}
	}

	switch backend {
	case models.SSHBackendNative:
		return ssh.NewNativeExecutor(opts)
	case models.SSHBackendSystem:
		return ssh.NewSystemExecutor(opts), nil
	case models.SSHBackendAuto:
		// Try native first, fall back to system
		executor, err := ssh.NewNativeExecutor(opts)
		if err != nil {
			s.logger.Debug().Err(err).Msg("native SSH failed, falling back to system")
			return ssh.NewSystemExecutor(opts), nil
		}
		return executor, nil
	default:
		return ssh.NewSystemExecutor(opts), nil
	}
}

// gatherNodeMetadata collects metadata about a node via SSH.
func (s *Service) gatherNodeMetadata(ctx context.Context, executor ssh.Executor) models.NodeMetadata {
	metadata := models.NodeMetadata{}

	// Get hostname
	if stdout, _, err := executor.Exec(ctx, "hostname"); err == nil {
		metadata.Hostname = strings.TrimSpace(string(stdout))
	}

	// Get platform
	if stdout, _, err := executor.Exec(ctx, "uname -s"); err == nil {
		metadata.Platform = strings.ToLower(strings.TrimSpace(string(stdout)))
	}

	// Get tmux version
	if stdout, _, err := executor.Exec(ctx, "tmux -V 2>/dev/null"); err == nil {
		metadata.TmuxVersion = strings.TrimSpace(string(stdout))
	}

	// Check for available agent adapters
	adapters := []string{}
	adapterChecks := map[string]string{
		"claude":   "which claude 2>/dev/null",
		"opencode": "which opencode 2>/dev/null",
		"codex":    "which codex 2>/dev/null",
		"aider":    "which aider 2>/dev/null",
	}

	for name, cmd := range adapterChecks {
		if stdout, _, err := executor.Exec(ctx, cmd); err == nil && len(stdout) > 0 {
			adapters = append(adapters, name)
		}
	}
	metadata.AvailableAdapters = adapters

	// Check for forged daemon
	if stdout, _, err := executor.Exec(ctx, "forged --version 2>/dev/null"); err == nil && len(stdout) > 0 {
		metadata.ForgedVersion = strings.TrimSpace(string(stdout))
	}

	// Check if forged is running
	if _, _, err := executor.Exec(ctx, "pgrep -x forged >/dev/null 2>&1"); err == nil {
		metadata.ForgedStatus = "running"
	} else if metadata.ForgedVersion != "" {
		metadata.ForgedStatus = "installed"
	} else {
		metadata.ForgedStatus = "not_installed"
	}

	return metadata
}

// ParseSSHTarget parses a user@host:port string into its components.
// It handles various formats:
//   - host
//   - user@host
//   - host:port
//   - user@host:port
func ParseSSHTarget(target string) (user, host string, port int) {
	port = 22 // default

	// Extract user
	if atIdx := strings.Index(target, "@"); atIdx >= 0 {
		user = target[:atIdx]
		target = target[atIdx+1:]
	}

	// Extract host and port
	h, p, err := net.SplitHostPort(target)
	if err != nil {
		// No port specified
		host = target
	} else {
		host = h
		if parsed, err := strconv.Atoi(p); err == nil {
			port = parsed
		}
	}

	return user, host, port
}

// ExecResult contains the result of executing a command on a node.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Error    string
}

// ExecCommand executes a command on a node and returns the result.
func (s *Service) ExecCommand(ctx context.Context, node *models.Node, cmd string) (*ExecResult, error) {
	executor, err := s.executorForNode(node)
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}
	defer executor.Close()

	// Execute the command
	stdout, stderr, execErr := executor.Exec(ctx, cmd)
	result := &ExecResult{
		Stdout:   string(stdout),
		Stderr:   string(stderr),
		ExitCode: 0,
	}

	if execErr != nil {
		result.Error = execErr.Error()
		// Try to extract exit code from error
		var exitErr *ssh.ExitError
		if errors.As(execErr, &exitErr) {
			result.ExitCode = exitErr.Code
		} else {
			result.ExitCode = 1
		}
	}

	return result, nil
}

// validateSSHTarget validates that an SSH target string is well-formed.
func validateSSHTarget(target string) error {
	if target == "" {
		return errors.New("empty target")
	}

	user, host, port := ParseSSHTarget(target)
	_ = user // user is optional

	if host == "" {
		return errors.New("missing host")
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}

	return nil
}
