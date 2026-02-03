package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/ssh"
	"github.com/tOgg1/forge/internal/testutil/mocks"
)

// mockSSHExecutorFunc returns a function that creates mock SSH executors.
func mockSSHExecutorFunc(executor ssh.Executor) func(*models.Node) (ssh.Executor, error) {
	return func(node *models.Node) (ssh.Executor, error) {
		return executor, nil
	}
}

// mockSSHExecutorErrFunc returns a function that fails to create SSH executors.
func mockSSHExecutorErrFunc(err error) func(*models.Node) (ssh.Executor, error) {
	return func(node *models.Node) (ssh.Executor, error) {
		return nil, err
	}
}

var _ = mockSSHExecutorErrFunc

func TestNewClient_SSHMode(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	// Mock successful SSH test command
	mockExec.SetResponse("echo", []byte("ok\n"), nil, nil)

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	client, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeSSH),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
		WithClientLogger(zerolog.Nop()),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if !client.IsSSHMode() {
		t.Error("expected SSH mode")
	}
	if client.IsDaemonMode() {
		t.Error("expected not daemon mode")
	}
	if client.Mode() != "ssh" {
		t.Errorf("Mode() = %q, want %q", client.Mode(), "ssh")
	}
}

func TestNewClient_NilNode(t *testing.T) {
	_, err := NewClient(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil node")
	}
}

func TestNewClient_SSHConnectionFailed(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	// Mock failed SSH test command
	mockExec.SetResponse("echo", nil, nil, errors.New("connection refused"))

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	_, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeSSH),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
	)
	if err == nil {
		t.Fatal("expected error for failed SSH connection")
	}
}

func TestNewClient_AutoFallbackToSSH(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	mockExec.SetResponse("echo", []byte("ok\n"), nil, nil)

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	// In auto mode with prefer daemon, it should fall back to SSH when daemon fails
	client, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeAuto),
		WithPreferDaemon(true),
		WithDaemonTimeout(100*time.Millisecond),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
		WithClientLogger(zerolog.Nop()),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	// Should have fallen back to SSH since daemon isn't running
	if !client.IsSSHMode() {
		t.Error("expected SSH mode after fallback")
	}
}

func TestClient_Ping_SSHMode(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	mockExec.SetResponse("echo", []byte("ok\n"), nil, nil)

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	client, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeSSH),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	// Ping should use SSH
	if err := client.Ping(context.Background()); err != nil {
		t.Errorf("Ping() error = %v", err)
	}

	// Should have called echo command
	if mockExec.CallCount() < 1 {
		t.Error("expected at least one SSH call for ping")
	}
}

func TestClient_CapturePane_SSHMode(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	// Mock tmux capture-pane command
	mockExec.SetResponse("echo", []byte("ok\n"), nil, nil)
	mockExec.SetResponse("tmux", []byte("pane content here\n"), nil, nil)

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	client, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeSSH),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	content, hash, err := client.CapturePane(context.Background(), "agent-1", "%0", false)
	if err != nil {
		t.Errorf("CapturePane() error = %v", err)
	}

	if content == "" {
		t.Error("expected non-empty content")
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}
}

func TestClient_SpawnAgentSSH(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	// Mock initial SSH test
	mockExec.SetResponse("echo", []byte("ok\n"), nil, nil)
	// Mock tmux commands - order matters for queue
	mockExec.QueueResponse([]byte("ok\n"), nil, nil) // echo ok
	mockExec.QueueResponse([]byte("1\n"), nil, nil)  // has-session
	mockExec.QueueResponse([]byte("%1\n"), nil, nil) // split-window returns pane ID
	mockExec.QueueResponse(nil, nil, nil)            // send-keys for command

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	client, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeSSH),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	req := &SpawnAgentRequest{
		AgentID:     "agent-1",
		WorkspaceID: "ws-1",
		Command:     "opencode",
		Args:        []string{"--headless"},
		WorkingDir:  "/tmp",
		SessionName: "test-session",
	}

	resp, err := client.SpawnAgent(context.Background(), req)
	if err != nil {
		t.Errorf("SpawnAgent() error = %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", resp.AgentID, "agent-1")
	}
}

func TestClient_KillAgent_SSHMode(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	mockExec.SetResponse("echo", []byte("ok\n"), nil, nil)
	mockExec.SetResponse("tmux", nil, nil, nil)

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	client, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeSSH),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	// Force kill should just kill the pane
	err = client.KillAgent(context.Background(), "agent-1", "%0", true)
	if err != nil {
		t.Errorf("KillAgent() error = %v", err)
	}
}

func TestClient_SendInput_SSHMode(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	mockExec.SetResponse("echo", []byte("ok\n"), nil, nil)
	mockExec.SetResponse("tmux", nil, nil, nil)

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	client, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeSSH),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	// Send text input
	err = client.SendInput(context.Background(), "agent-1", "%0", "hello", true, nil)
	if err != nil {
		t.Errorf("SendInput() error = %v", err)
	}

	// Send special keys
	err = client.SendInput(context.Background(), "agent-1", "%0", "", false, []string{"C-c"})
	if err != nil {
		t.Errorf("SendInput() with keys error = %v", err)
	}
}

func TestClient_ListSessions_SSHMode(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	mockExec.SetResponse("echo", []byte("ok\n"), nil, nil)
	// Mock tmux list-sessions output in expected format: session_name|window_count
	mockExec.SetResponse("tmux list-sessions", []byte("session1|2\nsession2|1\n"), nil, nil)

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	client, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeSSH),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	sessions, err := client.ListSessions(context.Background())
	if err != nil {
		t.Errorf("ListSessions() error = %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestClient_Close(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	mockExec.SetResponse("echo", []byte("ok\n"), nil, nil)

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	client, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeSSH),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// First close should succeed
	if err := client.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Second close should be idempotent
	if err := client.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}

	if !mockExec.Closed {
		t.Error("expected executor to be closed")
	}
}

func TestClient_GetDaemonStatus_SSHMode(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	mockExec.SetResponse("echo", []byte("ok\n"), nil, nil)

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	client, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeSSH),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	// Should return ErrDaemonNotFound in SSH mode
	_, err = client.GetDaemonStatus(context.Background())
	if !errors.Is(err, ErrDaemonNotFound) {
		t.Errorf("GetDaemonStatus() error = %v, want ErrDaemonNotFound", err)
	}
}

func TestClient_Node(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	mockExec.SetResponse("echo", []byte("ok\n"), nil, nil)

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	client, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeSSH),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if got := client.Node(); got != node {
		t.Error("Node() should return the same node")
	}
}

func TestClient_DaemonClient_SSHMode(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	mockExec.SetResponse("echo", []byte("ok\n"), nil, nil)

	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	client, err := NewClient(
		context.Background(),
		node,
		WithClientMode(ClientModeSSH),
		WithSSHExecutorFunc(mockSSHExecutorFunc(mockExec)),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if dc := client.DaemonClient(); dc != nil {
		t.Error("DaemonClient() should return nil in SSH mode")
	}
}

func TestClientModes(t *testing.T) {
	tests := []struct {
		mode ClientMode
		want string
	}{
		{ClientModeAuto, "auto"},
		{ClientModeDaemon, "daemon"},
		{ClientModeSSH, "ssh"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.want {
			t.Errorf("ClientMode %q != %q", tt.mode, tt.want)
		}
	}
}

func TestClientErrors(t *testing.T) {
	// Verify error variables are properly defined
	if ErrNoConnection == nil {
		t.Error("ErrNoConnection should not be nil")
	}
	if ErrDaemonNotFound == nil {
		t.Error("ErrDaemonNotFound should not be nil")
	}
	if ErrAgentNotFound == nil {
		t.Error("ErrAgentNotFound should not be nil")
	}
	if ErrOperationFailed == nil {
		t.Error("ErrOperationFailed should not be nil")
	}
}

func TestClient_ListPanes_NoTmuxClient(t *testing.T) {
	// This tests the case where tmuxClient is nil
	node := &models.Node{
		Name:    "test-node",
		IsLocal: true,
	}

	// Create a client manually without tmux client
	client := &Client{
		node:       node,
		logger:     zerolog.Nop(),
		tmuxClient: nil,
	}

	_, err := client.ListPanes(context.Background(), "test-session")
	if err == nil {
		t.Error("expected error when tmuxClient is nil")
	}
}

func TestSSHTmuxExecutor(t *testing.T) {
	mockExec := mocks.NewSSHExecutor()
	mockExec.SetResponse("test", []byte("output"), []byte("stderr"), nil)

	adapter := &sshTmuxExecutor{executor: mockExec}

	stdout, stderr, err := adapter.Exec(context.Background(), "test command")
	if err != nil {
		t.Errorf("Exec() error = %v", err)
	}
	if string(stdout) != "output" {
		t.Errorf("stdout = %q, want %q", stdout, "output")
	}
	if string(stderr) != "stderr" {
		t.Errorf("stderr = %q, want %q", stderr, "stderr")
	}
}
