package agentmail

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasAgentMailConfigEmptyPath(t *testing.T) {
	detected, err := HasAgentMailConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detected {
		t.Fatal("expected false for empty path")
	}
}

func TestHasAgentMailConfigDetectsToken(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "mcp.json")
	content := `{"mcpServers":{"agent-mail":{"command":"mcp_agent_mail"}}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	detected, err := HasAgentMailConfig(repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !detected {
		t.Fatal("expected detection when token is present")
	}
}

func TestHasAgentMailConfigIgnoresNonMatchingConfig(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "mcp.json")
	content := `{"mcpServers":{"other":{"command":"something"}}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	detected, err := HasAgentMailConfig(repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detected {
		t.Fatal("expected false when token is absent")
	}
}

func TestHasAgentMailConfigSkipsDirectories(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "mcp.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	detected, err := HasAgentMailConfig(repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detected {
		t.Fatal("expected false when candidate is a directory")
	}
}
