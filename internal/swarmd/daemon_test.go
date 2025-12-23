package swarmd

import (
	"context"
	"testing"

	"github.com/opencode-ai/swarm/internal/config"
	"github.com/rs/zerolog"
)

func TestNewDefaultsHostname(t *testing.T) {
	cfg := config.DefaultConfig()
	daemon, err := New(cfg, zerolog.Nop(), Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if got := daemon.bindAddr(); got != "127.0.0.1:0" {
		t.Fatalf("bindAddr() = %q, want %q", got, "127.0.0.1:0")
	}
}

func TestRunReturnsOnCanceledContext(t *testing.T) {
	cfg := config.DefaultConfig()
	daemon, err := New(cfg, zerolog.Nop(), Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := daemon.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}
