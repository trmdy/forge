// Package node provides the NodeService for managing Swarm nodes.
package node

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	swarmdv1 "github.com/opencode-ai/swarm/gen/swarmd/v1"
	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/ssh"
	"github.com/opencode-ai/swarm/internal/swarmd"
	"github.com/rs/zerolog"
)

// ExecutionMode indicates how commands are being executed on a node.
type ExecutionMode string

const (
	// ModeSwarmd uses the swarmd daemon for execution (preferred).
	ModeSwarmd ExecutionMode = "swarmd"
	// ModeSSH uses direct SSH execution (fallback).
	ModeSSH ExecutionMode = "ssh"
	// ModeLocal uses local execution.
	ModeLocal ExecutionMode = "local"
)

// Common fallback errors.
var (
	ErrSwarmdUnavailable = errors.New("swarmd daemon unavailable")
	ErrNoFallback        = errors.New("no fallback available")
	ErrNodeClosed        = errors.New("node executor closed")
)

// FallbackPolicy controls how the executor handles swarmd unavailability.
type FallbackPolicy string

const (
	// FallbackPolicyAuto automatically falls back to SSH when swarmd is unavailable.
	FallbackPolicyAuto FallbackPolicy = "auto"
	// FallbackPolicySwarmdOnly requires swarmd and fails if unavailable.
	FallbackPolicySwarmdOnly FallbackPolicy = "swarmd_only"
	// FallbackPolicySSHOnly only uses SSH, never tries swarmd.
	FallbackPolicySSHOnly FallbackPolicy = "ssh_only"
)

// NodeExecutor provides unified command execution on a node with automatic
// fallback from swarmd to SSH when the daemon is unavailable.
type NodeExecutor struct {
	node   *models.Node
	logger zerolog.Logger

	// Configuration
	policy      FallbackPolicy
	swarmdPort  int
	pingTimeout time.Duration

	// Connection state
	mu           sync.RWMutex
	mode         ExecutionMode
	sshExecutor  ssh.Executor
	swarmdClient *swarmd.Client
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

// WithSwarmdPort sets the swarmd port.
func WithSwarmdPort(port int) NodeExecutorOption {
	return func(e *NodeExecutor) {
		e.swarmdPort = port
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
		swarmdPort:  swarmd.DefaultPort,
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

	// Try to connect to swarmd if policy allows
	if e.policy != FallbackPolicySSHOnly {
		if err := e.connectSwarmd(ctx); err != nil {
			if e.policy == FallbackPolicySwarmdOnly {
				return nil, fmt.Errorf("swarmd required but unavailable: %w", err)
			}
			e.logger.Debug().Err(err).Msg("swarmd unavailable, using SSH fallback")
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

// IsSwarmdAvailable returns true if swarmd is currently available.
func (e *NodeExecutor) IsSwarmdAvailable() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.swarmdClient != nil && e.mode == ModeSwarmd
}

// connectSwarmd attempts to connect to the swarmd daemon.
func (e *NodeExecutor) connectSwarmd(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrNodeClosed
	}

	// Close existing client if any
	if e.swarmdClient != nil {
		_ = e.swarmdClient.Close()
		e.swarmdClient = nil
	}

	var client *swarmd.Client
	var err error

	if e.node.IsLocal {
		// Direct connection for local nodes
		target := fmt.Sprintf("127.0.0.1:%d", e.swarmdPort)
		client, err = swarmd.Dial(ctx, target, swarmd.WithLogger(e.logger))
	} else if e.sshExecutor != nil {
		// Use SSH tunnel for remote nodes
		forwarder, ok := e.sshExecutor.(ssh.PortForwarder)
		if !ok {
			return fmt.Errorf("SSH executor does not support port forwarding")
		}
		client, err = swarmd.DialSSHWithExecutor(ctx, forwarder, e.swarmdPort, swarmd.WithLogger(e.logger))
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
		return fmt.Errorf("swarmd ping failed: %w", err)
	}

	e.swarmdClient = client
	e.mode = ModeSwarmd
	e.consecutiveFail = 0
	e.lastHealthCheck = time.Now()

	e.logger.Info().
		Str("node", e.node.Name).
		Str("mode", string(e.mode)).
		Msg("connected to swarmd")

	return nil
}

// switchToSSH switches from swarmd mode to SSH fallback.
func (e *NodeExecutor) switchToSSH() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.swarmdClient != nil {
		_ = e.swarmdClient.Close()
		e.swarmdClient = nil
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

// handleSwarmdError processes a swarmd error and potentially triggers fallback.
func (e *NodeExecutor) handleSwarmdError(err error) error {
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
		return fmt.Errorf("%w: %v", ErrSwarmdUnavailable, err)
	}

	return err
}

// TryReconnectSwarmd attempts to reconnect to swarmd if currently in fallback mode.
func (e *NodeExecutor) TryReconnectSwarmd(ctx context.Context) error {
	e.mu.RLock()
	mode := e.mode
	policy := e.policy
	e.mu.RUnlock()

	if mode == ModeSwarmd {
		return nil // Already connected
	}

	if policy == FallbackPolicySSHOnly {
		return nil // Don't try to reconnect
	}

	return e.connectSwarmd(ctx)
}

// Exec executes a command on the node.
// For swarmd mode, this is not directly supported - use SSH mode.
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
	client := e.swarmdClient
	executor := e.sshExecutor
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return ErrNodeClosed
	}

	switch mode {
	case ModeSwarmd:
		if client == nil {
			return ErrSwarmdUnavailable
		}
		_, err := client.Ping(ctx)
		if err != nil {
			return e.handleSwarmdError(err)
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

// SwarmdClient returns the swarmd client if available.
// Returns nil if swarmd is not connected.
func (e *NodeExecutor) SwarmdClient() *swarmd.Client {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.swarmdClient
}

// SSHExecutor returns the SSH executor.
func (e *NodeExecutor) SSHExecutor() ssh.Executor {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sshExecutor
}

// SpawnAgent spawns an agent using swarmd if available, otherwise returns an error.
func (e *NodeExecutor) SpawnAgent(ctx context.Context, req *swarmdv1.SpawnAgentRequest) (*swarmdv1.SpawnAgentResponse, error) {
	e.mu.RLock()
	client := e.swarmdClient
	mode := e.mode
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, ErrNodeClosed
	}

	if mode != ModeSwarmd || client == nil {
		return nil, ErrSwarmdUnavailable
	}

	resp, err := client.SpawnAgent(ctx, req)
	if err != nil {
		return nil, e.handleSwarmdError(err)
	}
	return resp, nil
}

// KillAgent kills an agent using swarmd if available.
func (e *NodeExecutor) KillAgent(ctx context.Context, req *swarmdv1.KillAgentRequest) (*swarmdv1.KillAgentResponse, error) {
	e.mu.RLock()
	client := e.swarmdClient
	mode := e.mode
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, ErrNodeClosed
	}

	if mode != ModeSwarmd || client == nil {
		return nil, ErrSwarmdUnavailable
	}

	resp, err := client.KillAgent(ctx, req)
	if err != nil {
		return nil, e.handleSwarmdError(err)
	}
	return resp, nil
}

// CapturePane captures pane content using swarmd if available.
func (e *NodeExecutor) CapturePane(ctx context.Context, req *swarmdv1.CapturePaneRequest) (*swarmdv1.CapturePaneResponse, error) {
	e.mu.RLock()
	client := e.swarmdClient
	mode := e.mode
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, ErrNodeClosed
	}

	if mode != ModeSwarmd || client == nil {
		return nil, ErrSwarmdUnavailable
	}

	resp, err := client.CapturePane(ctx, req)
	if err != nil {
		return nil, e.handleSwarmdError(err)
	}
	return resp, nil
}

// SendInput sends input to an agent using swarmd if available.
func (e *NodeExecutor) SendInput(ctx context.Context, req *swarmdv1.SendInputRequest) (*swarmdv1.SendInputResponse, error) {
	e.mu.RLock()
	client := e.swarmdClient
	mode := e.mode
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, ErrNodeClosed
	}

	if mode != ModeSwarmd || client == nil {
		return nil, ErrSwarmdUnavailable
	}

	resp, err := client.SendInput(ctx, req)
	if err != nil {
		return nil, e.handleSwarmdError(err)
	}
	return resp, nil
}

// ListAgents lists agents using swarmd if available.
func (e *NodeExecutor) ListAgents(ctx context.Context, req *swarmdv1.ListAgentsRequest) (*swarmdv1.ListAgentsResponse, error) {
	e.mu.RLock()
	client := e.swarmdClient
	mode := e.mode
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, ErrNodeClosed
	}

	if mode != ModeSwarmd || client == nil {
		return nil, ErrSwarmdUnavailable
	}

	resp, err := client.ListAgents(ctx, req)
	if err != nil {
		return nil, e.handleSwarmdError(err)
	}
	return resp, nil
}

// GetAgent gets agent details using swarmd if available.
func (e *NodeExecutor) GetAgent(ctx context.Context, req *swarmdv1.GetAgentRequest) (*swarmdv1.GetAgentResponse, error) {
	e.mu.RLock()
	client := e.swarmdClient
	mode := e.mode
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return nil, ErrNodeClosed
	}

	if mode != ModeSwarmd || client == nil {
		return nil, ErrSwarmdUnavailable
	}

	resp, err := client.GetAgent(ctx, req)
	if err != nil {
		return nil, e.handleSwarmdError(err)
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

	if e.swarmdClient != nil {
		if err := e.swarmdClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close swarmd client: %w", err))
		}
		e.swarmdClient = nil
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
