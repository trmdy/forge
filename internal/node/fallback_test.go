package node

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/ssh"
	"github.com/rs/zerolog"
)

// mockExecutor is a mock SSH executor for testing.
type mockExecutor struct {
	execFunc  func(ctx context.Context, cmd string) ([]byte, []byte, error)
	closeFunc func() error
	closed    bool
}

func (m *mockExecutor) Exec(ctx context.Context, cmd string) ([]byte, []byte, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, cmd)
	}
	return []byte("ok\n"), nil, nil
}

func (m *mockExecutor) ExecInteractive(ctx context.Context, cmd string, stdin io.Reader) error {
	return nil
}

func (m *mockExecutor) StartSession() (ssh.Session, error) {
	return nil, errors.New("not implemented")
}

func (m *mockExecutor) Close() error {
	m.closed = true
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestNewNodeExecutor(t *testing.T) {
	tests := []struct {
		name    string
		node    *models.Node
		wantErr bool
	}{
		{
			name:    "nil node",
			node:    nil,
			wantErr: true,
		},
		{
			name: "local node",
			node: &models.Node{
				ID:      "local-1",
				Name:    "local",
				IsLocal: true,
			},
			wantErr: false,
		},
		{
			name: "remote node with ssh",
			node: &models.Node{
				ID:        "remote-1",
				Name:      "remote",
				IsLocal:   false,
				SSHTarget: "user@host:22",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var executor *mockExecutor
			if tt.node != nil && !tt.node.IsLocal {
				executor = &mockExecutor{}
			}

			exec, err := NewNodeExecutor(context.Background(), tt.node, executor,
				WithFallbackPolicy(FallbackPolicySSHOnly),
				WithNodeLogger(zerolog.Nop()),
			)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewNodeExecutor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && exec != nil {
				defer exec.Close()

				if tt.node.IsLocal {
					if exec.Mode() != ModeLocal {
						t.Errorf("Mode() = %v, want %v", exec.Mode(), ModeLocal)
					}
				} else {
					if exec.Mode() != ModeSSH {
						t.Errorf("Mode() = %v, want %v", exec.Mode(), ModeSSH)
					}
				}
			}
		})
	}
}

func TestNodeExecutorExec(t *testing.T) {
	node := &models.Node{
		ID:        "test-1",
		Name:      "test",
		IsLocal:   false,
		SSHTarget: "user@host:22",
	}

	executor := &mockExecutor{
		execFunc: func(ctx context.Context, cmd string) ([]byte, []byte, error) {
			return []byte("hello world\n"), nil, nil
		},
	}

	exec, err := NewNodeExecutor(context.Background(), node, executor,
		WithFallbackPolicy(FallbackPolicySSHOnly),
	)
	if err != nil {
		t.Fatalf("NewNodeExecutor() error = %v", err)
	}
	defer exec.Close()

	stdout, stderr, err := exec.Exec(context.Background(), "echo hello world")
	if err != nil {
		t.Errorf("Exec() error = %v", err)
	}
	if string(stdout) != "hello world\n" {
		t.Errorf("Exec() stdout = %q, want %q", string(stdout), "hello world\n")
	}
	if len(stderr) != 0 {
		t.Errorf("Exec() stderr = %q, want empty", string(stderr))
	}
}

func TestNodeExecutorPing(t *testing.T) {
	tests := []struct {
		name     string
		execFunc func(ctx context.Context, cmd string) ([]byte, []byte, error)
		wantErr  bool
	}{
		{
			name: "successful ping",
			execFunc: func(ctx context.Context, cmd string) ([]byte, []byte, error) {
				return []byte("ok\n"), nil, nil
			},
			wantErr: false,
		},
		{
			name: "failed ping",
			execFunc: func(ctx context.Context, cmd string) ([]byte, []byte, error) {
				return nil, nil, errors.New("connection refused")
			},
			wantErr: true,
		},
		{
			name: "empty response",
			execFunc: func(ctx context.Context, cmd string) ([]byte, []byte, error) {
				return []byte(""), nil, nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &models.Node{
				ID:        "test-1",
				Name:      "test",
				IsLocal:   false,
				SSHTarget: "user@host:22",
			}

			executor := &mockExecutor{execFunc: tt.execFunc}

			exec, err := NewNodeExecutor(context.Background(), node, executor,
				WithFallbackPolicy(FallbackPolicySSHOnly),
			)
			if err != nil {
				t.Fatalf("NewNodeExecutor() error = %v", err)
			}
			defer exec.Close()

			err = exec.Ping(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Ping() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeExecutorClose(t *testing.T) {
	node := &models.Node{
		ID:        "test-1",
		Name:      "test",
		IsLocal:   false,
		SSHTarget: "user@host:22",
	}

	executor := &mockExecutor{}

	exec, err := NewNodeExecutor(context.Background(), node, executor,
		WithFallbackPolicy(FallbackPolicySSHOnly),
	)
	if err != nil {
		t.Fatalf("NewNodeExecutor() error = %v", err)
	}

	// Close should succeed
	if err := exec.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Double close should be safe
	if err := exec.Close(); err != nil {
		t.Errorf("Close() second call error = %v", err)
	}

	// Operations after close should fail
	_, _, err = exec.Exec(context.Background(), "echo test")
	if !errors.Is(err, ErrNodeClosed) {
		t.Errorf("Exec() after close error = %v, want %v", err, ErrNodeClosed)
	}

	err = exec.Ping(context.Background())
	if !errors.Is(err, ErrNodeClosed) {
		t.Errorf("Ping() after close error = %v, want %v", err, ErrNodeClosed)
	}
}

func TestNodeExecutorSwarmdOperationsWithoutSwarmd(t *testing.T) {
	node := &models.Node{
		ID:        "test-1",
		Name:      "test",
		IsLocal:   false,
		SSHTarget: "user@host:22",
	}

	executor := &mockExecutor{}

	exec, err := NewNodeExecutor(context.Background(), node, executor,
		WithFallbackPolicy(FallbackPolicySSHOnly),
	)
	if err != nil {
		t.Fatalf("NewNodeExecutor() error = %v", err)
	}
	defer exec.Close()

	// swarmd operations should fail when in SSH mode
	if exec.IsSwarmdAvailable() {
		t.Error("IsSwarmdAvailable() should be false in SSH-only mode")
	}

	_, err = exec.SpawnAgent(context.Background(), nil)
	if !errors.Is(err, ErrSwarmdUnavailable) {
		t.Errorf("SpawnAgent() error = %v, want %v", err, ErrSwarmdUnavailable)
	}

	_, err = exec.KillAgent(context.Background(), nil)
	if !errors.Is(err, ErrSwarmdUnavailable) {
		t.Errorf("KillAgent() error = %v, want %v", err, ErrSwarmdUnavailable)
	}

	_, err = exec.CapturePane(context.Background(), nil)
	if !errors.Is(err, ErrSwarmdUnavailable) {
		t.Errorf("CapturePane() error = %v, want %v", err, ErrSwarmdUnavailable)
	}

	_, err = exec.ListAgents(context.Background(), nil)
	if !errors.Is(err, ErrSwarmdUnavailable) {
		t.Errorf("ListAgents() error = %v, want %v", err, ErrSwarmdUnavailable)
	}
}

func TestFallbackPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy FallbackPolicy
	}{
		{name: "auto", policy: FallbackPolicyAuto},
		{name: "swarmd_only", policy: FallbackPolicySwarmdOnly},
		{name: "ssh_only", policy: FallbackPolicySSHOnly},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &models.Node{
				ID:      "local-1",
				Name:    "local",
				IsLocal: true,
			}

			// For swarmd_only, it will fail if swarmd is not available
			if tt.policy == FallbackPolicySwarmdOnly {
				_, err := NewNodeExecutor(context.Background(), node, nil,
					WithFallbackPolicy(tt.policy),
					WithPingTimeout(100*time.Millisecond),
				)
				if err == nil {
					t.Error("Expected error for swarmd_only policy when swarmd unavailable")
				}
				return
			}

			exec, err := NewNodeExecutor(context.Background(), node, nil,
				WithFallbackPolicy(tt.policy),
			)
			if err != nil {
				t.Fatalf("NewNodeExecutor() error = %v", err)
			}
			defer exec.Close()
		})
	}
}

func TestExecutionModeConstants(t *testing.T) {
	if ModeSwarmd != "swarmd" {
		t.Errorf("ModeSwarmd = %q, want %q", ModeSwarmd, "swarmd")
	}
	if ModeSSH != "ssh" {
		t.Errorf("ModeSSH = %q, want %q", ModeSSH, "ssh")
	}
	if ModeLocal != "local" {
		t.Errorf("ModeLocal = %q, want %q", ModeLocal, "local")
	}
}

func TestFallbackPolicyConstants(t *testing.T) {
	if FallbackPolicyAuto != "auto" {
		t.Errorf("FallbackPolicyAuto = %q, want %q", FallbackPolicyAuto, "auto")
	}
	if FallbackPolicySwarmdOnly != "swarmd_only" {
		t.Errorf("FallbackPolicySwarmdOnly = %q, want %q", FallbackPolicySwarmdOnly, "swarmd_only")
	}
	if FallbackPolicySSHOnly != "ssh_only" {
		t.Errorf("FallbackPolicySSHOnly = %q, want %q", FallbackPolicySSHOnly, "ssh_only")
	}
}

func TestWithOptions(t *testing.T) {
	node := &models.Node{
		ID:      "local-1",
		Name:    "local",
		IsLocal: true,
	}

	logger := zerolog.Nop()

	exec, err := NewNodeExecutor(context.Background(), node, nil,
		WithFallbackPolicy(FallbackPolicySSHOnly),
		WithSwarmdPort(9999),
		WithPingTimeout(10*time.Second),
		WithNodeLogger(logger),
	)
	if err != nil {
		t.Fatalf("NewNodeExecutor() error = %v", err)
	}
	defer exec.Close()

	// Verify options were applied
	if exec.policy != FallbackPolicySSHOnly {
		t.Errorf("policy = %v, want %v", exec.policy, FallbackPolicySSHOnly)
	}
	if exec.swarmdPort != 9999 {
		t.Errorf("swarmdPort = %v, want %v", exec.swarmdPort, 9999)
	}
	if exec.pingTimeout != 10*time.Second {
		t.Errorf("pingTimeout = %v, want %v", exec.pingTimeout, 10*time.Second)
	}
}
