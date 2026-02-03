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

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/logging"
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

// WithPoolMaxPerHost sets the maximum number of pooled connections per host.
func WithPoolMaxPerHost(size int) NativeExecutorOption {
	return func(e *NativeExecutor) {
		e.pool.maxPerHost = size
	}
}

// WithPoolIdleTimeout sets how long an idle connection stays in the pool.
func WithPoolIdleTimeout(timeout time.Duration) NativeExecutorOption {
	return func(e *NativeExecutor) {
		e.pool.idleTimeout = timeout
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
			maxSize:     5,
			maxPerHost:  1,
			idleTimeout: 5 * time.Minute,
			conns:       make(map[string][]*pooledConn),
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
		if err := session.Signal(xssh.SIGKILL); err != nil {
			e.logger.Debug().Err(err).Msg("failed to signal ssh session")
		}
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
		if err := session.Signal(xssh.SIGKILL); err != nil {
			e.logger.Debug().Err(err).Msg("failed to signal ssh session")
		}
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
	jumps := parseProxyJumpList(e.options.ProxyJump)
	if len(jumps) == 0 {
		return e.dial(ctx)
	}

	dialer := &net.Dialer{
		Timeout: e.config.Timeout,
	}

	var proxyClients []*xssh.Client
	var prevClient *xssh.Client

	for i, jump := range jumps {
		proxyUser, proxyHost, proxyPort := parseSSHTarget(jump)
		if proxyHost == "" {
			closeProxyClients(proxyClients)
			return nil, fmt.Errorf("invalid proxy jump entry: %q", jump)
		}
		if proxyUser == "" {
			proxyUser = e.config.User
		}
		if proxyPort == "" {
			proxyPort = "22"
		}

		proxyAddr := net.JoinHostPort(proxyHost, proxyPort)
		proxyConfig := &xssh.ClientConfig{
			User:            proxyUser,
			Auth:            e.config.Auth,
			Timeout:         e.config.Timeout,
			HostKeyCallback: e.config.HostKeyCallback,
		}

		var conn net.Conn
		var err error
		if i == 0 {
			conn, err = dialer.DialContext(ctx, "tcp", proxyAddr)
		} else {
			conn, err = prevClient.Dial("tcp", proxyAddr)
		}
		if err != nil {
			closeProxyClients(proxyClients)
			return nil, fmt.Errorf("failed to connect to proxy %s: %w", proxyAddr, err)
		}

		proxySshConn, proxyChans, proxyReqs, err := xssh.NewClientConn(conn, proxyAddr, proxyConfig)
		if err != nil {
			conn.Close()
			closeProxyClients(proxyClients)
			return nil, fmt.Errorf("proxy SSH handshake failed: %w", err)
		}

		proxyClient := xssh.NewClient(proxySshConn, proxyChans, proxyReqs)
		proxyClients = append(proxyClients, proxyClient)
		prevClient = proxyClient
	}

	targetAddr := e.targetAddr()
	targetConn, err := prevClient.Dial("tcp", targetAddr)
	if err != nil {
		closeProxyClients(proxyClients)
		return nil, fmt.Errorf("failed to connect to target %s via proxy: %w", targetAddr, err)
	}

	closeFn := func() {
		closeProxyClients(proxyClients)
	}
	wrappedConn := &proxyConn{Conn: targetConn, onClose: closeFn}

	targetSshConn, targetChans, targetReqs, err := xssh.NewClientConn(wrappedConn, targetAddr, e.config)
	if err != nil {
		_ = wrappedConn.Close()
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
		_, _, err := client.SendRequest("keepalive@forge", true, nil)
		if err != nil {
			e.logger.Debug().Err(err).Msg("keep-alive failed, connection may be dead")
			return
		}
	}
}

type proxyConn struct {
	net.Conn
	onClose func()
}

func (c *proxyConn) Close() error {
	err := c.Conn.Close()
	if c.onClose != nil {
		c.onClose()
	}
	return err
}

func closeProxyClients(clients []*xssh.Client) {
	for i := len(clients) - 1; i >= 0; i-- {
		_ = clients[i].Close()
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
		if err := session.Signal(xssh.SIGKILL); err != nil {
			s.executor.logger.Debug().Err(err).Msg("failed to signal ssh session")
		}
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
		if err := session.Signal(xssh.SIGKILL); err != nil {
			s.executor.logger.Debug().Err(err).Msg("failed to signal ssh session")
		}
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
	mu          sync.Mutex
	maxSize     int
	maxPerHost  int
	idleTimeout time.Duration
	conns       map[string][]*pooledConn
	total       int
}

type pooledConn struct {
	client   *xssh.Client
	lastUsed time.Time
}

// get retrieves a connection from the pool.
func (p *connectionPool) get(addr string) *xssh.Client {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	p.pruneLocked(now)

	list, ok := p.conns[addr]
	if !ok || len(list) == 0 {
		return nil
	}

	for len(list) > 0 {
		idx := len(list) - 1
		pc := list[idx]
		list = list[:idx]
		p.total--

		if len(list) == 0 {
			delete(p.conns, addr)
		} else {
			p.conns[addr] = list
		}

		if pc == nil || pc.client == nil {
			continue
		}

		_, _, err := pc.client.SendRequest("keepalive@forge", true, nil)
		if err != nil {
			pc.client.Close()
			continue
		}
		return pc.client
	}

	return nil
}

// put returns a connection to the pool.
func (p *connectionPool) put(addr string, client *xssh.Client) {
	if client == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	p.pruneLocked(now)

	if p.maxPerHost > 0 {
		if list := p.conns[addr]; len(list) >= p.maxPerHost {
			p.evictOldestInHostLocked(addr)
		}
	}

	if p.maxSize > 0 {
		for p.total >= p.maxSize {
			p.evictOldestLocked()
			if p.total == 0 {
				break
			}
		}
	}

	list := p.conns[addr]
	list = append(list, &pooledConn{
		client:   client,
		lastUsed: now,
	})
	p.conns[addr] = list
	p.total++
}

// closeAll closes all pooled connections.
func (p *connectionPool) closeAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for addr, list := range p.conns {
		for _, pc := range list {
			if pc == nil || pc.client == nil {
				continue
			}
			if err := pc.client.Close(); err != nil {
				lastErr = err
			}
		}
		delete(p.conns, addr)
	}
	p.total = 0
	return lastErr
}

func (p *connectionPool) pruneLocked(now time.Time) {
	if p.idleTimeout <= 0 {
		return
	}

	for addr, list := range p.conns {
		if len(list) == 0 {
			delete(p.conns, addr)
			continue
		}
		kept := list[:0]
		for _, pc := range list {
			if pc == nil || pc.client == nil {
				p.total--
				continue
			}
			if now.Sub(pc.lastUsed) > p.idleTimeout {
				pc.client.Close()
				p.total--
				continue
			}
			kept = append(kept, pc)
		}
		if len(kept) == 0 {
			delete(p.conns, addr)
			continue
		}
		p.conns[addr] = kept
	}
}

func (p *connectionPool) evictOldestLocked() {
	var (
		oldestAddr  string
		oldestIndex int
		oldestTime  time.Time
	)
	for addr, list := range p.conns {
		for i, pc := range list {
			if pc == nil {
				continue
			}
			if oldestAddr == "" || pc.lastUsed.Before(oldestTime) {
				oldestAddr = addr
				oldestIndex = i
				oldestTime = pc.lastUsed
			}
		}
	}

	if oldestAddr == "" {
		return
	}

	list := p.conns[oldestAddr]
	if oldestIndex < 0 || oldestIndex >= len(list) {
		return
	}
	pc := list[oldestIndex]
	if pc != nil && pc.client != nil {
		pc.client.Close()
	}
	p.total--

	list = append(list[:oldestIndex], list[oldestIndex+1:]...)
	if len(list) == 0 {
		delete(p.conns, oldestAddr)
		return
	}
	p.conns[oldestAddr] = list
}

func (p *connectionPool) evictOldestInHostLocked(addr string) {
	list := p.conns[addr]
	if len(list) == 0 {
		return
	}

	oldestIndex := 0
	oldestTime := list[0].lastUsed
	for i, pc := range list {
		if pc == nil {
			continue
		}
		if pc.lastUsed.Before(oldestTime) {
			oldestIndex = i
			oldestTime = pc.lastUsed
		}
	}

	pc := list[oldestIndex]
	if pc != nil && pc.client != nil {
		pc.client.Close()
	}
	p.total--

	list = append(list[:oldestIndex], list[oldestIndex+1:]...)
	if len(list) == 0 {
		delete(p.conns, addr)
		return
	}
	p.conns[addr] = list
}
