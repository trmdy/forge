package ssh

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestLocalExecutorExec(t *testing.T) {
	t.Parallel()

	exec := NewLocalExecutor()
	stdout, stderr, err := exec.Exec(context.Background(), "printf 'hello'")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if string(stdout) != "hello" {
		t.Fatalf("expected stdout to be %q, got %q", "hello", string(stdout))
	}
	if len(stderr) != 0 {
		t.Fatalf("expected empty stderr, got %q", string(stderr))
	}
}

func TestLocalExecutorExecInteractive(t *testing.T) {
	t.Parallel()

	exec := NewLocalExecutor()
	input := strings.NewReader("ping\n")
	if err := exec.ExecInteractive(context.Background(), `read -r line; [ "$line" = "ping" ]`, input); err != nil {
		t.Fatalf("expected interactive command to succeed, got %v", err)
	}
}

func TestLocalExecutorSession(t *testing.T) {
	t.Parallel()

	exec := NewLocalExecutor()
	session, err := exec.StartSession()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	defer session.Close()

	stdout, stderr, err := session.Exec(context.Background(), "printf 'ok'")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if string(stdout) != "ok" {
		t.Fatalf("expected stdout to be %q, got %q", "ok", string(stdout))
	}
	if len(stderr) != 0 {
		t.Fatalf("expected empty stderr, got %q", string(stderr))
	}
}

func TestLocalExecutorExecInteractiveStdin(t *testing.T) {
	t.Parallel()

	exec := NewLocalExecutor()
	input := bytes.NewBufferString("alpha\n")
	if err := exec.ExecInteractive(context.Background(), `read -r line; [ "$line" = "alpha" ]`, input); err != nil {
		t.Fatalf("expected interactive command to succeed, got %v", err)
	}
}
