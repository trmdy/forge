// Package node provides the NodeService for managing Forge nodes.
package node

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	forgedv1 "github.com/tOgg1/forge/gen/forged/v1"
	"github.com/tOgg1/forge/internal/forged"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/ssh"
)

// ExecutionMode indicates how commands are being executed on a node.
type ExecutionMode string

const (
	// ModeForged uses the forged daemon for execution (preferred).
	ModeForged ExecutionMode = "forged"
	// ModeSSH uses direct SSH execution (fallback).
	ModeSSH ExecutionMode = "ssh"
	// ModeLocal uses local execution.
	ModeLocal ExecutionMode = "local"
)

// Common fallback errors.
var (
	ErrForgedUnavailable = errors.New("forged daemon unavailable")
	ErrNoFallback        = errors.New("no fallback available")
	ErrNodeClosed        = errors.New("node executor closed")
)

// FallbackPolicy controls how the executor handles forged unavailability.
type FallbackPolicy string

const (
	// FallbackPolicyAuto automatically falls back to SSH when forged is unavailable.
	FallbackPolicyAuto FallbackPolicy = "auto"
	// FallbackPolicyForgedOnly requires forged and fails if unavailable.
	FallbackPolicyForgedOnly FallbackPolicy = "forged_only"
	// FallbackPolicySSHOnly only uses SSH, never tries forged.
	FallbackPolicySSHOnly FallbackPolicy = "ssh_only"
)

// NodeExecutor provides unified command execution on a node with automatic
// fallback from forged to SSH when the daemon is unavailable.
type NodeExecutor struct {
	node   *models.Node
	logger zerolog.Logger

	// Configuration
	policy      FallbackPolicy
	forgedPort  int
	pingTimeout time.Duration

	// Connection state
	mu           sync.RWMutex
	mode         ExecutionMode
	sshExecutor  ssh.Executor
	forgedClient *forged.Client
	closed       bool

	// Health tracking
	lastHealthCheck time.Time
	consecutiveFail int
}

// NodeExecutorOption configures a NodeExecutor.
type NodeExecutorOption func(*NodeExecutor)

// WithFallbackPolicy sets the fallback policy.
func WithFallbackPolicy(policy FallbackPolicy) NodeExecutorOption {
	return func(e *NodeExecutor) {
		e.policy = policy
	}
}

// WithForgedPort sets the forged port.
func WithForgedPort(port int) NodeExecutorOption {
	return func(e *NodeExecutor) {
		e.forgedPort = port
	}
}

// WithPingTimeout sets the timeout for health checks.
func WithPingTimeout(timeout time.Duration) NodeExecutorOption {
	return func(e *NodeExecutor) {
		e.pingTimeout = timeout
	}
}

// WithNodeLogger sets the logger for the executor.
func WithNodeLogger(logger zerolog.Logger) NodeExecutorOption {
	return func(e *NodeExecutor) {
		e.logger = logger
	}
}

// NewNodeExecutor creates a new NodeExecutor for the given node.
func NewNodeExecutor(ctx context.Context, node *models.Node, sshExecutor ssh.Executor, opts ...NodeExecutorOption) (*NodeExecutor, error) {
	if node == nil {
		return nil, errors.New("node is required")
	}

	e := &NodeExecutor{
		node:        node,
		logger:      zerolog.Nop(),
		policy:      FallbackPolicyAuto,
		forgedPort:  forged.DefaultPort,
		pingTimeout: 5 * time.Second,
		sshExecutor: sshExecutor,
	}

	for _, opt := range opts {
		opt(e)
	}

	// Determine initial mode based on node type
	if node.IsLocal {
		e.mode = ModeLocal
	} else {
		e.mode = ModeSSH
	}

	// Try to connect to forged if policy allows
	if e.policy != FallbackPolicySSHOnly {
		if err := e.connectForged(ctx); err != nil {
			if e.policy == FallbackPolicyForgedOnly {
				return nil, fmt.Errorf("forged required but unavailable: %w", err)
			}
			e.logger.Debug().Err(err).Msg("forged unavailable, using SSH fallback")
		}
	}

	return e, nil
}

// Mode returns the current execution mode.
func (e *NodeExecutor) Mode() ExecutionMode {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mode
}

// IsForgedAvailable returns true if forged is currently available.
func (e *NodeExecutor) IsForgedAvailable() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.forgedClient != nil && e.mode == ModeForged
}

// connectForged attempts to connect to the forged daemon.
func (e *NodeExecutor) connectForged(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrNodeClosed
	}

	// Close existing client if any
	if e.forgedClient != nil {
		_ = e.forgedClient.Close()
		e.forgedClient = nil
	}

	var client *forged.Client
	var err error

	if e.node.IsLocal {
		// Direct connection for local nodes
		target := fmt.Sprintf("127.0.0.1:%d", e.forgedPort)
		client, err = forged.Dial(ctx, target, forged.WithLogger(e.logger))
	} else if e.sshExecutor != nil {
		// Use SSH tunnel for remote nodes
		forwarder, ok := e.sshExecutor.(ssh.PortForwarder)
		if !ok {
			return fmt.Errorf("SSH executor does not support port forwarding")
		}
		client, err = forged.DialSSHWithExecutor(ctx, forwarder, e.forgedPort, forged.WithLogger(e.logger))
	} else {
		return fmt.Errorf("no SSH executor available for remote node")
	}

	if err != nil {
		return err
	}

	// Verify connectivity with a ping
	pingCtx, cancel := context.WithTimeout(ctx, e.pingTimeout)
	defer cancel()

	if _, err := client.Ping(pingCtx); err != nil {
		_ = client.Close()
		return fmt.Errorf("forged ping failed: %w", err)
	}

	e.forgedClient = client
	e.mode = ModeForged
	e.consecutiveFail = 0
	e.lastHealthCheck = time.Now()

	e.logger.Info().
		Str("node", e.node.Name).
		Str("mode", string(e.mode)).
		Msg("connected to forged")

	return nil
}

// switchToSSH switches from forged mode to SSH fallback.
func (e *NodeExecutor) switchToSSH() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.forgedClient != nil {
		_ = e.forgedClient.Close()
		e.forgedClient = nil
	}

	if e.node.IsLocal {
		e.mode = ModeLocal
	} else {
		e.mode = ModeSSH
	}

	e.logger.Warn().
		Str("node", e.node.Name).
		Str("mode", string(e.mode)).
		Msg("switched to SSH fallback")
}

// handleForgedError processes a forged error and potentially triggers fallback.
func (e *NodeExecutor) handleForgedError(err error) error {
	if err == nil {
		return nil
	}

	e.mu.Lock()
	e.consecutiveFail++
	fail := e.consecutiveFail
	e.mu.Unlock()

	// After 3 consecutive failures, switch to SSH fallback
	if fail >= 3 && e.policy == FallbackPolicyAuto {
		e.switchToSSH()
		return fmt.Errorf("%w: %v", ErrForgedUnavailable, err)
	}

	return err
}

// TryReconnectForged attempts to reconnect to forged if currently in fallback mode.
func (e *NodeExecutor) TryReconnectForged(ctx context.Context) error {
	e.mu.RLock()
	mode := e.mode
	policy := e.policy
	e.mu.RUnlock()

	if mode == ModeForged {
		return nil // Already connected
	}

	if policy == FallbackPolicySSHOnly {
		return nil // Don't try to reconnect
	}

	return e.connectForged(ctx)
}

// Exec executes a command on the node.
// For forged mode, this is not directly supported - use SSH mode.
func (e *NodeExecutor) Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error) {
	e.mu.RLock()
	executor := e.sshExecutor
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, nil, ErrNodeClosed
	}

	if executor == nil {
		return nil, nil, errors.New("no SSH executor available")
	}

	return executor.Exec(ctx, cmd)
}

// Ping checks if the node is responsive.
func (e *NodeExecutor) Ping(ctx context.Context) error {
	e.mu.RLock()
	mode := e.mode
	client := e.forgedClient
	executor := e.sshExecutor
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return ErrNodeClosed
	}

	switch mode {
	case ModeForged:
		if client == nil {
			return ErrForgedUnavailable
		}
		_, err := client.Ping(ctx)
		if err != nil {
			return e.handleForgedError(err)
		}
		e.mu.Lock()
		e.consecutiveFail = 0
		e.lastHealthCheck = time.Now()
		e.mu.Unlock()
		return nil

	case ModeSSH, ModeLocal:
		if executor == nil {
			return errors.New("no executor available")
		}
		stdout, stderr, err := executor.Exec(ctx, "echo ok")
		if err != nil {
			return fmt.Errorf("ping failed: %w (stderr: %s)", err, string(stderr))
		}
		if len(stdout) == 0 {
			return errors.New("ping returned empty response")
		}
		return nil

	default:
		return fmt.Errorf("unknown mode: %s", mode)
	}
}

// ForgedClient returns the forged client if available.
// Returns nil if forged is not connected.
func (e *NodeExecutor) ForgedClient() *forged.Client {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.forgedClient
}

// SSHExecutor returns the SSH executor.
func (e *NodeExecutor) SSHExecutor() ssh.Executor {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sshExecutor
}

// SpawnAgent spawns an agent using forged if available, otherwise returns an error.
func (e *NodeExecutor) SpawnAgent(ctx context.Context, req *forgedv1.SpawnAgentRequest) (*forgedv1.SpawnAgentResponse, error) {
	e.mu.RLock()
	client := e.forgedClient
	mode := e.mode
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, ErrNodeClosed
	}

	if mode != ModeForged || client == nil {
		return nil, ErrForgedUnavailable
	}

	resp, err := client.SpawnAgent(ctx, req)
	if err != nil {
		return nil, e.handleForgedError(err)
	}
	return resp, nil
}

// KillAgent kills an agent using forged if available.
func (e *NodeExecutor) KillAgent(ctx context.Context, req *forgedv1.KillAgentRequest) (*forgedv1.KillAgentResponse, error) {
	e.mu.RLock()
	client := e.forgedClient
	mode := e.mode
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, ErrNodeClosed
	}

	if mode != ModeForged || client == nil {
		return nil, ErrForgedUnavailable
	}

	resp, err := client.KillAgent(ctx, req)
	if err != nil {
		return nil, e.handleForgedError(err)
	}
	return resp, nil
}

// CapturePane captures pane content using forged if available.
func (e *NodeExecutor) CapturePane(ctx context.Context, req *forgedv1.CapturePaneRequest) (*forgedv1.CapturePaneResponse, error) {
	e.mu.RLock()
	client := e.forgedClient
	mode := e.mode
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, ErrNodeClosed
	}

	if mode != ModeForged || client == nil {
		return nil, ErrForgedUnavailable
	}

	resp, err := client.CapturePane(ctx, req)
	if err != nil {
		return nil, e.handleForgedError(err)
	}
	return resp, nil
}

// SendInput sends input to an agent using forged if available.
func (e *NodeExecutor) SendInput(ctx context.Context, req *forgedv1.SendInputRequest) (*forgedv1.SendInputResponse, error) {
	e.mu.RLock()
	client := e.forgedClient
	mode := e.mode
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, ErrNodeClosed
	}

	if mode != ModeForged || client == nil {
		return nil, ErrForgedUnavailable
	}

	resp, err := client.SendInput(ctx, req)
	if err != nil {
		return nil, e.handleForgedError(err)
	}
	return resp, nil
}

// ListAgents lists agents using forged if available.
func (e *NodeExecutor) ListAgents(ctx context.Context, req *forgedv1.ListAgentsRequest) (*forgedv1.ListAgentsResponse, error) {
	e.mu.RLock()
	client := e.forgedClient
	mode := e.mode
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, ErrNodeClosed
	}

	if mode != ModeForged || client == nil {
		return nil, ErrForgedUnavailable
	}

	resp, err := client.ListAgents(ctx, req)
	if err != nil {
		return nil, e.handleForgedError(err)
	}
	return resp, nil
}

// GetAgent gets agent details using forged if available.
func (e *NodeExecutor) GetAgent(ctx context.Context, req *forgedv1.GetAgentRequest) (*forgedv1.GetAgentResponse, error) {
	e.mu.RLock()
	client := e.forgedClient
	mode := e.mode
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, ErrNodeClosed
	}

	if mode != ModeForged || client == nil {
		return nil, ErrForgedUnavailable
	}

	resp, err := client.GetAgent(ctx, req)
	if err != nil {
		return nil, e.handleForgedError(err)
	}
	return resp, nil
}

// Close closes the executor and releases all resources.
func (e *NodeExecutor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}
	e.closed = true

	var errs []error

	if e.forgedClient != nil {
		if err := e.forgedClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close forged client: %w", err))
		}
		e.forgedClient = nil
	}

	// Note: We don't close sshExecutor here because it may be shared
	// The caller is responsible for managing the SSH executor lifecycle

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// NewNodeExecutorForService creates a NodeExecutor using the node service's executor creation.
func (s *Service) NewNodeExecutor(ctx context.Context, node *models.Node, opts ...NodeExecutorOption) (*NodeExecutor, error) {
	var sshExecutor ssh.Executor

	if !node.IsLocal {
		var err error
		sshExecutor, err = s.executorForNode(node)
		if err != nil {
			return nil, fmt.Errorf("failed to create SSH executor: %w", err)
		}
	}

	exec, err := NewNodeExecutor(ctx, node, sshExecutor, opts...)
	if err != nil {
		if sshExecutor != nil {
			_ = sshExecutor.Close()
		}
		return nil, err
	}

	return exec, nil
}
