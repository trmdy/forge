package forged

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	forgedv1 "github.com/tOgg1/forge/gen/forged/v1"
	"github.com/tOgg1/forge/internal/config"
	"google.golang.org/grpc"
)

func skipIfNoNetwork(t *testing.T) {
	t.Helper()
	if os.Getenv("FORGE_TEST_SKIP_NETWORK") != "" {
		t.Skip("skipping network test: FORGE_TEST_SKIP_NETWORK is set")
	}
}

func TestDialDirect(t *testing.T) {
	skipIfNoNetwork(t)

	// Start a test daemon
	cfg := config.DefaultConfig()
	root := t.TempDir()
	cfg.Global.DataDir = filepath.Join(root, "data")
	cfg.Global.ConfigDir = filepath.Join(root, "config")
	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("failed to create config dirs: %v", err)
	}
	daemon, err := New(cfg, zerolog.Nop(), Options{Port: 50100})
	if err != nil {
		t.Fatalf("failed to create daemon: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start daemon in background
	daemonErr := make(chan error, 1)
	go func() {
		daemonErr <- daemon.Run(ctx)
	}()

	// Give daemon time to start
	time.Sleep(100 * time.Millisecond)

	// Connect directly
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dialCancel()

	client, err := Dial(dialCtx, "127.0.0.1:50100", WithLogger(zerolog.Nop()))
	if err != nil {
		cancel() // Stop daemon
		t.Fatalf("failed to dial: %v", err)
	}
	defer client.Close()

	// Test ping
	pingResp, err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if pingResp.Timestamp == nil {
		t.Error("expected timestamp in ping response")
	}

	// Test GetStatus
	statusResp, err := client.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	if statusResp.Status == nil {
		t.Error("expected status in response")
	}

	// Verify not tunneled
	if client.IsTunneled() {
		t.Error("expected direct connection to not be tunneled")
	}

	// Cleanup
	cancel()
	select {
	case err := <-daemonErr:
		if err != nil && err != context.Canceled {
			t.Errorf("daemon error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("daemon did not shut down in time")
	}
}

func TestClientClose(t *testing.T) {
	skipIfNoNetwork(t)

	// Create a mock server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	server := grpc.NewServer()
	forgedv1.RegisterForgedServiceServer(server, NewServer(zerolog.Nop()))

	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Stop()

	// Connect
	client, err := Dial(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	// Close should not error
	if err := client.Close(); err != nil {
		t.Errorf("close error: %v", err)
	}

	// Double close should be safe
	if err := client.Close(); err != nil {
		t.Errorf("second close error: %v", err)
	}
}

func TestClientMethods(t *testing.T) {
	skipIfNoNetwork(t)

	// Create a mock server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	server := grpc.NewServer()
	forgedv1.RegisterForgedServiceServer(server, NewServer(zerolog.Nop(), WithVersion("test-1.0")))

	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Stop()

	// Connect
	client, err := Dial(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer client.Close()

	// Test Ping
	pingResp, err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("ping error: %v", err)
	}
	if pingResp.Version != "test-1.0" {
		t.Errorf("version = %q, want %q", pingResp.Version, "test-1.0")
	}

	// Test GetStatus
	statusResp, err := client.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("get status error: %v", err)
	}
	if statusResp.Status == nil {
		t.Error("expected status")
	}

	// Test ListAgents (empty)
	listResp, err := client.ListAgents(context.Background(), &forgedv1.ListAgentsRequest{})
	if err != nil {
		t.Fatalf("list agents error: %v", err)
	}
	if len(listResp.Agents) != 0 {
		t.Errorf("agents count = %d, want 0", len(listResp.Agents))
	}

	// Test GetAgent (not found)
	_, err = client.GetAgent(context.Background(), &forgedv1.GetAgentRequest{AgentId: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}

	// Test Service accessor
	if client.Service() == nil {
		t.Error("expected non-nil service")
	}

	// Test LocalAddr
	if client.LocalAddr() == "" {
		t.Error("expected non-empty local addr")
	}
}

func TestDialSSHValidation(t *testing.T) {
	ctx := context.Background()

	// Empty host should fail
	_, err := DialSSH(ctx, "", 22, DefaultPort)
	if err == nil {
		t.Error("expected error for empty SSH host")
	}
}
