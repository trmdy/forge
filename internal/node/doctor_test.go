package node

import (
	"context"
	"errors"
	"testing"
)

type execResult struct {
	stdout string
	stderr string
	err    error
}

type fakeExecutor struct {
	results map[string]execResult
}

func (f *fakeExecutor) Exec(ctx context.Context, cmd string) ([]byte, []byte, error) {
	result, ok := f.results[cmd]
	if !ok {
		return nil, nil, errors.New("unexpected command")
	}
	return []byte(result.stdout), []byte(result.stderr), result.err
}

func TestRunDoctorChecks_AllPass(t *testing.T) {
	executor := &fakeExecutor{results: map[string]execResult{
		"echo ok":             {stdout: "ok"},
		"tmux -V 2>/dev/null": {stdout: "tmux 3.3a"},
		"df -kP / | tail -1":  {stdout: "overlay 100 10 90 10% /"},
		"nproc 2>/dev/null || getconf _NPROCESSORS_ONLN": {stdout: "4"},
		"awk '/MemTotal/ {print $2}' /proc/meminfo":      {stdout: "1048576"},
		"command -v opencode":                            {stdout: "/usr/bin/opencode"},
		"command -v claude":                              {stdout: "/usr/bin/claude"},
		"command -v codex":                               {stdout: "/usr/bin/codex"},
		"command -v gemini":                              {stdout: "/usr/bin/gemini"},
	}}

	checks := runDoctorChecks(context.Background(), executor, false)
	if len(checks) == 0 {
		t.Fatalf("expected checks")
	}
	if !allChecksPassing(checks) {
		t.Fatalf("expected all checks passing")
	}
}

func TestRunDoctorChecks_DiskFail(t *testing.T) {
	executor := &fakeExecutor{results: map[string]execResult{
		"echo ok":             {stdout: "ok"},
		"tmux -V 2>/dev/null": {stdout: "tmux 3.3a"},
		"df -kP / | tail -1":  {stdout: "overlay 100 10 90 95% /"},
		"nproc 2>/dev/null || getconf _NPROCESSORS_ONLN": {stdout: "4"},
		"awk '/MemTotal/ {print $2}' /proc/meminfo":      {stdout: "1048576"},
		"command -v opencode":                            {stdout: "/usr/bin/opencode"},
		"command -v claude":                              {stdout: "/usr/bin/claude"},
		"command -v codex":                               {stdout: "/usr/bin/codex"},
		"command -v gemini":                              {stdout: "/usr/bin/gemini"},
	}}

	checks := runDoctorChecks(context.Background(), executor, false)
	var diskCheck *DoctorCheck
	for i := range checks {
		if checks[i].Name == "disk" {
			diskCheck = &checks[i]
			break
		}
	}
	if diskCheck == nil {
		t.Fatalf("expected disk check")
	}
	if diskCheck.Status != CheckFail {
		t.Fatalf("expected disk check to fail, got %q", diskCheck.Status)
	}
}

func TestRunDoctorChecks_MissingCLIWarn(t *testing.T) {
	executor := &fakeExecutor{results: map[string]execResult{
		"echo ok":             {stdout: "ok"},
		"tmux -V 2>/dev/null": {stdout: "tmux 3.3a"},
		"df -kP / | tail -1":  {stdout: "overlay 100 10 90 10% /"},
		"nproc 2>/dev/null || getconf _NPROCESSORS_ONLN": {stdout: "4"},
		"awk '/MemTotal/ {print $2}' /proc/meminfo":      {stdout: "1048576"},
		"command -v opencode":                            {stdout: "/usr/bin/opencode"},
		"command -v claude":                              {stdout: ""},
		"command -v codex":                               {stdout: "/usr/bin/codex"},
		"command -v gemini":                              {stdout: "/usr/bin/gemini"},
	}}

	checks := runDoctorChecks(context.Background(), executor, false)
	var cliCheck *DoctorCheck
	for i := range checks {
		if checks[i].Name == "cli:claude" {
			cliCheck = &checks[i]
			break
		}
	}
	if cliCheck == nil {
		t.Fatalf("expected cli:claude check")
	}
	if cliCheck.Status != CheckWarn {
		t.Fatalf("expected cli warn, got %q", cliCheck.Status)
	}
}

func TestRunDoctorChecks_TmuxTooOldWarn(t *testing.T) {
	executor := &fakeExecutor{results: map[string]execResult{
		"echo ok":             {stdout: "ok"},
		"tmux -V 2>/dev/null": {stdout: "tmux 2.0"},
		"df -kP / | tail -1":  {stdout: "overlay 100 10 90 10% /"},
		"nproc 2>/dev/null || getconf _NPROCESSORS_ONLN": {stdout: "4"},
		"awk '/MemTotal/ {print $2}' /proc/meminfo":      {stdout: "1048576"},
		"command -v opencode":                            {stdout: "/usr/bin/opencode"},
		"command -v claude":                              {stdout: "/usr/bin/claude"},
		"command -v codex":                               {stdout: "/usr/bin/codex"},
		"command -v gemini":                              {stdout: "/usr/bin/gemini"},
	}}

	checks := runDoctorChecks(context.Background(), executor, false)
	var tmuxCheck *DoctorCheck
	for i := range checks {
		if checks[i].Name == "tmux" {
			tmuxCheck = &checks[i]
			break
		}
	}
	if tmuxCheck == nil {
		t.Fatalf("expected tmux check")
	}
	if tmuxCheck.Status != CheckWarn {
		t.Fatalf("expected tmux warn, got %q", tmuxCheck.Status)
	}
}
