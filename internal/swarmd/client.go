// Package swarmd provides the daemon scaffolding and client for the Swarm node service.
package swarmd

import (
	"context"
	"fmt"
	"net"
	"sync"

	swarmdv1 "github.com/opencode-ai/swarm/gen/swarmd/v1"
	"github.com/opencode-ai/swarm/internal/ssh"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client provides a gRPC client connection to a swarmd daemon.
// It supports both direct connections and connections over SSH tunnels.
type Client struct {
	conn   *grpc.ClientConn
	svc    swarmdv1.SwarmdServiceClient
	tunnel ssh.PortForward
	logger zerolog.Logger

	mu     sync.Mutex
	closed bool
}

// ClientOption configures a Client.
type ClientOption func(*clientConfig)

type clientConfig struct {
	logger     zerolog.Logger
	dialOpts   []grpc.DialOption
	sshOptions []ssh.NativeExecutorOption
}

// WithLogger sets the logger for the client.
func WithLogger(logger zerolog.Logger) ClientOption {
	return func(c *clientConfig) {
		c.logger = logger
	}
}

// WithDialOptions adds gRPC dial options.
func WithDialOptions(opts ...grpc.DialOption) ClientOption {
	return func(c *clientConfig) {
		c.dialOpts = append(c.dialOpts, opts...)
	}
}

// WithSSHOptions adds SSH executor options for tunnel connections.
func WithSSHOptions(opts ...ssh.NativeExecutorOption) ClientOption {
	return func(c *clientConfig) {
		c.sshOptions = append(c.sshOptions, opts...)
	}
}

// Dial creates a direct connection to a swarmd daemon.
// Use this for local connections or when the daemon is directly reachable.
func Dial(ctx context.Context, target string, opts ...ClientOption) (*Client, error) {
	cfg := &clientConfig{
		logger: zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	dialOpts := append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}, cfg.dialOpts...)

	conn, err := grpc.DialContext(ctx, target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial swarmd at %s: %w", target, err)
	}

	cfg.logger.Debug().
		Str("target", target).
		Msg("connected to swarmd directly")

	return &Client{
		conn:   conn,
		svc:    swarmdv1.NewSwarmdServiceClient(conn),
		logger: cfg.logger,
	}, nil
}

// DialSSH creates a connection to a remote swarmd daemon over an SSH tunnel.
// This is the recommended way to connect to swarmd on remote nodes.
func DialSSH(ctx context.Context, sshHost string, sshPort int, swarmdPort int, opts ...ClientOption) (*Client, error) {
	cfg := &clientConfig{
		logger: zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if sshHost == "" {
		return nil, fmt.Errorf("SSH host is required")
	}
	if sshPort <= 0 {
		sshPort = 22
	}
	if swarmdPort <= 0 {
		swarmdPort = DefaultPort
	}

	// Create SSH executor for the target host
	sshConnOpts := ssh.ConnectionOptions{
		Host: sshHost,
		Port: sshPort,
	}

	executor, err := ssh.NewNativeExecutor(sshConnOpts, cfg.sshOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH executor: %w", err)
	}

	// Start port forward to swarmd
	spec := ssh.PortForwardSpec{
		LocalHost:  "127.0.0.1",
		LocalPort:  0, // Let the system assign a free port
		RemoteHost: "127.0.0.1",
		RemotePort: swarmdPort,
	}

	tunnel, err := executor.StartPortForward(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to start SSH tunnel: %w", err)
	}

	// Connect to swarmd through the tunnel
	localAddr := tunnel.LocalAddr()
	cfg.logger.Debug().
		Str("ssh_host", sshHost).
		Int("ssh_port", sshPort).
		Int("swarmd_port", swarmdPort).
		Str("local_addr", localAddr).
		Msg("SSH tunnel established")

	dialOpts := append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}, cfg.dialOpts...)

	conn, err := grpc.DialContext(ctx, localAddr, dialOpts...)
	if err != nil {
		_ = tunnel.Close()
		return nil, fmt.Errorf("failed to dial swarmd through tunnel: %w", err)
	}

	cfg.logger.Info().
		Str("ssh_host", sshHost).
		Str("local_addr", localAddr).
		Msg("connected to swarmd via SSH tunnel")

	return &Client{
		conn:   conn,
		svc:    swarmdv1.NewSwarmdServiceClient(conn),
		tunnel: tunnel,
		logger: cfg.logger,
	}, nil
}

// DialSSHWithExecutor creates a connection using an existing SSH executor.
// This is useful when you want to reuse an SSH connection or have custom SSH configuration.
func DialSSHWithExecutor(ctx context.Context, executor ssh.PortForwarder, swarmdPort int, opts ...ClientOption) (*Client, error) {
	cfg := &clientConfig{
		logger: zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if executor == nil {
		return nil, fmt.Errorf("SSH executor is required")
	}
	if swarmdPort <= 0 {
		swarmdPort = DefaultPort
	}

	// Start port forward to swarmd
	spec := ssh.PortForwardSpec{
		LocalHost:  "127.0.0.1",
		LocalPort:  0,
		RemoteHost: "127.0.0.1",
		RemotePort: swarmdPort,
	}

	tunnel, err := executor.StartPortForward(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to start SSH tunnel: %w", err)
	}

	localAddr := tunnel.LocalAddr()
	cfg.logger.Debug().
		Str("local_addr", localAddr).
		Int("swarmd_port", swarmdPort).
		Msg("SSH tunnel established via executor")

	dialOpts := append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}, cfg.dialOpts...)

	conn, err := grpc.DialContext(ctx, localAddr, dialOpts...)
	if err != nil {
		_ = tunnel.Close()
		return nil, fmt.Errorf("failed to dial swarmd through tunnel: %w", err)
	}

	return &Client{
		conn:   conn,
		svc:    swarmdv1.NewSwarmdServiceClient(conn),
		tunnel: tunnel,
		logger: cfg.logger,
	}, nil
}

// Service returns the underlying gRPC service client.
func (c *Client) Service() swarmdv1.SwarmdServiceClient {
	return c.svc
}

// Ping checks if the daemon is responsive.
func (c *Client) Ping(ctx context.Context) (*swarmdv1.PingResponse, error) {
	return c.svc.Ping(ctx, &swarmdv1.PingRequest{})
}

// GetStatus returns the daemon's status.
func (c *Client) GetStatus(ctx context.Context) (*swarmdv1.GetStatusResponse, error) {
	return c.svc.GetStatus(ctx, &swarmdv1.GetStatusRequest{})
}

// SpawnAgent creates a new agent in a tmux pane.
func (c *Client) SpawnAgent(ctx context.Context, req *swarmdv1.SpawnAgentRequest) (*swarmdv1.SpawnAgentResponse, error) {
	return c.svc.SpawnAgent(ctx, req)
}

// KillAgent terminates an agent.
func (c *Client) KillAgent(ctx context.Context, req *swarmdv1.KillAgentRequest) (*swarmdv1.KillAgentResponse, error) {
	return c.svc.KillAgent(ctx, req)
}

// ListAgents returns all agents.
func (c *Client) ListAgents(ctx context.Context, req *swarmdv1.ListAgentsRequest) (*swarmdv1.ListAgentsResponse, error) {
	return c.svc.ListAgents(ctx, req)
}

// GetAgent returns details for a specific agent.
func (c *Client) GetAgent(ctx context.Context, req *swarmdv1.GetAgentRequest) (*swarmdv1.GetAgentResponse, error) {
	return c.svc.GetAgent(ctx, req)
}

// SendInput sends text or keys to an agent.
func (c *Client) SendInput(ctx context.Context, req *swarmdv1.SendInputRequest) (*swarmdv1.SendInputResponse, error) {
	return c.svc.SendInput(ctx, req)
}

// CapturePane captures the content of an agent's pane.
func (c *Client) CapturePane(ctx context.Context, req *swarmdv1.CapturePaneRequest) (*swarmdv1.CapturePaneResponse, error) {
	return c.svc.CapturePane(ctx, req)
}

// StreamPaneUpdates streams pane content updates.
func (c *Client) StreamPaneUpdates(ctx context.Context, req *swarmdv1.StreamPaneUpdatesRequest) (swarmdv1.SwarmdService_StreamPaneUpdatesClient, error) {
	return c.svc.StreamPaneUpdates(ctx, req)
}

// StreamEvents streams daemon events.
func (c *Client) StreamEvents(ctx context.Context, req *swarmdv1.StreamEventsRequest) (swarmdv1.SwarmdService_StreamEventsClient, error) {
	return c.svc.StreamEvents(ctx, req)
}

// GetTranscript retrieves an agent's transcript.
func (c *Client) GetTranscript(ctx context.Context, req *swarmdv1.GetTranscriptRequest) (*swarmdv1.GetTranscriptResponse, error) {
	return c.svc.GetTranscript(ctx, req)
}

// StreamTranscript streams transcript updates.
func (c *Client) StreamTranscript(ctx context.Context, req *swarmdv1.StreamTranscriptRequest) (swarmdv1.SwarmdService_StreamTranscriptClient, error) {
	return c.svc.StreamTranscript(ctx, req)
}

// LocalAddr returns the local address of the connection.
// For SSH tunnel connections, this is the local tunnel endpoint.
func (c *Client) LocalAddr() string {
	if c.tunnel != nil {
		return c.tunnel.LocalAddr()
	}
	return c.conn.Target()
}

// IsTunneled returns true if the connection goes through an SSH tunnel.
func (c *Client) IsTunneled() bool {
	return c.tunnel != nil
}

// Close closes the client connection and any SSH tunnel.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	var errs []error

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close gRPC connection: %w", err))
		}
	}

	if c.tunnel != nil {
		if err := c.tunnel.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close SSH tunnel: %w", err))
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// sshDialer implements a net.Conn dialer using SSH port forwarding.
// This allows using the SSH tunnel as a custom dialer for gRPC.
type sshDialer struct {
	forwarder  ssh.PortForwarder
	remotePort int
	logger     zerolog.Logger
}

// DialContext implements the grpc.WithContextDialer interface.
func (d *sshDialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	spec := ssh.PortForwardSpec{
		LocalHost:  "127.0.0.1",
		LocalPort:  0,
		RemoteHost: "127.0.0.1",
		RemotePort: d.remotePort,
	}

	tunnel, err := d.forwarder.StartPortForward(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create tunnel: %w", err)
	}

	// Connect to the local tunnel endpoint
	conn, err := net.Dial("tcp", tunnel.LocalAddr())
	if err != nil {
		_ = tunnel.Close()
		return nil, fmt.Errorf("failed to connect to tunnel: %w", err)
	}

	// Wrap connection to close tunnel when connection closes
	return &tunneledConn{
		Conn:   conn,
		tunnel: tunnel,
	}, nil
}

// tunneledConn wraps a net.Conn to close the SSH tunnel when the connection is closed.
type tunneledConn struct {
	net.Conn
	tunnel     ssh.PortForward
	closedOnce sync.Once
}

func (c *tunneledConn) Close() error {
	var connErr, tunnelErr error
	c.closedOnce.Do(func() {
		connErr = c.Conn.Close()
		if c.tunnel != nil {
			tunnelErr = c.tunnel.Close()
		}
	})
	if connErr != nil {
		return connErr
	}
	return tunnelErr
}
