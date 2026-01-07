package harness

import (
	"context"
	"strings"
	"testing"

	"github.com/tOgg1/forge/internal/models"
)

func TestBuildExecutionEnvMode(t *testing.T) {
	profile := models.Profile{
		Name:            "claude",
		Harness:         models.HarnessClaude,
		PromptMode:      models.PromptModeEnv,
		CommandTemplate: "claude -p \"$FORGE_PROMPT_CONTENT\"",
	}

	exec, err := BuildExecution(context.Background(), profile, "", "hello")
	if err != nil {
		t.Fatalf("BuildExecution failed: %v", err)
	}

	found := false
	for _, value := range exec.Env {
		if value == "FORGE_PROMPT_CONTENT=hello" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected FORGE_PROMPT_CONTENT env to be set")
	}
}

func TestBuildExecutionPathMode(t *testing.T) {
	profile := models.Profile{
		Name:            "pi",
		Harness:         models.HarnessPi,
		PromptMode:      models.PromptModePath,
		AuthHome:        "/tmp/pi",
		CommandTemplate: "pi -p \"{prompt}\"",
	}

	exec, err := BuildExecution(context.Background(), profile, "/repo/PROMPT.md", "")
	if err != nil {
		t.Fatalf("BuildExecution failed: %v", err)
	}

	command := strings.Join(exec.Cmd.Args, " ")
	if !strings.Contains(command, "/repo/PROMPT.md") {
		t.Fatalf("expected prompt path in command, got %s", command)
	}

	found := false
	for _, value := range exec.Env {
		if value == "PI_CODING_AGENT_DIR=/tmp/pi" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected PI_CODING_AGENT_DIR env to be set")
	}
}

func TestBuildExecutionStdinMode(t *testing.T) {
	profile := models.Profile{
		Name:            "codex",
		Harness:         models.HarnessCodex,
		PromptMode:      models.PromptModeStdin,
		CommandTemplate: "codex exec --full-auto -",
	}

	exec, err := BuildExecution(context.Background(), profile, "", "prompt")
	if err != nil {
		t.Fatalf("BuildExecution failed: %v", err)
	}

	if exec.Stdin == nil {
		t.Fatalf("expected stdin to be set")
	}
}
