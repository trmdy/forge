package mocks

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestSSHExecutor_Exec(t *testing.T) {
	exec := NewSSHExecutor()
	exec.SetResponse("echo", []byte("hello\n"), nil, nil)

	stdout, stderr, err := exec.Exec(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "hello\n" {
		t.Errorf("expected stdout 'hello\\n', got %q", string(stdout))
	}
	if len(stderr) != 0 {
		t.Errorf("expected empty stderr, got %q", string(stderr))
	}

	if exec.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", exec.CallCount())
	}
	call, ok := exec.LastCall()
	if !ok {
		t.Fatal("expected to have a last call")
	}
	if call.Cmd != "echo hello" {
		t.Errorf("expected cmd 'echo hello', got %q", call.Cmd)
	}
}

func TestSSHExecutor_ExecWithError(t *testing.T) {
	exec := NewSSHExecutor()
	expectedErr := errors.New("command failed")
	exec.SetResponse("fail", nil, []byte("error output"), expectedErr)

	stdout, stderr, err := exec.Exec(context.Background(), "fail now")
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if len(stdout) != 0 {
		t.Errorf("expected empty stdout, got %q", string(stdout))
	}
	if string(stderr) != "error output" {
		t.Errorf("expected stderr 'error output', got %q", string(stderr))
	}
}

func TestSSHExecutor_DefaultResponse(t *testing.T) {
	exec := NewSSHExecutor()
	exec.DefaultResponse = SSHExecResponse{
		Stdout: []byte("default"),
		Stderr: nil,
		Err:    nil,
	}

	stdout, _, err := exec.Exec(context.Background(), "unknown command")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "default" {
		t.Errorf("expected 'default', got %q", string(stdout))
	}
}

func TestSSHExecutor_ResponseQueue(t *testing.T) {
	exec := NewSSHExecutor()
	exec.QueueResponse([]byte("first"), nil, nil)
	exec.QueueResponse([]byte("second"), nil, nil)
	exec.QueueResponse([]byte("third"), nil, nil)

	// First call
	stdout, _, _ := exec.Exec(context.Background(), "cmd1")
	if string(stdout) != "first" {
		t.Errorf("expected 'first', got %q", string(stdout))
	}

	// Second call
	stdout, _, _ = exec.Exec(context.Background(), "cmd2")
	if string(stdout) != "second" {
		t.Errorf("expected 'second', got %q", string(stdout))
	}

	// Third call
	stdout, _, _ = exec.Exec(context.Background(), "cmd3")
	if string(stdout) != "third" {
		t.Errorf("expected 'third', got %q", string(stdout))
	}

	// Fourth call should use default
	stdout, _, _ = exec.Exec(context.Background(), "cmd4")
	if len(stdout) != 0 {
		t.Errorf("expected empty stdout, got %q", string(stdout))
	}
}

func TestSSHExecutor_ContextCancellation(t *testing.T) {
	exec := NewSSHExecutor()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := exec.Exec(ctx, "echo test")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSSHExecutor_ExecInteractive(t *testing.T) {
	exec := NewSSHExecutor()
	stdin := bytes.NewReader([]byte("input data"))

	err := exec.ExecInteractive(context.Background(), "cat", stdin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	call, ok := exec.LastCall()
	if !ok {
		t.Fatal("expected to have a last call")
	}
	if call.Cmd != "cat" {
		t.Errorf("expected cmd 'cat', got %q", call.Cmd)
	}
	if string(call.Stdin) != "input data" {
		t.Errorf("expected stdin 'input data', got %q", string(call.Stdin))
	}
}

func TestSSHExecutor_ExecInteractiveError(t *testing.T) {
	exec := NewSSHExecutor()
	expectedErr := errors.New("interactive error")
	exec.ExecInteractiveError = expectedErr

	err := exec.ExecInteractive(context.Background(), "cmd", nil)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestSSHExecutor_StartSession(t *testing.T) {
	exec := NewSSHExecutor()
	exec.SetResponse("session", []byte("session output"), nil, nil)

	session, err := exec.StartSession()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stdout, _, err := session.Exec(context.Background(), "session cmd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "session output" {
		t.Errorf("expected 'session output', got %q", string(stdout))
	}

	if err := session.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}

	// After close, should error
	_, _, err = session.Exec(context.Background(), "another cmd")
	if err == nil {
		t.Error("expected error after session close")
	}
}

func TestSSHExecutor_Close(t *testing.T) {
	exec := NewSSHExecutor()
	if exec.Closed {
		t.Error("executor should not be closed initially")
	}

	if err := exec.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exec.Closed {
		t.Error("executor should be closed after Close()")
	}
}

func TestSSHExecutor_Reset(t *testing.T) {
	exec := NewSSHExecutor()
	exec.SetResponse("test", []byte("output"), nil, nil)
	exec.QueueResponse([]byte("queued"), nil, nil)
	if _, _, err := exec.Exec(context.Background(), "test"); err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	exec.Close()

	exec.Reset()

	if len(exec.Calls) != 0 {
		t.Error("calls should be empty after reset")
	}
	if len(exec.Responses) != 0 {
		t.Error("responses should be empty after reset")
	}
	if len(exec.ResponseQueue) != 0 {
		t.Error("response queue should be empty after reset")
	}
	if exec.Closed {
		t.Error("closed should be false after reset")
	}
}

func TestSSHExecutor_Concurrent(t *testing.T) {
	exec := NewSSHExecutor()
	exec.DefaultResponse = SSHExecResponse{Stdout: []byte("ok")}

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if _, _, err := exec.Exec(ctx, "concurrent cmd"); err != nil {
				t.Errorf("unexpected exec error: %v", err)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if exec.CallCount() != 10 {
		t.Errorf("expected 10 calls, got %d", exec.CallCount())
	}
}
