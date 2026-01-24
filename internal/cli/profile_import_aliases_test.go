package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDetectDefaultHarnessAliases(t *testing.T) {
	tmpDir := t.TempDir()

	originalPath := os.Getenv("PATH")
	t.Cleanup(func() {
		_ = os.Setenv("PATH", originalPath)
	})

	if err := os.Setenv("PATH", tmpDir); err != nil {
		t.Fatalf("set PATH failed: %v", err)
	}

	commands := []string{"codex", "claude-code", "opencode", "pi", "droid"}
	for _, cmd := range commands {
		if err := writeExecutable(filepath.Join(tmpDir, cmd)); err != nil {
			t.Fatalf("write executable %s failed: %v", cmd, err)
		}
	}

	got := make(map[string]string)
	for _, entry := range detectDefaultHarnessAliases(map[string]string{}) {
		got[entry.Name] = entry.Output
	}

	want := map[string]string{
		"codex":    "codex",
		"claude":   "claude-code",
		"opencode": "opencode",
		"pi":       "pi",
		"droid":    "droid",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries mismatch: got %#v want %#v", got, want)
	}

	got = make(map[string]string)
	for _, entry := range detectDefaultHarnessAliases(map[string]string{"codex": "alias codex=codex", "pi": "alias pi=pi"}) {
		got[entry.Name] = entry.Output
	}

	if _, ok := got["codex"]; ok {
		t.Fatalf("codex should be skipped when already present")
	}
	if _, ok := got["pi"]; ok {
		t.Fatalf("pi should be skipped when already present")
	}
}

func writeExecutable(path string) error {
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0755); err != nil {
		return err
	}
	return nil
}
