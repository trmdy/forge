package workflows

import (
	"path/filepath"
	"strings"
)

// PromptResolution describes how a step prompt was resolved.
type PromptResolution struct {
	Inline string
	Path   string
	Source string
}

// ResolveStepPrompt resolves prompt data for a step.
func ResolveStepPrompt(wf *Workflow, step WorkflowStep) PromptResolution {
	if step.Prompt != "" {
		return PromptResolution{Inline: step.Prompt, Source: "inline"}
	}

	root := RepoRootFromWorkflow(wf)

	if step.PromptPath != "" {
		path := step.PromptPath
		if root != "" && !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		return PromptResolution{Path: path, Source: "path"}
	}

	if step.PromptName != "" {
		name := step.PromptName
		if !strings.HasSuffix(name, ".md") {
			name += ".md"
		}
		path := name
		if root != "" {
			path = filepath.Join(root, ".forge", "prompts", name)
		}
		return PromptResolution{Path: path, Source: "name"}
	}

	return PromptResolution{}
}

// RepoRootFromWorkflow returns the repo root derived from workflow source path.
func RepoRootFromWorkflow(wf *Workflow) string {
	if wf == nil {
		return ""
	}
	return repoRootFromPath(wf.Source)
}

func repoRootFromPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}

	dir := filepath.Dir(path)
	for dir != "" {
		if filepath.Base(dir) == ".forge" {
			return filepath.Dir(dir)
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}

	return ""
}
