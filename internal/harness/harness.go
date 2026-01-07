package harness

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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

	env := baseEnv(profile, promptMode, promptContent)
	cmd.Env = env
	if promptMode == models.PromptModeStdin {
		stdin = strings.NewReader(promptContent)
		cmd.Stdin = stdin
	}

	return &Execution{Cmd: cmd, Stdin: stdin, Env: env}, nil
}

func baseEnv(profile models.Profile, mode models.PromptMode, promptContent string) []string {
	env := append([]string{}, defaultEnv()...)

	if profile.AuthHome != "" {
		env = append(env, "HOME="+profile.AuthHome)
		if profile.Harness == models.HarnessCodex {
			env = append(env, "CODEX_HOME="+profile.AuthHome)
		}
	}

	if mode == models.PromptModeEnv {
		env = append(env, "FORGE_PROMPT_CONTENT="+promptContent)
	}

	if profile.Harness == models.HarnessPi && profile.AuthHome != "" {
		env = append(env, "PI_CODING_AGENT_DIR="+profile.AuthHome)
	}

	for key, value := range profile.Env {
		env = append(env, key+"="+value)
	}

	return env
}

func defaultEnv() []string {
	return os.Environ()
}
