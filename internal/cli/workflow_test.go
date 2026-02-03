package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tOgg1/forge/internal/config"
	"github.com/tOgg1/forge/internal/workflows"
)

func TestWorkflowCLIListAndShow(t *testing.T) {
	repo := setupWorkflowRepo(t)
	writeFixture(t, repo, "basic.toml", filepath.Join("..", "workflows", "testdata", "valid-basic.toml"))

	cleanupConfig := withTempConfig(t, repo)
	defer cleanupConfig()

	withWorkingDir(t, repo, func() {
		originalJSON := jsonOutput
		originalJSONL := jsonlOutput
		jsonOutput = true
		jsonlOutput = false
		defer func() {
			jsonOutput = originalJSON
			jsonlOutput = originalJSONL
		}()

		out, err := captureStdout(func() error {
			return workflowListCmd.RunE(workflowListCmd, nil)
		})
		if err != nil {
			t.Fatalf("workflow ls: %v", err)
		}

		var items []workflows.Workflow
		if err := json.Unmarshal([]byte(out), &items); err != nil {
			t.Fatalf("parse workflow ls output: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 workflow, got %d", len(items))
		}
		if items[0].Name != "basic" {
			t.Fatalf("expected workflow name basic, got %q", items[0].Name)
		}

		jsonOutput = false

		out, err = captureStdout(func() error {
			return workflowShowCmd.RunE(workflowShowCmd, []string{"basic"})
		})
		if err != nil {
			t.Fatalf("workflow show: %v", err)
		}

		golden := readGolden(t, "workflow_show.golden.txt")
		normalized := strings.ReplaceAll(out, filepath.Join("/private", repo), "/repo")
		normalized = strings.ReplaceAll(normalized, repo, "/repo")
		if strings.TrimSpace(normalized) != strings.TrimSpace(golden) {
			t.Fatalf("workflow show output mismatch\nGot:\n%s\nWant:\n%s", normalized, golden)
		}
	})
}

func TestWorkflowCLIValidateInvalidGolden(t *testing.T) {
	repo := setupWorkflowRepo(t)
	writeFixture(t, repo, "bad-dep.toml", filepath.Join("..", "workflows", "testdata", "invalid-unknown-dep.toml"))

	cleanupConfig := withTempConfig(t, repo)
	defer cleanupConfig()

	withWorkingDir(t, repo, func() {
		originalJSON := jsonOutput
		originalJSONL := jsonlOutput
		jsonOutput = true
		jsonlOutput = false
		defer func() {
			jsonOutput = originalJSON
			jsonlOutput = originalJSONL
		}()

		out, err := captureStdout(func() error {
			return workflowValidateCmd.RunE(workflowValidateCmd, []string{"bad-dep"})
		})
		if err == nil {
			t.Fatalf("expected validation error")
		}
		var exitErr *ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("expected ExitError, got %T", err)
		}
		if exitErr.Code != 1 {
			t.Fatalf("expected exit code 1, got %d", exitErr.Code)
		}

		normalized := strings.ReplaceAll(out, filepath.Join("/private", repo), "<repo>")
		normalized = strings.ReplaceAll(normalized, repo, "<repo>")
		golden := readGolden(t, "workflow_validate_invalid.golden.json")
		if strings.TrimSpace(normalized) != strings.TrimSpace(golden) {
			t.Fatalf("workflow validate output mismatch\nGot:\n%s\nWant:\n%s", normalized, golden)
		}
	})
}

func setupWorkflowRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(repo, ".forge", "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}

	return repo
}

func writeFixture(t *testing.T, repo, name, fixturePath string) {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	path := filepath.Join(repo, ".forge", "workflows", name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func withTempConfig(t *testing.T, repo string) func() {
	t.Helper()
	original := appConfig

	cfg := config.DefaultConfig()
	cfg.Global.DataDir = filepath.Join(repo, "data")
	cfg.Global.ConfigDir = filepath.Join(repo, "config")
	appConfig = cfg

	if err := os.MkdirAll(cfg.Global.DataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	if err := os.MkdirAll(cfg.Global.ConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	return func() { appConfig = original }
}

func withWorkingDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	original, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(original) }()
	fn()
}

func captureStdout(fn func() error) (string, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	original := os.Stdout
	os.Stdout = w
	cmdErr := fn()
	_ = w.Close()
	os.Stdout = original

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String(), cmdErr
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve test file path")
	}
	base := filepath.Dir(file)
	path := filepath.Join(base, "testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	return string(data)
}
