// Package mocks provides shared mock implementations for testing.
package mocks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/opencode-ai/swarm/internal/ssh"
)

// SSHExecCall records a single call to Exec or ExecInteractive.
type SSHExecCall struct {
	Cmd   string
	Stdin []byte // Only populated for ExecInteractive
}

// SSHExecResponse defines a canned response for a command.
type SSHExecResponse struct {
	Stdout []byte
	Stderr []byte
	Err    error
}

// SSHExecutor is a mock implementation of ssh.Executor for testing.
type SSHExecutor struct {
	mu sync.Mutex

	// Calls records all commands executed.
	Calls []SSHExecCall

	// Responses maps command prefixes to canned responses.
	// If a command starts with a key, that response is returned.
	Responses map[string]SSHExecResponse

	// DefaultResponse is returned when no matching response is found.
	DefaultResponse SSHExecResponse

	// ResponseQueue is a FIFO queue of responses, used in order regardless of command.
	ResponseQueue []SSHExecResponse

	// ExecInteractiveError is returned by ExecInteractive when set.
	ExecInteractiveError error

	// Closed tracks whether Close was called.
	Closed bool
}

// NewSSHExecutor creates a new mock SSH executor.
func NewSSHExecutor() *SSHExecutor {
	return &SSHExecutor{
		Responses: make(map[string]SSHExecResponse),
	}
}

// Exec runs a command and returns a canned response.
func (m *SSHExecutor) Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Calls = append(m.Calls, SSHExecCall{Cmd: cmd})

	// Check context cancellation
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}

	// Return from queue if available
	if len(m.ResponseQueue) > 0 {
		resp := m.ResponseQueue[0]
		m.ResponseQueue = m.ResponseQueue[1:]
		return resp.Stdout, resp.Stderr, resp.Err
	}

	// Check for matching response
	for prefix, resp := range m.Responses {
		if strings.HasPrefix(cmd, prefix) {
			return resp.Stdout, resp.Stderr, resp.Err
		}
	}

	return m.DefaultResponse.Stdout, m.DefaultResponse.Stderr, m.DefaultResponse.Err
}

// ExecInteractive runs a command with stdin streaming.
func (m *SSHExecutor) ExecInteractive(ctx context.Context, cmd string, stdin io.Reader) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var stdinData []byte
	if stdin != nil {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		stdinData = data
	}

	m.Calls = append(m.Calls, SSHExecCall{Cmd: cmd, Stdin: stdinData})

	if m.ExecInteractiveError != nil {
		return m.ExecInteractiveError
	}

	// Check context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}

// StartSession returns a mock session that implements ssh.Session.
func (m *SSHExecutor) StartSession() (ssh.Session, error) {
	return &SSHSession{executor: m}, nil
}

// Close marks the executor as closed.
func (m *SSHExecutor) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Closed = true
	return nil
}

// Reset clears all recorded calls and responses.
func (m *SSHExecutor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = nil
	m.Responses = make(map[string]SSHExecResponse)
	m.ResponseQueue = nil
	m.DefaultResponse = SSHExecResponse{}
	m.ExecInteractiveError = nil
	m.Closed = false
}

// LastCall returns the most recent command executed.
func (m *SSHExecutor) LastCall() (SSHExecCall, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Calls) == 0 {
		return SSHExecCall{}, false
	}
	return m.Calls[len(m.Calls)-1], true
}

// CallCount returns the number of commands executed.
func (m *SSHExecutor) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls)
}

// SetResponse sets a canned response for commands starting with the given prefix.
func (m *SSHExecutor) SetResponse(cmdPrefix string, stdout, stderr []byte, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses[cmdPrefix] = SSHExecResponse{
		Stdout: stdout,
		Stderr: stderr,
		Err:    err,
	}
}

// QueueResponse adds a response to the queue.
func (m *SSHExecutor) QueueResponse(stdout, stderr []byte, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ResponseQueue = append(m.ResponseQueue, SSHExecResponse{
		Stdout: stdout,
		Stderr: stderr,
		Err:    err,
	})
}

// Session defines the interface for SSH sessions (matches ssh.Session).
type Session interface {
	Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error)
	ExecInteractive(ctx context.Context, cmd string, stdin io.Reader) error
	Close() error
}

// SSHSession is a mock SSH session.
type SSHSession struct {
	executor *SSHExecutor
	closed   bool
}

// Exec delegates to the parent executor.
func (s *SSHSession) Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error) {
	if s.closed {
		return nil, nil, errors.New("session closed")
	}
	return s.executor.Exec(ctx, cmd)
}

// ExecInteractive delegates to the parent executor.
func (s *SSHSession) ExecInteractive(ctx context.Context, cmd string, stdin io.Reader) error {
	if s.closed {
		return errors.New("session closed")
	}
	return s.executor.ExecInteractive(ctx, cmd, stdin)
}

// Close marks the session as closed.
func (s *SSHSession) Close() error {
	s.closed = true
	return nil
}
