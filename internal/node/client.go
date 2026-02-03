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
	"github.com/tOgg1/forge/internal/tmux"
)

// Common client errors
var (
	ErrNoConnection    = errors.New("no connection to node")
	ErrDaemonNotFound  = errors.New("forged daemon not available")
	ErrAgentNotFound   = errors.New("agent not found")
	ErrOperationFailed = errors.New("operation failed")
)

// ClientMode indicates how the client connects to the node.
type ClientMode string

const (
	// ClientModeAuto automatically selects the best mode.
	ClientModeAuto ClientMode = "auto"
	// ClientModeDaemon always uses the forged daemon (fails if unavailable).
	ClientModeDaemon ClientMode = "daemon"
	// ClientModeSSH always uses SSH commands (no daemon).
	ClientModeSSH ClientMode = "ssh"
)

// Client provides a unified interface for interacting with a node,
// abstracting the difference between forged daemon mode and SSH-only mode.
type Client struct {
	node   *models.Node
	logger zerolog.Logger
	mode   ClientMode

	// Daemon connection (nil if using SSH mode)
	daemonClient *forged.Client

	// SSH executor (for SSH mode or daemon unavailable)
	sshExecutor ssh.Executor
	tmuxClient  *tmux.Client

	mu     sync.RWMutex
	closed bool
}

// ClientOption configures a Client.
type ClientOption func(*clientOpts)

type clientOpts struct {
	logger          zerolog.Logger
	mode            ClientMode
	daemonPort      int
	sshOptions      []ssh.NativeExecutorOption //nolint:unused // reserved for future SSH options
	daemonTimeout   time.Duration
	preferDaemon    bool
	sshExecutorFunc func(*models.Node) (ssh.Executor, error)
}

// WithClientLogger sets the logger for the client.
func WithClientLogger(logger zerolog.Logger) ClientOption {
	return func(o *clientOpts) {
		o.logger = logger
	}
}

// WithClientMode sets the connection mode.
func WithClientMode(mode ClientMode) ClientOption {
	return func(o *clientOpts) {
		o.mode = mode
	}
}

// WithDaemonPort sets the forged daemon port.
func WithDaemonPort(port int) ClientOption {
	return func(o *clientOpts) {
		o.daemonPort = port
	}
}

// WithDaemonTimeout sets the timeout for connecting to the daemon.
func WithDaemonTimeout(timeout time.Duration) ClientOption {
	return func(o *clientOpts) {
		o.daemonTimeout = timeout
	}
}

// WithPreferDaemon sets whether to prefer daemon mode when available.
func WithPreferDaemon(prefer bool) ClientOption {
	return func(o *clientOpts) {
		o.preferDaemon = prefer
	}
}

// WithSSHExecutorFunc provides a custom function to create SSH executors.
func WithSSHExecutorFunc(fn func(*models.Node) (ssh.Executor, error)) ClientOption {
	return func(o *clientOpts) {
		o.sshExecutorFunc = fn
	}
}

// NewClient creates a new unified client for a node.
// It automatically detects whether to use forged daemon or SSH mode.
// The node's ExecutionMode and ForgedPort settings are used as defaults,
// but can be overridden via ClientOptions.
func NewClient(ctx context.Context, node *models.Node, opts ...ClientOption) (*Client, error) {
	if node == nil {
		return nil, errors.New("node is required")
	}

	// Apply defaults from node configuration
	defaultMode := ClientModeAuto
	switch node.ExecutionMode {
	case models.ExecutionModeForged:
		defaultMode = ClientModeDaemon
	case models.ExecutionModeSSH:
		defaultMode = ClientModeSSH
	case models.ExecutionModeAuto:
		defaultMode = ClientModeAuto
	}

	defaultPort := forged.DefaultPort
	if node.ForgedPort > 0 {
		defaultPort = node.ForgedPort
	}

	// Default preferDaemon based on node's ForgedEnabled setting
	preferDaemon := node.ForgedEnabled

	cfg := &clientOpts{
		logger:        zerolog.Nop(),
		mode:          defaultMode,
		daemonPort:    defaultPort,
		daemonTimeout: 5 * time.Second,
		preferDaemon:  preferDaemon,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	client := &Client{
		node:   node,
		logger: cfg.logger,
		mode:   cfg.mode,
	}

	// Determine connection mode
	switch cfg.mode {
	case ClientModeDaemon:
		// Must use daemon - fail if unavailable
		if err := client.connectDaemon(ctx, cfg); err != nil {
			return nil, fmt.Errorf("daemon connection required but failed: %w", err)
		}
	case ClientModeSSH:
		// Always use SSH
		if err := client.connectSSH(ctx, node, cfg); err != nil {
			return nil, fmt.Errorf("SSH connection failed: %w", err)
		}
	case ClientModeAuto:
		// Try daemon first if preferred, fall back to SSH
		if cfg.preferDaemon {
			if err := client.connectDaemon(ctx, cfg); err != nil {
				client.logger.Debug().Err(err).Msg("daemon unavailable, falling back to SSH")
				if err := client.connectSSH(ctx, node, cfg); err != nil {
					return nil, fmt.Errorf("no connection method available: %w", err)
				}
			}
		} else {
			// Try SSH first
			if err := client.connectSSH(ctx, node, cfg); err != nil {
				client.logger.Debug().Err(err).Msg("SSH failed, trying daemon")
				if err := client.connectDaemon(ctx, cfg); err != nil {
					return nil, fmt.Errorf("no connection method available: %w", err)
				}
			}
		}
	}

	client.logger.Info().
		Str("node", node.Name).
		Bool("daemon", client.daemonClient != nil).
		Msg("node client connected")

	return client, nil
}

func (c *Client) connectDaemon(ctx context.Context, cfg *clientOpts) error {
	// Create a timeout context for daemon connection
	dialCtx, cancel := context.WithTimeout(ctx, cfg.daemonTimeout)
	defer cancel()

	var daemonClient *forged.Client
	var err error

	if c.node.IsLocal {
		// Direct connection for local node
		target := fmt.Sprintf("127.0.0.1:%d", cfg.daemonPort)
		daemonClient, err = forged.Dial(dialCtx, target, forged.WithLogger(c.logger))
	} else {
		// SSH tunnel for remote node
		user, host, port := ParseSSHTarget(c.node.SSHTarget)
		_ = user // SSH executor handles user
		daemonClient, err = forged.DialSSH(dialCtx, host, port, cfg.daemonPort, forged.WithLogger(c.logger))
	}

	if err != nil {
		return err
	}

	// Verify daemon is responsive
	pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pingCancel()

	if _, err := daemonClient.Ping(pingCtx); err != nil {
		_ = daemonClient.Close()
		c.node.ForgedAvailable = false
		return fmt.Errorf("daemon ping failed: %w", err)
	}

	c.daemonClient = daemonClient
	c.node.ForgedAvailable = true
	return nil
}

func (c *Client) connectSSH(ctx context.Context, node *models.Node, cfg *clientOpts) error {
	var executor ssh.Executor
	var err error

	if cfg.sshExecutorFunc != nil {
		executor, err = cfg.sshExecutorFunc(node)
	} else if node.IsLocal {
		executor = ssh.NewLocalExecutor()
	} else {
		user, host, port := ParseSSHTarget(node.SSHTarget)
		opts := ssh.ConnectionOptions{
			Host:    host,
			Port:    port,
			User:    user,
			KeyPath: node.SSHKeyPath,
		}
		executor, err = ssh.NewNativeExecutor(opts)
	}

	if err != nil {
		return err
	}

	// Verify SSH connection works
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stdout, _, err := executor.Exec(testCtx, "echo ok")
	if err != nil {
		_ = executor.Close()
		return fmt.Errorf("SSH test failed: %w", err)
	}

	if string(stdout) != "ok\n" && string(stdout) != "ok" {
		_ = executor.Close()
		return fmt.Errorf("unexpected SSH response: %q", string(stdout))
	}

	c.sshExecutor = executor
	c.tmuxClient = tmux.NewClient(&sshTmuxExecutor{executor: executor})

	return nil
}

// IsDaemonMode returns true if the client is using the forged daemon.
func (c *Client) IsDaemonMode() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.daemonClient != nil
}

// IsSSHMode returns true if the client is using SSH-only mode.
func (c *Client) IsSSHMode() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sshExecutor != nil && c.daemonClient == nil
}

// Mode returns the current connection mode.
func (c *Client) Mode() string {
	if c.IsDaemonMode() {
		return "daemon"
	}
	return "ssh"
}

// Node returns the node this client is connected to.
func (c *Client) Node() *models.Node {
	return c.node
}

// DaemonClient returns the underlying forged client, or nil if not in daemon mode.
func (c *Client) DaemonClient() *forged.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.daemonClient
}

// =============================================================================
// Agent Operations
// =============================================================================

// SpawnAgentRequest contains parameters for spawning an agent.
type SpawnAgentRequest struct {
	AgentID     string
	WorkspaceID string
	Command     string
	Args        []string
	Env         map[string]string
	WorkingDir  string
	SessionName string
	Adapter     string
}

// SpawnAgentResponse contains the result of spawning an agent.
type SpawnAgentResponse struct {
	AgentID string
	PaneID  string
}

// SpawnAgent creates a new agent on the node.
func (c *Client) SpawnAgent(ctx context.Context, req *SpawnAgentRequest) (*SpawnAgentResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.daemonClient != nil {
		// Use daemon
		resp, err := c.daemonClient.SpawnAgent(ctx, &forgedv1.SpawnAgentRequest{
			AgentId:     req.AgentID,
			WorkspaceId: req.WorkspaceID,
			Command:     req.Command,
			Args:        req.Args,
			Env:         req.Env,
			WorkingDir:  req.WorkingDir,
			SessionName: req.SessionName,
			Adapter:     req.Adapter,
		})
		if err != nil {
			return nil, err
		}
		return &SpawnAgentResponse{
			AgentID: resp.Agent.Id,
			PaneID:  resp.PaneId,
		}, nil
	}

	// Use SSH/tmux
	return c.spawnAgentSSH(ctx, req)
}

func (c *Client) spawnAgentSSH(ctx context.Context, req *SpawnAgentRequest) (*SpawnAgentResponse, error) {
	sessionName := req.SessionName
	if sessionName == "" {
		sessionName = fmt.Sprintf("forge-%s", req.WorkspaceID)
	}

	workDir := req.WorkingDir
	if workDir == "" {
		workDir = "~"
	}

	// Ensure session exists
	hasSession, err := c.tmuxClient.HasSession(ctx, sessionName)
	if err != nil {
		return nil, fmt.Errorf("failed to check session: %w", err)
	}
	if !hasSession {
		if err := c.tmuxClient.NewSession(ctx, sessionName, workDir); err != nil {
			return nil, fmt.Errorf("failed to create session: %w", err)
		}
	}

	// Create pane
	paneID, err := c.tmuxClient.SplitWindow(ctx, sessionName, true, workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create pane: %w", err)
	}

	// Set env vars
	for k, v := range req.Env {
		envCmd := fmt.Sprintf("export %s=%q", k, v)
		if err := c.tmuxClient.SendKeys(ctx, paneID, envCmd, true, true); err != nil {
			c.logger.Warn().Err(err).Str("pane", paneID).Msg("failed to set env var")
		}
	}

	// Build and send command
	cmdLine := req.Command
	for _, arg := range req.Args {
		cmdLine += " " + arg
	}

	if err := c.tmuxClient.SendKeys(ctx, paneID, cmdLine, true, true); err != nil {
		_ = c.tmuxClient.KillPane(ctx, paneID)
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	return &SpawnAgentResponse{
		AgentID: req.AgentID,
		PaneID:  paneID,
	}, nil
}

// KillAgent terminates an agent.
func (c *Client) KillAgent(ctx context.Context, agentID string, paneID string, force bool) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.daemonClient != nil {
		_, err := c.daemonClient.KillAgent(ctx, &forgedv1.KillAgentRequest{
			AgentId: agentID,
			Force:   force,
		})
		return err
	}

	// Use SSH/tmux
	if !force {
		if err := c.tmuxClient.SendInterrupt(ctx, paneID); err != nil {
			c.logger.Warn().Err(err).Str("pane", paneID).Msg("failed to send interrupt")
		}
		// Give it a moment to terminate gracefully
		time.Sleep(500 * time.Millisecond)
	}

	return c.tmuxClient.KillPane(ctx, paneID)
}

// SendInput sends text or keys to an agent.
func (c *Client) SendInput(ctx context.Context, agentID string, paneID string, text string, sendEnter bool, keys []string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.daemonClient != nil {
		_, err := c.daemonClient.SendInput(ctx, &forgedv1.SendInputRequest{
			AgentId:   agentID,
			Text:      text,
			SendEnter: sendEnter,
			Keys:      keys,
		})
		return err
	}

	// Use SSH/tmux
	// Send special keys first
	for _, key := range keys {
		keyCmd := fmt.Sprintf("tmux send-keys -t %s %s", paneID, key)
		if _, _, err := c.sshExecutor.Exec(ctx, keyCmd); err != nil {
			return fmt.Errorf("failed to send key %q: %w", key, err)
		}
	}

	// Send text
	if text != "" {
		return c.tmuxClient.SendKeys(ctx, paneID, text, true, sendEnter)
	}

	return nil
}

// CapturePane captures the content of an agent's pane.
func (c *Client) CapturePane(ctx context.Context, agentID string, paneID string, includeHistory bool) (string, string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.daemonClient != nil {
		lines := int32(0)
		if includeHistory {
			lines = -1
		}
		resp, err := c.daemonClient.CapturePane(ctx, &forgedv1.CapturePaneRequest{
			AgentId: agentID,
			Lines:   lines,
		})
		if err != nil {
			return "", "", err
		}
		return resp.Content, resp.ContentHash, nil
	}

	// Use SSH/tmux
	content, err := c.tmuxClient.CapturePane(ctx, paneID, includeHistory)
	if err != nil {
		return "", "", err
	}
	hash := tmux.HashSnapshot(content)
	return content, hash, nil
}

// =============================================================================
// Session Operations
// =============================================================================

// ListSessions returns all tmux sessions on the node.
func (c *Client) ListSessions(ctx context.Context) ([]tmux.Session, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.daemonClient != nil {
		// Daemon doesn't have a ListSessions RPC, fall back to tmux
		// This is fine since we still have SSH access
		if c.sshExecutor != nil && c.tmuxClient != nil {
			return c.tmuxClient.ListSessions(ctx)
		}
		// If no SSH executor, we can't list sessions
		return nil, errors.New("cannot list sessions: no SSH executor available")
	}

	return c.tmuxClient.ListSessions(ctx)
}

// ListPanes returns all panes in a session.
func (c *Client) ListPanes(ctx context.Context, session string) ([]tmux.Pane, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.tmuxClient == nil {
		return nil, errors.New("tmux client not available")
	}

	return c.tmuxClient.ListPanes(ctx, session)
}

// =============================================================================
// Health & Status
// =============================================================================

// Ping checks if the node is responsive.
func (c *Client) Ping(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.daemonClient != nil {
		_, err := c.daemonClient.Ping(ctx)
		return err
	}

	// Use SSH
	_, _, err := c.sshExecutor.Exec(ctx, "echo ok")
	return err
}

// GetDaemonStatus returns the forged daemon status, or nil if not in daemon mode.
func (c *Client) GetDaemonStatus(ctx context.Context) (*forgedv1.DaemonStatus, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.daemonClient == nil {
		return nil, ErrDaemonNotFound
	}

	resp, err := c.daemonClient.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	return resp.Status, nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	var errs []error

	if c.daemonClient != nil {
		if err := c.daemonClient.Close(); err != nil {
			errs = append(errs, err)
		}
		c.daemonClient = nil
	}

	if c.sshExecutor != nil {
		if err := c.sshExecutor.Close(); err != nil {
			errs = append(errs, err)
		}
		c.sshExecutor = nil
	}

	c.tmuxClient = nil

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// sshTmuxExecutor adapts an ssh.Executor to the tmux.Executor interface.
type sshTmuxExecutor struct {
	executor ssh.Executor
}

func (e *sshTmuxExecutor) Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error) {
	return e.executor.Exec(ctx, cmd)
}
