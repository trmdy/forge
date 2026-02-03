package workflows

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestValidateWorkflowValidFixture(t *testing.T) {
	path := filepath.Join("testdata", "valid-basic.toml")
	wf, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("load workflow: %v", err)
	}

	validated, err := ValidateWorkflow(wf)
	if err != nil {
		t.Fatalf("validate workflow: %v", err)
	}
	if validated.Name != "basic" {
		t.Fatalf("expected name basic, got %q", validated.Name)
	}
	if len(validated.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(validated.Steps))
	}
	if validated.Steps[1].Type != StepTypeBash {
		t.Fatalf("expected normalized step type bash, got %q", validated.Steps[1].Type)
	}
}

func TestValidateWorkflowCycle(t *testing.T) {
	wf := &Workflow{
		Name:   "cycle",
		Source: "testdata/cycle.toml",
		Steps: []WorkflowStep{
			{ID: "a", Type: StepTypeBash, Cmd: "echo a", DependsOn: []string{"b"}},
			{ID: "b", Type: StepTypeBash, Cmd: "echo b", DependsOn: []string{"a"}},
		},
	}

	_, err := ValidateWorkflow(wf)
	if err == nil {
		t.Fatalf("expected cycle error")
	}

	var list *ErrorList
	if !errors.As(err, &list) {
		t.Fatalf("expected ErrorList, got %T", err)
	}
	found := false
	for _, item := range list.Errors {
		if item.Code == ErrCodeCycle {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected cycle error code")
	}
}

func TestValidateWorkflowMissingName(t *testing.T) {
	path := filepath.Join("testdata", "invalid-missing-name.toml")
	wf, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("load workflow: %v", err)
	}

	_, err = ValidateWorkflow(wf)
	if err == nil {
		t.Fatalf("expected validation error")
	}

	var list *ErrorList
	if !errors.As(err, &list) {
		t.Fatalf("expected ErrorList, got %T", err)
	}

	found := false
	for _, item := range list.Errors {
		if item.Code == ErrCodeMissingField && item.Field == "name" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing name error")
	}
}

func TestValidateWorkflowUnknownDependency(t *testing.T) {
	path := filepath.Join("testdata", "invalid-unknown-dep.toml")
	wf, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("load workflow: %v", err)
	}

	_, err = ValidateWorkflow(wf)
	if err == nil {
		t.Fatalf("expected validation error")
	}

	var list *ErrorList
	if !errors.As(err, &list) {
		t.Fatalf("expected ErrorList, got %T", err)
	}

	found := false
	for _, item := range list.Errors {
		if item.Code == ErrCodeMissingStep {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing dependency error")
	}
}
