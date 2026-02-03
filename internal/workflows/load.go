package workflows

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// LoadWorkflow reads a workflow from disk without validation.
func LoadWorkflow(path string) (*Workflow, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("workflow path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflow %s: %w", path, err)
	}

	wf, err := parseWorkflow(data)
	if err != nil {
		return nil, wrapParseError(path, err)
	}
	wf.Source = path
	return wf, nil
}

// LoadWorkflowsFromDir loads all workflows from a directory.
func LoadWorkflowsFromDir(dir string) ([]*Workflow, error) {
	if strings.TrimSpace(dir) == "" {
		return []*Workflow{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Workflow{}, nil
		}
		return nil, fmt.Errorf("read workflows dir %s: %w", dir, err)
	}

	workflows := make([]*Workflow, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".toml" {
			continue
		}
		path := filepath.Join(dir, name)
		wf, err := LoadWorkflow(path)
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, wf)
	}

	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].Name < workflows[j].Name
	})

	return workflows, nil
}

// WorkflowSearchPaths returns workflow search directories in precedence order.
func WorkflowSearchPaths(projectDir string) []string {
	if strings.TrimSpace(projectDir) == "" {
		return nil
	}
	return []string{filepath.Join(projectDir, ".forge", "workflows")}
}

// LoadWorkflowsFromSearchPaths loads workflows from search paths with first-hit precedence.
func LoadWorkflowsFromSearchPaths(projectDir string) ([]*Workflow, error) {
	paths := WorkflowSearchPaths(projectDir)
	seen := make(map[string]*Workflow)
	order := make([]string, 0)

	for _, path := range paths {
		items, err := LoadWorkflowsFromDir(path)
		if err != nil {
			return nil, err
		}
		for _, wf := range items {
			if _, exists := seen[wf.Name]; exists {
				continue
			}
			seen[wf.Name] = wf
			order = append(order, wf.Name)
		}
	}

	resolved := make([]*Workflow, 0, len(order))
	for _, name := range order {
		resolved = append(resolved, seen[name])
	}
	return resolved, nil
}

func parseWorkflow(data []byte) (*Workflow, error) {
	var wf Workflow
	if err := toml.Unmarshal(data, &wf); err != nil {
		return nil, err
	}
	return &wf, nil
}

func wrapParseError(path string, err error) error {
	list := &ErrorList{}

	var strictErr *toml.StrictMissingError
	if errors.As(err, &strictErr) {
		for _, decodeErr := range strictErr.Errors {
			line, column := decodeErr.Position()
			list.Add(WorkflowError{
				Code:    ErrCodeParse,
				Message: decodeErr.Error(),
				Path:    path,
				Line:    line,
				Column:  column,
				Field:   strings.Join(decodeErr.Key(), "."),
			})
		}
		return list
	}

	var decodeErr *toml.DecodeError
	if errors.As(err, &decodeErr) {
		line, column := decodeErr.Position()
		list.Add(WorkflowError{
			Code:    ErrCodeParse,
			Message: decodeErr.Error(),
			Path:    path,
			Line:    line,
			Column:  column,
		})
		return list
	}

	list.Add(WorkflowError{
		Code:    ErrCodeParse,
		Message: err.Error(),
		Path:    path,
	})
	return list
}
