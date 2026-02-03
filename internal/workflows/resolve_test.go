package workflows

import (
	"path/filepath"
	"testing"
)

func TestResolveStepPrompt(t *testing.T) {
	root := filepath.Join("", "repo")
	wf := &Workflow{Source: filepath.Join(root, ".forge", "workflows", "basic.toml")}

	inline := ResolveStepPrompt(wf, WorkflowStep{Prompt: "hello"})
	if inline.Source != "inline" || inline.Inline != "hello" {
		t.Fatalf("expected inline prompt, got %#v", inline)
	}

	path := ResolveStepPrompt(wf, WorkflowStep{PromptPath: "prompts/plan.md"})
	wantPath := filepath.Join(root, "prompts", "plan.md")
	if path.Path != wantPath || path.Source != "path" {
		t.Fatalf("expected path %q, got %#v", wantPath, path)
	}

	name := ResolveStepPrompt(wf, WorkflowStep{PromptName: "plan"})
	wantName := filepath.Join(root, ".forge", "prompts", "plan.md")
	if name.Path != wantName || name.Source != "name" {
		t.Fatalf("expected name path %q, got %#v", wantName, name)
	}
}
