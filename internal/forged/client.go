// Package forged provides the daemon scaffolding and client for the Forge node service.
package forged

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/rs/zerolog"
	forgedv1 "github.com/tOgg1/forge/gen/forged/v1"
	"github.com/tOgg1/forge/internal/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client provides a gRPC client connection to a forged daemon.
// It supports both direct connections and connections over SSH tunnels.
type Client struct {
	conn   *grpc.ClientConn
	svc    forgedv1.ForgedServiceClient
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

// Dial creates a direct connection to a forged daemon.
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

	//nolint:staticcheck // grpc.DialContext remains supported across gRPC 1.x
	conn, err := grpc.DialContext(ctx, target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial forged at %s: %w", target, err)
	}

	cfg.logger.Debug().
		Str("target", target).
		Msg("connected to forged directly")

	return &Client{
		conn:   conn,
		svc:    forgedv1.NewForgedServiceClient(conn),
		logger: cfg.logger,
	}, nil
}

// DialSSH creates a connection to a remote forged daemon over an SSH tunnel.
// This is the recommended way to connect to forged on remote nodes.
func DialSSH(ctx context.Context, sshHost string, sshPort int, forgedPort int, opts ...ClientOption) (*Client, error) {
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
	if forgedPort <= 0 {
		forgedPort = DefaultPort
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

	// Start port forward to forged
	spec := ssh.PortForwardSpec{
		LocalHost:  "127.0.0.1",
		LocalPort:  0, // Let the system assign a free port
		RemoteHost: "127.0.0.1",
		RemotePort: forgedPort,
	}

	tunnel, err := executor.StartPortForward(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to start SSH tunnel: %w", err)
	}

	// Connect to forged through the tunnel
	localAddr := tunnel.LocalAddr()
	cfg.logger.Debug().
		Str("ssh_host", sshHost).
		Int("ssh_port", sshPort).
		Int("forged_port", forgedPort).
		Str("local_addr", localAddr).
		Msg("SSH tunnel established")

	dialOpts := append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}, cfg.dialOpts...)

	//nolint:staticcheck // grpc.DialContext remains supported across gRPC 1.x
	conn, err := grpc.DialContext(ctx, localAddr, dialOpts...)
	if err != nil {
		_ = tunnel.Close()
		return nil, fmt.Errorf("failed to dial forged through tunnel: %w", err)
	}

	cfg.logger.Info().
		Str("ssh_host", sshHost).
		Str("local_addr", localAddr).
		Msg("connected to forged via SSH tunnel")

	return &Client{
		conn:   conn,
		svc:    forgedv1.NewForgedServiceClient(conn),
		tunnel: tunnel,
		logger: cfg.logger,
	}, nil
}

// DialSSHWithExecutor creates a connection using an existing SSH executor.
// This is useful when you want to reuse an SSH connection or have custom SSH configuration.
func DialSSHWithExecutor(ctx context.Context, executor ssh.PortForwarder, forgedPort int, opts ...ClientOption) (*Client, error) {
	cfg := &clientConfig{
		logger: zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if executor == nil {
		return nil, fmt.Errorf("SSH executor is required")
	}
	if forgedPort <= 0 {
		forgedPort = DefaultPort
	}

	// Start port forward to forged
	spec := ssh.PortForwardSpec{
		LocalHost:  "127.0.0.1",
		LocalPort:  0,
		RemoteHost: "127.0.0.1",
		RemotePort: forgedPort,
	}

	tunnel, err := executor.StartPortForward(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to start SSH tunnel: %w", err)
	}

	localAddr := tunnel.LocalAddr()
	cfg.logger.Debug().
		Str("local_addr", localAddr).
		Int("forged_port", forgedPort).
		Msg("SSH tunnel established via executor")

	dialOpts := append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}, cfg.dialOpts...)

	//nolint:staticcheck // grpc.DialContext remains supported across gRPC 1.x
	conn, err := grpc.DialContext(ctx, localAddr, dialOpts...)
	if err != nil {
		_ = tunnel.Close()
		return nil, fmt.Errorf("failed to dial forged through tunnel: %w", err)
	}

	return &Client{
		conn:   conn,
		svc:    forgedv1.NewForgedServiceClient(conn),
		tunnel: tunnel,
		logger: cfg.logger,
	}, nil
}

// Service returns the underlying gRPC service client.
func (c *Client) Service() forgedv1.ForgedServiceClient {
	return c.svc
}

// Ping checks if the daemon is responsive.
func (c *Client) Ping(ctx context.Context) (*forgedv1.PingResponse, error) {
	return c.svc.Ping(ctx, &forgedv1.PingRequest{})
}

// GetStatus returns the daemon's status.
func (c *Client) GetStatus(ctx context.Context) (*forgedv1.GetStatusResponse, error) {
	return c.svc.GetStatus(ctx, &forgedv1.GetStatusRequest{})
}

// SpawnAgent creates a new agent in a tmux pane.
func (c *Client) SpawnAgent(ctx context.Context, req *forgedv1.SpawnAgentRequest) (*forgedv1.SpawnAgentResponse, error) {
	return c.svc.SpawnAgent(ctx, req)
}

// KillAgent terminates an agent.
func (c *Client) KillAgent(ctx context.Context, req *forgedv1.KillAgentRequest) (*forgedv1.KillAgentResponse, error) {
	return c.svc.KillAgent(ctx, req)
}

// ListAgents returns all agents.
func (c *Client) ListAgents(ctx context.Context, req *forgedv1.ListAgentsRequest) (*forgedv1.ListAgentsResponse, error) {
	return c.svc.ListAgents(ctx, req)
}

// GetAgent returns details for a specific agent.
func (c *Client) GetAgent(ctx context.Context, req *forgedv1.GetAgentRequest) (*forgedv1.GetAgentResponse, error) {
	return c.svc.GetAgent(ctx, req)
}

// SendInput sends text or keys to an agent.
func (c *Client) SendInput(ctx context.Context, req *forgedv1.SendInputRequest) (*forgedv1.SendInputResponse, error) {
	return c.svc.SendInput(ctx, req)
}

// CapturePane captures the content of an agent's pane.
func (c *Client) CapturePane(ctx context.Context, req *forgedv1.CapturePaneRequest) (*forgedv1.CapturePaneResponse, error) {
	return c.svc.CapturePane(ctx, req)
}

// StreamPaneUpdates streams pane content updates.
func (c *Client) StreamPaneUpdates(ctx context.Context, req *forgedv1.StreamPaneUpdatesRequest) (forgedv1.ForgedService_StreamPaneUpdatesClient, error) {
	return c.svc.StreamPaneUpdates(ctx, req)
}

// StreamEvents streams daemon events.
func (c *Client) StreamEvents(ctx context.Context, req *forgedv1.StreamEventsRequest) (forgedv1.ForgedService_StreamEventsClient, error) {
	return c.svc.StreamEvents(ctx, req)
}

// GetTranscript retrieves an agent's transcript.
func (c *Client) GetTranscript(ctx context.Context, req *forgedv1.GetTranscriptRequest) (*forgedv1.GetTranscriptResponse, error) {
	return c.svc.GetTranscript(ctx, req)
}

// StreamTranscript streams transcript updates.
func (c *Client) StreamTranscript(ctx context.Context, req *forgedv1.StreamTranscriptRequest) (forgedv1.ForgedService_StreamTranscriptClient, error) {
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
	logger     zerolog.Logger //nolint:unused // reserved for future logging
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

var (
	_ = (*sshDialer)(nil)
	_ = (*sshDialer).DialContext
	_ = (*tunneledConn)(nil)
	_ = (*tunneledConn).Close
)
