package loop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tOgg1/forge/internal/models"
)

type promptSpec struct {
	Path     string
	Content  string
	Source   string
	Override bool
	FromFile bool
}

func resolveBasePrompt(loop *models.Loop) (promptSpec, error) {
	if loop == nil {
		return promptSpec{}, errors.New("loop is nil")
	}

	if strings.TrimSpace(loop.BasePromptMsg) != "" {
		return promptSpec{Content: loop.BasePromptMsg, Source: "base", Override: false, FromFile: false}, nil
	}

	if strings.TrimSpace(loop.BasePromptPath) != "" {
		path := resolveRepoPath(loop.RepoPath, loop.BasePromptPath)
		content, err := os.ReadFile(path)
		if err != nil {
			return promptSpec{}, err
		}
		return promptSpec{Path: path, Content: string(content), Source: "base", Override: false, FromFile: true}, nil
	}

	promptPath := filepath.Join(loop.RepoPath, "PROMPT.md")
	if _, err := os.Stat(promptPath); err == nil {
		content, err := os.ReadFile(promptPath)
		if err != nil {
			return promptSpec{}, err
		}
		return promptSpec{Path: promptPath, Content: string(content), Source: "base", Override: false, FromFile: true}, nil
	}

	fallback := filepath.Join(loop.RepoPath, ".forge", "prompts", "default.md")
	content, err := os.ReadFile(fallback)
	if err != nil {
		return promptSpec{}, err
	}
	return promptSpec{Path: fallback, Content: string(content), Source: "base", Override: false, FromFile: true}, nil
}

func resolveOverridePrompt(repoPath string, payload models.NextPromptOverridePayload) (promptSpec, error) {
	if strings.TrimSpace(payload.Prompt) == "" {
		return promptSpec{}, errors.New("override prompt is empty")
	}

	if payload.IsPath {
		path := resolveRepoPath(repoPath, payload.Prompt)
		content, err := os.ReadFile(path)
		if err != nil {
			return promptSpec{}, err
		}
		return promptSpec{Path: path, Content: string(content), Source: "override", Override: true, FromFile: true}, nil
	}

	return promptSpec{Content: payload.Prompt, Source: "override", Override: true, FromFile: false}, nil
}

func resolveRepoPath(repoRoot, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(repoRoot, path)
}

func appendOperatorMessages(base string, messages []messageEntry) string {
	if len(messages) == 0 {
		return base
	}

	builder := strings.Builder{}
	builder.WriteString(strings.TrimRight(base, "\n"))
	for _, entry := range messages {
		builder.WriteString("\n\n## Operator Message (")
		builder.WriteString(entry.Timestamp.UTC().Format(time.RFC3339))
		builder.WriteString(")\n\n")
		builder.WriteString(strings.TrimSpace(entry.Text))
	}

	return builder.String()
}
