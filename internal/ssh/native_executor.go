package ssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/opencode-ai/swarm/internal/logging"
	"github.com/rs/zerolog"
	xssh "golang.org/x/crypto/ssh"
)

// NativeExecutor uses golang.org/x/crypto/ssh to run commands on remote nodes.
type NativeExecutor struct {
	options ConnectionOptions
	config  *xssh.ClientConfig
	pool    *connectionPool
	logger  zerolog.Logger
	agent   *AgentConnection

	// Callback for passphrase prompting
	PassphrasePrompt PassphrasePrompt

	// HostKeyPrompt is used when an unknown host key is encountered.
	HostKeyPrompt HostKeyPrompt

	// KeepAliveInterval is the interval for sending keep-alive requests.
	// Zero disables keep-alive.
	KeepAliveInterval time.Duration

	// KeepAliveTimeout is how long to wait for keep-alive response.
	KeepAliveTimeout time.Duration

	knownHostsFiles []string
	knownHostsPath  string
}

// NativeExecutorOption configures a NativeExecutor.
type NativeExecutorOption func(*NativeExecutor)

// WithPassphrasePrompt sets the passphrase prompt callback.
func WithPassphrasePrompt(prompt PassphrasePrompt) NativeExecutorOption {
	return func(e *NativeExecutor) {
		e.PassphrasePrompt = prompt
	}
}

// WithHostKeyPrompt sets the host key prompt callback.
func WithHostKeyPrompt(prompt HostKeyPrompt) NativeExecutorOption {
	return func(e *NativeExecutor) {
		e.HostKeyPrompt = prompt
	}
}

// WithKnownHostsFiles sets the known_hosts files to load and the primary write path.
func WithKnownHostsFiles(paths ...string) NativeExecutorOption {
	return func(e *NativeExecutor) {
		var files []string
		for _, path := range paths {
			if strings.TrimSpace(path) == "" {
				continue
			}
			files = append(files, path)
		}
		e.knownHostsFiles = files
		if len(files) > 0 {
			e.knownHostsPath = files[0]
		}
	}
}

// WithKeepAlive enables SSH keep-alive with the given interval and timeout.
func WithKeepAlive(interval, timeout time.Duration) NativeExecutorOption {
	return func(e *NativeExecutor) {
		e.KeepAliveInterval = interval
		e.KeepAliveTimeout = timeout
	}
}

// WithPoolSize sets the maximum number of pooled connections.
func WithPoolSize(size int) NativeExecutorOption {
	return func(e *NativeExecutor) {
		e.pool.maxSize = size
	}
}

// NewNativeExecutor creates a new NativeExecutor with the given options.
func NewNativeExecutor(options ConnectionOptions, opts ...NativeExecutorOption) (*NativeExecutor, error) {
	if options.Host == "" {
		return nil, ErrMissingHost
	}

	e := &NativeExecutor{
		options:           options,
		logger:            logging.Component("ssh"),
		KeepAliveInterval: 30 * time.Second,
		KeepAliveTimeout:  15 * time.Second,
		pool: &connectionPool{
			maxSize: 5,
			conns:   make(map[string]*pooledConn),
		},
	}

	for _, opt := range opts {
		opt(e)
	}

	// Build SSH client config
	config, err := e.buildConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build SSH config: %w", err)
	}
	e.config = config

	return e, nil
}

// buildConfig creates the SSH client configuration.
func (e *NativeExecutor) buildConfig() (*xssh.ClientConfig, error) {
	var authMethods []xssh.AuthMethod

	// Try ssh-agent first
	agentConn, err := ConnectAgent()
	if err == nil {
		e.agent = agentConn
		authMethods = append(authMethods, agentConn.AuthMethod())
		e.logger.Debug().Msg("using ssh-agent for authentication")
	}

	// Try private key if specified
	if e.options.KeyPath != "" {
		signer, err := LoadPrivateKey(e.options.KeyPath, e.PassphrasePrompt)
		if err != nil {
			// Only error if we have no other auth methods
			if len(authMethods) == 0 {
				return nil, fmt.Errorf("failed to load private key: %w", err)
			}
			e.logger.Warn().Err(err).Str("key_path", e.options.KeyPath).Msg("failed to load private key, falling back to agent")
		} else {
			authMethods = append(authMethods, xssh.PublicKeys(signer))
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication methods available")
	}

	timeout := e.options.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	hostKeyCallback, err := e.buildHostKeyCallback()
	if err != nil {
		return nil, fmt.Errorf("failed to load known_hosts: %w", err)
	}

	config := &xssh.ClientConfig{
		User:            e.options.User,
		Auth:            authMethods,
		Timeout:         timeout,
		HostKeyCallback: hostKeyCallback,
	}

	return config, nil
}

func (e *NativeExecutor) buildHostKeyCallback() (xssh.HostKeyCallback, error) {
	files := e.knownHostsFiles
	if len(files) == 0 {
		defaultFiles, err := defaultKnownHostsFiles()
		if err != nil {
			return nil, err
		}
		files = defaultFiles
	}

	writePath := e.knownHostsPath
	if writePath == "" && len(files) > 0 {
		writePath = files[0]
	}

	prompt := e.HostKeyPrompt
	if prompt == nil {
		prompt = DefaultHostKeyPrompt
	}

	return buildKnownHostsCallback(files, writePath, prompt, e.logger)
}

// Exec runs a command and returns its stdout and stderr output.
func (e *NativeExecutor) Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error) {
	client, release, err := e.getConnection(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	session, err := client.NewSession()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	// Handle context cancellation
	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case <-ctx.Done():
		session.Signal(xssh.SIGKILL)
		return nil, nil, ctx.Err()
	case err := <-done:
		stdout = stdoutBuf.Bytes()
		stderr = stderrBuf.Bytes()
		if err != nil {
			return stdout, stderr, wrapSSHError(err, cmd, stdout, stderr)
		}
		return stdout, stderr, nil
	}
}

// ExecInteractive runs a command, streaming stdin to the remote process.
func (e *NativeExecutor) ExecInteractive(ctx context.Context, cmd string, stdin io.Reader) error {
	client, release, err := e.getConnection(ctx)
	if err != nil {
		return err
	}
	defer release()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	session.Stdin = stdin

	// Handle context cancellation
	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case <-ctx.Done():
		session.Signal(xssh.SIGKILL)
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return wrapSSHError(err, cmd, nil, nil)
		}
		return nil
	}
}

// StartSession opens a long-lived SSH session.
func (e *NativeExecutor) StartSession() (Session, error) {
	client, release, err := e.getConnection(context.Background())
	if err != nil {
		return nil, err
	}

	return &NativeSession{
		executor: e,
		client:   client,
		release:  release,
	}, nil
}

// Close releases all resources held by the executor.
func (e *NativeExecutor) Close() error {
	err := e.pool.closeAll()
	if e.agent != nil {
		if agentErr := e.agent.Close(); err == nil {
			err = agentErr
		}
	}
	return err
}

// getConnection returns a pooled or new SSH client connection.
func (e *NativeExecutor) getConnection(ctx context.Context) (*xssh.Client, func(), error) {
	addr := e.targetAddr()

	// Try to get from pool
	if conn := e.pool.get(addr); conn != nil {
		e.logger.Debug().Str("addr", addr).Msg("reusing pooled connection")
		return conn, func() { e.pool.put(addr, conn) }, nil
	}

	// Create new connection
	client, err := e.dial(ctx)
	if err != nil {
		return nil, nil, err
	}

	e.logger.Debug().Str("addr", addr).Msg("created new connection")

	// Start keep-alive if enabled
	if e.KeepAliveInterval > 0 {
		go e.keepAlive(client)
	}

	return client, func() { e.pool.put(addr, client) }, nil
}

// dial establishes a new SSH connection.
func (e *NativeExecutor) dial(ctx context.Context) (*xssh.Client, error) {
	addr := e.targetAddr()

	// Handle ProxyJump if specified
	if e.options.ProxyJump != "" {
		return e.dialViaProxy(ctx)
	}

	// Direct connection
	dialer := &net.Dialer{
		Timeout: e.config.Timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	// SSH handshake
	sshConn, chans, reqs, err := xssh.NewClientConn(conn, addr, e.config)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SSH handshake failed: %w", err)
	}

	return xssh.NewClient(sshConn, chans, reqs), nil
}

// dialViaProxy connects through a ProxyJump host.
func (e *NativeExecutor) dialViaProxy(ctx context.Context) (*xssh.Client, error) {
	// Parse proxy jump
	proxyUser, proxyHost, proxyPort := parseSSHTarget(e.options.ProxyJump)
	if proxyPort == "" {
		proxyPort = "22"
	}

	proxyAddr := net.JoinHostPort(proxyHost, proxyPort)

	// Build proxy config (use same auth methods)
	proxyConfig := &xssh.ClientConfig{
		User:            proxyUser,
		Auth:            e.config.Auth,
		Timeout:         e.config.Timeout,
		HostKeyCallback: e.config.HostKeyCallback,
	}

	// Connect to proxy
	dialer := &net.Dialer{
		Timeout: e.config.Timeout,
	}

	proxyConn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to proxy %s: %w", proxyAddr, err)
	}

	proxySshConn, proxyChans, proxyReqs, err := xssh.NewClientConn(proxyConn, proxyAddr, proxyConfig)
	if err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("proxy SSH handshake failed: %w", err)
	}

	proxyClient := xssh.NewClient(proxySshConn, proxyChans, proxyReqs)

	// Connect to target through proxy
	targetAddr := e.targetAddr()
	targetConn, err := proxyClient.Dial("tcp", targetAddr)
	if err != nil {
		proxyClient.Close()
		return nil, fmt.Errorf("failed to connect to target %s via proxy: %w", targetAddr, err)
	}

	targetSshConn, targetChans, targetReqs, err := xssh.NewClientConn(targetConn, targetAddr, e.config)
	if err != nil {
		targetConn.Close()
		proxyClient.Close()
		return nil, fmt.Errorf("target SSH handshake failed: %w", err)
	}

	return xssh.NewClient(targetSshConn, targetChans, targetReqs), nil
}

// targetAddr returns the target address as host:port.
func (e *NativeExecutor) targetAddr() string {
	port := e.options.Port
	if port == 0 {
		port = 22
	}
	return net.JoinHostPort(e.options.Host, fmt.Sprintf("%d", port))
}

// keepAlive sends periodic keep-alive requests to the server.
func (e *NativeExecutor) keepAlive(client *xssh.Client) {
	ticker := time.NewTicker(e.KeepAliveInterval)
	defer ticker.Stop()

	for range ticker.C {
		// Send keep-alive request
		_, _, err := client.SendRequest("keepalive@swarm", true, nil)
		if err != nil {
			e.logger.Debug().Err(err).Msg("keep-alive failed, connection may be dead")
			return
		}
	}
}

// ParseSSHTarget parses a user@host:port string into ConnectionOptions.
// This is the public API for parsing SSH targets.
func ParseSSHTarget(target string) (*ConnectionOptions, error) {
	user, host, port := parseSSHTarget(target)
	if host == "" {
		return nil, fmt.Errorf("invalid SSH target: missing host")
	}

	opts := &ConnectionOptions{
		Host: host,
		User: user,
	}

	if port != "" {
		var p int
		if _, err := fmt.Sscanf(port, "%d", &p); err != nil {
			return nil, fmt.Errorf("invalid port: %s", port)
		}
		opts.Port = p
	}

	return opts, nil
}

// parseSSHTarget parses a user@host:port string (internal helper).
func parseSSHTarget(target string) (user, host, port string) {
	// Extract user
	if at := bytes.IndexByte([]byte(target), '@'); at >= 0 {
		user = target[:at]
		target = target[at+1:]
	}

	// Extract port
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		host = target
	}

	return user, host, port
}

// wrapSSHError wraps an SSH error with additional context.
func wrapSSHError(err error, cmd string, stdout, stderr []byte) error {
	if exitErr, ok := err.(*xssh.ExitError); ok {
		return &ExecError{
			Command:  cmd,
			ExitCode: exitErr.ExitStatus(),
			Stdout:   stdout,
			Stderr:   stderr,
			Err:      err,
		}
	}
	return err
}

// NativeSession wraps a persistent SSH connection for multiple commands.
type NativeSession struct {
	executor *NativeExecutor
	client   *xssh.Client
	release  func()
}

// Exec runs a command and returns its stdout and stderr output.
func (s *NativeSession) Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error) {
	session, err := s.client.NewSession()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case <-ctx.Done():
		session.Signal(xssh.SIGKILL)
		return nil, nil, ctx.Err()
	case err := <-done:
		stdout = stdoutBuf.Bytes()
		stderr = stderrBuf.Bytes()
		if err != nil {
			return stdout, stderr, wrapSSHError(err, cmd, stdout, stderr)
		}
		return stdout, stderr, nil
	}
}

// ExecInteractive runs a command, streaming stdin to the remote process.
func (s *NativeSession) ExecInteractive(ctx context.Context, cmd string, stdin io.Reader) error {
	session, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	session.Stdin = stdin

	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case <-ctx.Done():
		session.Signal(xssh.SIGKILL)
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return wrapSSHError(err, cmd, nil, nil)
		}
		return nil
	}
}

// Close ends the session and returns the connection to the pool.
func (s *NativeSession) Close() error {
	if s.release != nil {
		s.release()
	}
	return nil
}

// connectionPool manages a pool of SSH connections.
type connectionPool struct {
	mu      sync.Mutex
	maxSize int
	conns   map[string]*pooledConn
}

type pooledConn struct {
	client   *xssh.Client
	lastUsed time.Time
}

// get retrieves a connection from the pool.
func (p *connectionPool) get(addr string) *xssh.Client {
	p.mu.Lock()
	defer p.mu.Unlock()

	if pc, ok := p.conns[addr]; ok {
		delete(p.conns, addr)
		// Check if connection is still alive
		_, _, err := pc.client.SendRequest("keepalive@swarm", true, nil)
		if err != nil {
			pc.client.Close()
			return nil
		}
		return pc.client
	}
	return nil
}

// put returns a connection to the pool.
func (p *connectionPool) put(addr string, client *xssh.Client) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Evict oldest if at capacity
	if len(p.conns) >= p.maxSize {
		var oldestAddr string
		var oldestTime time.Time
		for a, pc := range p.conns {
			if oldestAddr == "" || pc.lastUsed.Before(oldestTime) {
				oldestAddr = a
				oldestTime = pc.lastUsed
			}
		}
		if oldestAddr != "" {
			if pc, ok := p.conns[oldestAddr]; ok {
				pc.client.Close()
				delete(p.conns, oldestAddr)
			}
		}
	}

	p.conns[addr] = &pooledConn{
		client:   client,
		lastUsed: time.Now(),
	}
}

// closeAll closes all pooled connections.
func (p *connectionPool) closeAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for addr, pc := range p.conns {
		if err := pc.client.Close(); err != nil {
			lastErr = err
		}
		delete(p.conns, addr)
	}
	return lastErr
}
