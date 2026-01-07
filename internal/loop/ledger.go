package loop

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tOgg1/forge/internal/models"
	"gopkg.in/yaml.v3"
)

func ensureLedgerFile(loop *models.Loop) error {
	if loop.LedgerPath == "" {
		return nil
	}

	if _, err := os.Stat(loop.LedgerPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(loop.LedgerPath), 0o755); err != nil {
		return err
	}

	content := strings.Builder{}
	content.WriteString("---\n")
	content.WriteString(fmt.Sprintf("loop_id: %s\n", loop.ID))
	content.WriteString(fmt.Sprintf("loop_name: %s\n", loop.Name))
	content.WriteString(fmt.Sprintf("repo_path: %s\n", loop.RepoPath))
	content.WriteString(fmt.Sprintf("created_at: %s\n", time.Now().UTC().Format(time.RFC3339)))
	content.WriteString("---\n\n")
	content.WriteString(fmt.Sprintf("# Loop Ledger: %s\n\n", loop.Name))

	return os.WriteFile(loop.LedgerPath, []byte(content.String()), 0o644)
}

func appendLedgerEntry(loop *models.Loop, run *models.LoopRun, profile *models.Profile, outputTail string, tailLines int) error {
	if loop.LedgerPath == "" {
		return nil
	}

	f, err := os.OpenFile(loop.LedgerPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	entry := strings.Builder{}
	entry.WriteString(fmt.Sprintf("## %s\n\n", time.Now().UTC().Format(time.RFC3339)))
	entry.WriteString(fmt.Sprintf("- run_id: %s\n", run.ID))
	entry.WriteString(fmt.Sprintf("- loop_name: %s\n", loop.Name))
	entry.WriteString(fmt.Sprintf("- status: %s\n", run.Status))
	entry.WriteString(fmt.Sprintf("- profile: %s\n", profile.Name))
	if profile.Harness != "" {
		entry.WriteString(fmt.Sprintf("- harness: %s\n", profile.Harness))
	}
	if profile.AuthKind != "" {
		entry.WriteString(fmt.Sprintf("- auth_kind: %s\n", profile.AuthKind))
	}
	entry.WriteString(fmt.Sprintf("- prompt_source: %s\n", run.PromptSource))
	if run.PromptPath != "" {
		entry.WriteString(fmt.Sprintf("- prompt_path: %s\n", run.PromptPath))
	}
	entry.WriteString(fmt.Sprintf("- prompt_override: %t\n", run.PromptOverride))
	entry.WriteString(fmt.Sprintf("- started_at: %s\n", run.StartedAt.UTC().Format(time.RFC3339)))
	if run.FinishedAt != nil {
		entry.WriteString(fmt.Sprintf("- finished_at: %s\n", run.FinishedAt.UTC().Format(time.RFC3339)))
	}
	if run.ExitCode != nil {
		entry.WriteString(fmt.Sprintf("- exit_code: %d\n", *run.ExitCode))
	}
	entry.WriteString("\n")

	outputTail = limitOutputLines(outputTail, tailLines)
	if strings.TrimSpace(outputTail) != "" {
		entry.WriteString("```\n")
		entry.WriteString(strings.TrimSpace(outputTail))
		entry.WriteString("\n```\n")
	}

	ledgerConfig := loadLedgerConfig(loop.RepoPath)
	if gitSummary := buildGitSummary(loop.RepoPath, ledgerConfig); strings.TrimSpace(gitSummary) != "" {
		entry.WriteString("\n### Git Summary\n\n```\n")
		entry.WriteString(strings.TrimSpace(gitSummary))
		entry.WriteString("\n```\n")
	}
	entry.WriteString("\n")

	_, err = f.WriteString(entry.String())
	return err
}

func limitOutputLines(text string, maxLines int) string {
	if maxLines <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

type repoConfig struct {
	Ledger ledgerConfig `yaml:"ledger"`
}

type ledgerConfig struct {
	GitStatus   bool `yaml:"git_status"`
	GitDiffStat bool `yaml:"git_diff_stat"`
}

func loadLedgerConfig(repoPath string) ledgerConfig {
	path := filepath.Join(repoPath, ".forge", "forge.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return ledgerConfig{}
	}

	var cfg repoConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ledgerConfig{}
	}
	return cfg.Ledger
}

func buildGitSummary(repoPath string, cfg ledgerConfig) string {
	if !cfg.GitStatus && !cfg.GitDiffStat {
		return ""
	}
	if !isGitRepo(repoPath) {
		return ""
	}

	lines := make([]string, 0, 8)
	if cfg.GitStatus {
		status, err := runGit(repoPath, "status", "--porcelain")
		if err == nil {
			lines = append(lines, "status --porcelain:")
			status = strings.TrimSpace(status)
			if status == "" {
				lines = append(lines, "  (clean)")
			} else {
				lines = append(lines, status)
			}
		}
	}
	if cfg.GitDiffStat {
		diffStat, err := runGit(repoPath, "diff", "--stat")
		if err == nil {
			lines = append(lines, "diff --stat:")
			diffStat = strings.TrimSpace(diffStat)
			if diffStat == "" {
				lines = append(lines, "  (clean)")
			} else {
				lines = append(lines, diffStat)
			}
		}
	}

	return strings.Join(lines, "\n")
}

func isGitRepo(repoPath string) bool {
	output, err := runGit(repoPath, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	return strings.TrimSpace(output) == "true"
}

func runGit(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}
