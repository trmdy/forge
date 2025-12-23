package adapters

import "testing"

func TestOpenCodeAdapter_SpawnCommandDefaults(t *testing.T) {
	adapter := OpenCodeAdapter()

	cmd, args := adapter.SpawnCommand(SpawnOptions{})
	if cmd != "opencode" {
		t.Fatalf("expected command opencode, got %q", cmd)
	}

	for i := 0; i < len(args); i++ {
		if args[i] == "--hostname" {
			if i+1 >= len(args) {
				t.Fatalf("expected value after --hostname, got none")
			}
			if args[i+1] != "127.0.0.1" {
				t.Fatalf("expected hostname 127.0.0.1, got %q", args[i+1])
			}
			return
		}
	}

	t.Fatal("expected --hostname flag in opencode spawn command")
}
