package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCreatesScaffold(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	initForce = false
	initPromptsFrom = ""
	initNoCreatePrompt = false

	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	forgeDir := filepath.Join(tmpDir, ".forge")
	paths := []string{
		filepath.Join(forgeDir, "prompts"),
		filepath.Join(forgeDir, "templates"),
		filepath.Join(forgeDir, "sequences"),
		filepath.Join(forgeDir, "ledgers"),
		filepath.Join(forgeDir, "forge.yaml"),
		filepath.Join(tmpDir, "PROMPT.md"),
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
}

func TestInitNoCreatePrompt(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	initForce = false
	initPromptsFrom = ""
	initNoCreatePrompt = true

	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "PROMPT.md")); err == nil {
		t.Fatalf("PROMPT.md should not be created when --no-create-prompt is set")
	}
}

func TestInitPromptsFrom(t *testing.T) {
	tmpDir := t.TempDir()
	promptDir := filepath.Join(tmpDir, "seed-prompts")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	seedPrompt := filepath.Join(promptDir, "review.md")
	if err := os.WriteFile(seedPrompt, []byte("review"), 0644); err != nil {
		t.Fatalf("write prompt failed: %v", err)
	}

	originalWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	initForce = false
	initPromptsFrom = promptDir
	initNoCreatePrompt = true

	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	copied := filepath.Join(tmpDir, ".forge", "prompts", "review.md")
	if _, err := os.Stat(copied); err != nil {
		t.Fatalf("expected copied prompt %s: %v", copied, err)
	}
}
