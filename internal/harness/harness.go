package harness

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tOgg1/forge/internal/models"
)

// Execution represents a prepared harness execution.
type Execution struct {
	Cmd   *exec.Cmd
	Stdin io.Reader
	Env   []string
}

// BuildExecution prepares a harness command based on profile and prompt settings.
func BuildExecution(ctx context.Context, profile models.Profile, promptPath, promptContent string) (*Execution, error) {
	command := strings.TrimSpace(profile.CommandTemplate)
	if command == "" {
		return nil, errors.New("command template is required")
	}

	if len(profile.ExtraArgs) > 0 {
		command = command + " " + strings.Join(profile.ExtraArgs, " ")
	}

	promptMode := profile.PromptMode
	if promptMode == "" {
		promptMode = models.PromptModeEnv
	}

	codexConfig := ""
	if profile.Harness == models.HarnessCodex {
		codexConfig = resolveCodexConfigPath(profile)
		command = applyCodexSandbox(command, codexConfig)
	}

	switch promptMode {
	case models.PromptModePath:
		if promptPath == "" {
			return nil, errors.New("prompt path is required for path mode")
		}
		command = strings.ReplaceAll(command, "{prompt}", promptPath)
	case models.PromptModeEnv, models.PromptModeStdin:
		// no-op
	default:
		return nil, fmt.Errorf("unknown prompt mode %q", promptMode)
	}

	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	stdin := io.Reader(nil)

	env := baseEnv(profile, promptMode, promptContent, codexConfig)
	cmd.Env = env
	if promptMode == models.PromptModeStdin {
		stdin = strings.NewReader(promptContent)
		cmd.Stdin = stdin
	}

	return &Execution{Cmd: cmd, Stdin: stdin, Env: env}, nil
}

func baseEnv(profile models.Profile, mode models.PromptMode, promptContent, codexConfig string) []string {
	env := append([]string{}, defaultEnv()...)

	if profile.AuthHome != "" {
		// Don't set HOME for Claude, Codex, or OpenCode - it breaks tilde expansion in command templates.
		// Each tool uses its own config directory environment variable.
		if profile.Harness != models.HarnessClaude && profile.Harness != models.HarnessCodex && profile.Harness != models.HarnessOpenCode {
			env = append(env, "HOME="+profile.AuthHome)
		}
		if profile.Harness == models.HarnessCodex {
			env = append(env, "CODEX_HOME="+profile.AuthHome)
		}
		if profile.Harness == models.HarnessOpenCode {
			env = append(env, "OPENCODE_CONFIG_DIR="+profile.AuthHome)
			env = append(env, "XDG_DATA_HOME="+profile.AuthHome)
		}
	}

	if mode == models.PromptModeEnv {
		env = append(env, "FORGE_PROMPT_CONTENT="+promptContent)
	}

	if codexConfig != "" {
		env = append(env, "CODEX_CONFIG="+codexConfig)
	}

	if profile.Harness == models.HarnessPi && profile.AuthHome != "" {
		env = append(env, "PI_CODING_AGENT_DIR="+profile.AuthHome)
	}

	if profile.Harness == models.HarnessClaude && profile.AuthHome != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+profile.AuthHome)
	}

	for key, value := range profile.Env {
		env = append(env, key+"="+value)
	}

	return env
}

func defaultEnv() []string {
	return os.Environ()
}

func resolveCodexConfigPath(profile models.Profile) string {
	candidates := []string{}
	if profile.Env != nil {
		if configPath := profile.Env["CODEX_CONFIG"]; configPath != "" {
			candidates = append(candidates, configPath)
		}
	}
	if configPath := os.Getenv("CODEX_CONFIG"); configPath != "" {
		candidates = append(candidates, configPath)
	}
	if profile.AuthHome != "" {
		candidates = append(candidates, filepath.Join(profile.AuthHome, "config.toml"))
	}

	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".codex", "config.toml"))
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func detectCodexSandbox(configPath string) string {
	if configPath == "" {
		return ""
	}

	file, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		if strings.HasPrefix(line, "sandbox_mode") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			value := strings.TrimSpace(parts[1])
			value = strings.Trim(value, "\"")
			return value
		}
	}
	return ""
}

func applyCodexSandbox(command string, codexConfig string) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return trimmed
	}

	sandbox := detectCodexSandbox(codexConfig)
	if sandbox == "" {
		return trimmed
	}

	tokens := strings.Fields(trimmed)
	if len(tokens) == 0 {
		return trimmed
	}

	// --full-auto forces workspace-write, so remove it when a stricter sandbox is configured.
	if sandbox != "workspace-write" {
		tokens = filterTokens(tokens, map[string]struct{}{"--full-auto": {}})
	}

	// When sandbox_mode is configured, drop the bypass flag so config wins.
	tokens = filterTokens(tokens, map[string]struct{}{"--dangerously-bypass-approvals-and-sandbox": {}})

	if hasSandboxFlag(tokens) || sandbox == "workspace-write" {
		return strings.Join(tokens, " ")
	}

	if len(tokens) > 0 && tokens[len(tokens)-1] == "-" {
		tokens = append(tokens[:len(tokens)-1], "--sandbox", sandbox, "-")
		return strings.Join(tokens, " ")
	}
	tokens = append(tokens, "--sandbox", sandbox)
	return strings.Join(tokens, " ")
}

func filterTokens(tokens []string, remove map[string]struct{}) []string {
	if len(remove) == 0 {
		return tokens
	}
	out := tokens[:0]
	for _, token := range tokens {
		if _, ok := remove[token]; ok {
			continue
		}
		out = append(out, token)
	}
	return out
}

func hasSandboxFlag(tokens []string) bool {
	for _, token := range tokens {
		if token == "--sandbox" || strings.HasPrefix(token, "--sandbox=") {
			return true
		}
	}
	return false
}
