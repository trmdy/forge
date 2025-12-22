package ssh

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplySSHConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configPath := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatalf("create ssh dir: %v", err)
	}

	config := `
Host myalias
  HostName real.example.com
  User ubuntu
  Port 2222
  ProxyJump jumpbox
  IdentityFile ~/.ssh/id_test

Host *
  User default
  Port 22
  IdentityFile ~/.ssh/id_default
`

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	opts := ConnectionOptions{Host: "myalias"}
	resolved, err := ApplySSHConfig(opts, configPath)
	if err != nil {
		t.Fatalf("ApplySSHConfig: %v", err)
	}

	if resolved.Host != "real.example.com" {
		t.Fatalf("Host = %q, want %q", resolved.Host, "real.example.com")
	}
	if resolved.User != "ubuntu" {
		t.Fatalf("User = %q, want %q", resolved.User, "ubuntu")
	}
	if resolved.Port != 2222 {
		t.Fatalf("Port = %d, want %d", resolved.Port, 2222)
	}
	if resolved.ProxyJump != "jumpbox" {
		t.Fatalf("ProxyJump = %q, want %q", resolved.ProxyJump, "jumpbox")
	}
	wantKey := filepath.Join(home, ".ssh", "id_test")
	if resolved.KeyPath != wantKey {
		t.Fatalf("KeyPath = %q, want %q", resolved.KeyPath, wantKey)
	}
}

func TestApplySSHConfig_NoConfig(t *testing.T) {
	opts := ConnectionOptions{Host: "example.com", User: "root", Port: 2200}
	resolved, err := ApplySSHConfig(opts, filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Host != opts.Host || resolved.User != opts.User || resolved.Port != opts.Port {
		t.Fatalf("expected options to remain unchanged")
	}
}
