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

func TestInitCreatesGitignoreWithFmail(t *testing.T) {
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

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	if string(content) != ".fmail/\n" {
		t.Fatalf("expected .gitignore to contain '.fmail/', got %q", string(content))
	}
}

func TestInitAppendsFmailToExistingGitignore(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	// Create existing .gitignore
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("node_modules/\n"), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	initForce = false
	initPromptsFrom = ""
	initNoCreatePrompt = true

	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	expected := "node_modules/\n.fmail/\n"
	if string(content) != expected {
		t.Fatalf("expected .gitignore to be %q, got %q", expected, string(content))
	}
}

func TestInitDoesNotDuplicateFmailInGitignore(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	// Create existing .gitignore with .fmail/ already present
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	existing := "node_modules/\n.fmail/\n"
	if err := os.WriteFile(gitignorePath, []byte(existing), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	initForce = false
	initPromptsFrom = ""
	initNoCreatePrompt = true

	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	if string(content) != existing {
		t.Fatalf("expected .gitignore to remain %q, got %q", existing, string(content))
	}
}
