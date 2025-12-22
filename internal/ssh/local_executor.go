package ssh

import (
	"bytes"
	"context"
	"io"
	"os/exec"
)

// LocalExecutor runs commands directly on the local machine.
type LocalExecutor struct{}

// NewLocalExecutor creates a new LocalExecutor.
func NewLocalExecutor() *LocalExecutor {
	return &LocalExecutor{}
}

// Exec runs a command and returns its stdout and stderr output.
func (e *LocalExecutor) Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error) {
	return e.exec(ctx, cmd, nil)
}

// ExecInteractive runs a command, streaming stdin to the local process.
func (e *LocalExecutor) ExecInteractive(ctx context.Context, cmd string, stdin io.Reader) error {
	_, _, err := e.exec(ctx, cmd, stdin)
	return err
}

// StartSession opens a session wrapper that reuses this executor.
func (e *LocalExecutor) StartSession() (Session, error) {
	return &LocalSession{executor: e}, nil
}

// Close releases any resources held by the executor.
func (e *LocalExecutor) Close() error {
	return nil
}

func (e *LocalExecutor) exec(ctx context.Context, cmd string, stdin io.Reader) ([]byte, []byte, error) {
	command := exec.CommandContext(ctx, "sh", "-c", cmd)
	if stdin != nil {
		command.Stdin = stdin
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	command.Stdout = &stdoutBuf
	command.Stderr = &stderrBuf

	err := command.Run()
	stdout := stdoutBuf.Bytes()
	stderr := stderrBuf.Bytes()
	if err != nil {
		return stdout, stderr, wrapExecError(err, cmd, stdout, stderr)
	}
	return stdout, stderr, nil
}

// LocalSession provides a session-style wrapper over LocalExecutor.
type LocalSession struct {
	executor *LocalExecutor
}

// Exec runs a command and returns its stdout and stderr output.
func (s *LocalSession) Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error) {
	return s.executor.Exec(ctx, cmd)
}

// ExecInteractive runs a command, streaming stdin to the local process.
func (s *LocalSession) ExecInteractive(ctx context.Context, cmd string, stdin io.Reader) error {
	return s.executor.ExecInteractive(ctx, cmd, stdin)
}

// Close ends the session (no-op for local execution).
func (s *LocalSession) Close() error {
	return nil
}
