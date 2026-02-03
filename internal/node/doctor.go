// Package node provides node diagnostics for Forge.
package node

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/ssh"
	"github.com/tOgg1/forge/internal/tmux"
)

// CheckStatus indicates the result of a diagnostic check.
type CheckStatus string

const (
	CheckPass CheckStatus = "pass"
	CheckWarn CheckStatus = "warn"
	CheckFail CheckStatus = "fail"
	CheckSkip CheckStatus = "skip"
)

// DoctorCheck represents a single diagnostic result.
type DoctorCheck struct {
	Name    string      `json:"name"`
	Status  CheckStatus `json:"status"`
	Details string      `json:"details,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// DoctorReport aggregates diagnostic results for a node.
type DoctorReport struct {
	NodeID    string        `json:"node_id"`
	NodeName  string        `json:"node_name"`
	Checks    []DoctorCheck `json:"checks"`
	Success   bool          `json:"success"`
	CheckedAt time.Time     `json:"checked_at"`
}

// Doctor runs diagnostics against the given node.
func (s *Service) Doctor(ctx context.Context, node *models.Node) (*DoctorReport, error) {
	if node == nil {
		return nil, errors.New("node is required")
	}

	var executor execer
	var closeFn func() error

	if node.IsLocal {
		executor = &localExecutor{}
		closeFn = func() error { return nil }
	} else {
		user, host, port := ParseSSHTarget(node.SSHTarget)
		opts := ssh.ConnectionOptions{
			Host:    host,
			Port:    port,
			User:    user,
			KeyPath: node.SSHKeyPath,
			Timeout: s.DefaultTimeout,
		}
		sshExec, err := s.createExecutor(node.SSHBackend, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create SSH executor: %w", err)
		}
		executor = sshExec
		closeFn = sshExec.Close
	}
	defer func() {
		if err := closeFn(); err != nil {
			s.logger.Warn().Err(err).Msg("failed to close node executor")
		}
	}()

	checks := runDoctorChecks(ctx, executor, node.IsLocal)

	report := &DoctorReport{
		NodeID:    node.ID,
		NodeName:  node.Name,
		Checks:    checks,
		Success:   allChecksPassing(checks),
		CheckedAt: time.Now().UTC(),
	}

	return report, nil
}

type execer interface {
	Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error)
}

type localExecutor struct{}

func (l *localExecutor) Exec(ctx context.Context, cmd string) ([]byte, []byte, error) {
	command := exec.CommandContext(ctx, "sh", "-c", cmd)
	var stdoutBuf, stderrBuf strings.Builder
	command.Stdout = &stdoutBuf
	command.Stderr = &stderrBuf
	err := command.Run()
	return []byte(stdoutBuf.String()), []byte(stderrBuf.String()), err
}

func runDoctorChecks(ctx context.Context, executor execer, isLocal bool) []DoctorCheck {
	checks := []DoctorCheck{}

	checks = append(checks, checkSSH(ctx, executor, isLocal))
	checks = append(checks, checkTmux(ctx, executor))
	checks = append(checks, checkDisk(ctx, executor))
	checks = append(checks, checkCPU(ctx, executor))
	checks = append(checks, checkMemory(ctx, executor))

	cliChecks := []string{"opencode", "claude", "codex", "gemini"}
	for _, cli := range cliChecks {
		checks = append(checks, checkCLI(ctx, executor, cli))
	}

	return checks
}

func allChecksPassing(checks []DoctorCheck) bool {
	for _, check := range checks {
		if check.Status == CheckFail {
			return false
		}
	}
	return true
}

func checkSSH(ctx context.Context, executor execer, isLocal bool) DoctorCheck {
	if isLocal {
		return DoctorCheck{
			Name:    "ssh",
			Status:  CheckSkip,
			Details: "local node",
		}
	}

	stdout, stderr, err := executor.Exec(ctx, "echo ok")
	if err != nil {
		return DoctorCheck{
			Name:   "ssh",
			Status: CheckFail,
			Error:  fmt.Sprintf("%v (stderr: %s)", err, strings.TrimSpace(string(stderr))),
		}
	}

	if strings.TrimSpace(string(stdout)) != "ok" {
		return DoctorCheck{
			Name:   "ssh",
			Status: CheckFail,
			Error:  fmt.Sprintf("unexpected response: %s", strings.TrimSpace(string(stdout))),
		}
	}

	return DoctorCheck{Name: "ssh", Status: CheckPass}
}

func checkTmux(ctx context.Context, executor execer) DoctorCheck {
	stdout, stderr, err := executor.Exec(ctx, "tmux -V 2>/dev/null")
	if err != nil {
		return DoctorCheck{
			Name:   "tmux",
			Status: CheckFail,
			Error:  strings.TrimSpace(string(stderr)),
		}
	}

	details := strings.TrimSpace(string(stdout))
	if details == "" {
		details = "tmux detected"
	}
	version, err := tmux.ParseVersion(details)
	if err != nil {
		return DoctorCheck{
			Name:    "tmux",
			Status:  CheckWarn,
			Details: details,
		}
	}

	if version.LessThan(tmux.MinVersion) {
		return DoctorCheck{
			Name:    "tmux",
			Status:  CheckWarn,
			Details: fmt.Sprintf("%s (min %s)", details, tmux.MinVersion.String()),
		}
	}

	return DoctorCheck{
		Name:    "tmux",
		Status:  CheckPass,
		Details: details,
	}
}

func checkDisk(ctx context.Context, executor execer) DoctorCheck {
	stdout, stderr, err := executor.Exec(ctx, "df -kP / | tail -1")
	if err != nil {
		return DoctorCheck{
			Name:   "disk",
			Status: CheckFail,
			Error:  fmt.Sprintf("%v (stderr: %s)", err, strings.TrimSpace(string(stderr))),
		}
	}

	fields := strings.Fields(strings.TrimSpace(string(stdout)))
	if len(fields) < 5 {
		return DoctorCheck{
			Name:    "disk",
			Status:  CheckWarn,
			Details: strings.TrimSpace(string(stdout)),
		}
	}

	usage := strings.TrimSuffix(fields[4], "%")
	usagePct, err := strconv.Atoi(usage)
	if err != nil {
		return DoctorCheck{
			Name:    "disk",
			Status:  CheckWarn,
			Details: strings.TrimSpace(string(stdout)),
		}
	}

	status := CheckPass
	switch {
	case usagePct >= 95:
		status = CheckFail
	case usagePct >= 90:
		status = CheckWarn
	}

	return DoctorCheck{
		Name:    "disk",
		Status:  status,
		Details: fmt.Sprintf("%d%% used", usagePct),
	}
}

func checkCPU(ctx context.Context, executor execer) DoctorCheck {
	stdout, stderr, err := executor.Exec(ctx, "nproc 2>/dev/null || getconf _NPROCESSORS_ONLN")
	if err != nil {
		return DoctorCheck{
			Name:   "cpu",
			Status: CheckWarn,
			Error:  strings.TrimSpace(string(stderr)),
		}
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(stdout)))
	if err != nil || count < 1 {
		return DoctorCheck{
			Name:    "cpu",
			Status:  CheckWarn,
			Details: strings.TrimSpace(string(stdout)),
		}
	}

	return DoctorCheck{
		Name:    "cpu",
		Status:  CheckPass,
		Details: fmt.Sprintf("%d cores", count),
	}
}

func checkMemory(ctx context.Context, executor execer) DoctorCheck {
	stdout, stderr, err := executor.Exec(ctx, "awk '/MemTotal/ {print $2}' /proc/meminfo")
	if err != nil {
		return DoctorCheck{
			Name:   "memory",
			Status: CheckWarn,
			Error:  strings.TrimSpace(string(stderr)),
		}
	}

	kb, err := strconv.Atoi(strings.TrimSpace(string(stdout)))
	if err != nil || kb <= 0 {
		return DoctorCheck{
			Name:    "memory",
			Status:  CheckWarn,
			Details: strings.TrimSpace(string(stdout)),
		}
	}

	gb := float64(kb) / 1024.0 / 1024.0
	return DoctorCheck{
		Name:    "memory",
		Status:  CheckPass,
		Details: fmt.Sprintf("%.1f GB", gb),
	}
}

func checkCLI(ctx context.Context, executor execer, name string) DoctorCheck {
	stdout, _, err := executor.Exec(ctx, fmt.Sprintf("command -v %s", name))
	if err != nil || strings.TrimSpace(string(stdout)) == "" {
		return DoctorCheck{
			Name:    fmt.Sprintf("cli:%s", name),
			Status:  CheckWarn,
			Details: "not found",
		}
	}

	return DoctorCheck{
		Name:    fmt.Sprintf("cli:%s", name),
		Status:  CheckPass,
		Details: strings.TrimSpace(string(stdout)),
	}
}
