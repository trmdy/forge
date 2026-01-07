package loop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tOgg1/forge/internal/models"
)

func TestResolveBasePromptPrecedence(t *testing.T) {
	repo := t.TempDir()
	forgeDir := filepath.Join(repo, ".forge", "prompts")
	if err := os.MkdirAll(forgeDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	defaultPrompt := filepath.Join(forgeDir, "default.md")
	if err := os.WriteFile(defaultPrompt, []byte("default"), 0o644); err != nil {
		t.Fatalf("write default prompt: %v", err)
	}

	promptFile := filepath.Join(repo, "PROMPT.md")
	if err := os.WriteFile(promptFile, []byte("prompt"), 0o644); err != nil {
		t.Fatalf("write PROMPT.md: %v", err)
	}

	loop := &models.Loop{RepoPath: repo, BasePromptMsg: "inline"}
	prompt, err := resolveBasePrompt(loop)
	if err != nil {
		t.Fatalf("resolve base prompt: %v", err)
	}
	if prompt.Content != "inline" || prompt.FromFile {
		t.Fatalf("expected inline prompt, got %q", prompt.Content)
	}

	custom := filepath.Join(repo, "custom.md")
	if err := os.WriteFile(custom, []byte("custom"), 0o644); err != nil {
		t.Fatalf("write custom prompt: %v", err)
	}
	loop = &models.Loop{RepoPath: repo, BasePromptPath: "custom.md"}
	prompt, err = resolveBasePrompt(loop)
	if err != nil {
		t.Fatalf("resolve base prompt: %v", err)
	}
	if prompt.Content != "custom" || prompt.Path != custom {
		t.Fatalf("expected custom prompt path, got %q", prompt.Path)
	}

	loop = &models.Loop{RepoPath: repo}
	prompt, err = resolveBasePrompt(loop)
	if err != nil {
		t.Fatalf("resolve base prompt: %v", err)
	}
	if prompt.Path != promptFile {
		t.Fatalf("expected PROMPT.md, got %q", prompt.Path)
	}

	if err := os.Remove(promptFile); err != nil {
		t.Fatalf("remove PROMPT.md: %v", err)
	}
	prompt, err = resolveBasePrompt(loop)
	if err != nil {
		t.Fatalf("resolve fallback prompt: %v", err)
	}
	if prompt.Path != defaultPrompt {
		t.Fatalf("expected default prompt path, got %q", prompt.Path)
	}
}
