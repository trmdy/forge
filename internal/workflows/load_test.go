package workflows

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkflowParseErrorIncludesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.toml")
	data := []byte("name = \"bad\"\nsteps = [\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write test workflow: %v", err)
	}

	_, err := LoadWorkflow(path)
	if err == nil {
		t.Fatalf("expected parse error")
	}

	var list *ErrorList
	if !errors.As(err, &list) {
		t.Fatalf("expected ErrorList, got %T", err)
	}
	if len(list.Errors) == 0 {
		t.Fatalf("expected errors")
	}

	errItem := list.Errors[0]
	if errItem.Path != path {
		t.Fatalf("expected path %q, got %q", path, errItem.Path)
	}
	if errItem.Code != ErrCodeParse {
		t.Fatalf("expected parse code, got %q", errItem.Code)
	}
	if errItem.Line == 0 {
		t.Fatalf("expected line info on parse error")
	}
}
