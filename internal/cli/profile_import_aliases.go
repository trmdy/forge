package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/harness"
	"github.com/tOgg1/forge/internal/models"
)

var profileImportAliasesCmd = &cobra.Command{
	Use:   "import-aliases [alias...]",
	Short: "Import profiles from shell aliases",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		aliasNames := args
		if len(aliasNames) == 0 {
			aliasNames = defaultAliasNames
		}

		aliasFile := resolveAliasFile()
		shellPath := resolveAliasShell()

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewProfileRepository(database)
		result := importAliasResult{}

		ctx := context.Background()
		for _, aliasName := range aliasNames {
			aliasName = strings.TrimSpace(aliasName)
			if aliasName == "" {
				continue
			}

			aliasOutput, err := getAliasOutput(aliasName, aliasFile, shellPath)
			if err != nil {
				result.Missing = append(result.Missing, aliasName)
				continue
			}

			aliasCmd := parseAliasCommand(aliasOutput, aliasName)
			if aliasCmd == "" {
				result.Missing = append(result.Missing, aliasName)
				continue
			}

			profile, err := buildAliasProfile(aliasName, aliasCmd)
			if err != nil {
				return err
			}

			if _, err := repo.GetByName(ctx, profile.Name); err == nil {
				result.Skipped = append(result.Skipped, profile.Name)
				continue
			} else if !errors.Is(err, db.ErrProfileNotFound) {
				return err
			}

			if err := repo.Create(ctx, profile); err != nil {
				if errors.Is(err, db.ErrProfileAlreadyExists) {
					result.Skipped = append(result.Skipped, profile.Name)
					continue
				}
				return err
			}

			result.Created = append(result.Created, profile.Name)
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, result)
		}

		printAliasResult(result)
		return nil
	},
}

type importAliasResult struct {
	Created []string `json:"created"`
	Skipped []string `json:"skipped"`
	Missing []string `json:"missing"`
}

var defaultAliasNames = []string{
	"oc1",
	"oc2",
	"oc3",
	"codex1",
	"codex2",
	"cc1",
	"cc2",
	"cc3",
	"pi",
}

func printAliasResult(result importAliasResult) {
	if len(result.Created) == 0 && len(result.Skipped) == 0 && len(result.Missing) == 0 {
		fmt.Fprintln(os.Stdout, "No aliases processed")
		return
	}

	if len(result.Created) > 0 {
		fmt.Fprintf(os.Stdout, "Created: %s\n", strings.Join(result.Created, ", "))
	}
	if len(result.Skipped) > 0 {
		fmt.Fprintf(os.Stdout, "Skipped: %s\n", strings.Join(result.Skipped, ", "))
	}
	if len(result.Missing) > 0 {
		fmt.Fprintf(os.Stdout, "Missing: %s\n", strings.Join(result.Missing, ", "))
	}
}

func resolveAliasFile() string {
	aliasFile := strings.TrimSpace(os.Getenv("FORGE_ALIAS_FILE"))
	if aliasFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		aliasFile = filepath.Join(home, ".zsh_aliases")
	}
	return expandHome(aliasFile)
}

func resolveAliasShell() string {
	shellPath := strings.TrimSpace(os.Getenv("FORGE_ALIAS_SHELL"))
	if shellPath == "" {
		shellPath = strings.TrimSpace(os.Getenv("SHELL"))
	}
	if shellPath == "" {
		shellPath = "/bin/zsh"
	}
	return shellPath
}

var errAliasNotFound = errors.New("alias not found")

func getAliasOutput(aliasName, aliasFile, shellPath string) (string, error) {
	aliasName = strings.TrimSpace(aliasName)
	if aliasName == "" {
		return "", errAliasNotFound
	}

	if shellPath != "" {
		cmd := exec.Command(shellPath, "-lc", fmt.Sprintf("source %q >/dev/null 2>&1; alias %s", aliasFile, aliasName))
		if output, err := cmd.Output(); err == nil {
			text := strings.TrimSpace(string(output))
			if text != "" {
				return text, nil
			}
		}
	}

	if aliasFile != "" {
		data, err := os.ReadFile(aliasFile)
		if err == nil {
			re := regexp.MustCompile("^alias\\s+" + regexp.QuoteMeta(aliasName) + "=")
			for _, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				if re.MatchString(trimmed) {
					return trimmed, nil
				}
			}
		}
	}

	return "", errAliasNotFound
}

func parseAliasCommand(aliasOutput, aliasName string) string {
	line := strings.TrimSpace(aliasOutput)
	if line == "" {
		return ""
	}
	if idx := strings.Index(line, "\n"); idx != -1 {
		line = line[:idx]
	}
	line = strings.TrimPrefix(line, "alias ")
	if strings.HasPrefix(line, aliasName+"=") {
		line = strings.TrimPrefix(line, aliasName+"=")
	} else if idx := strings.Index(line, "="); idx != -1 {
		line = line[idx+1:]
	}
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "\"")
	line = strings.TrimSuffix(line, "\"")
	line = strings.TrimPrefix(line, "'")
	line = strings.TrimSuffix(line, "'")
	return strings.TrimSpace(line)
}

func buildAliasProfile(aliasName, aliasCmd string) (*models.Profile, error) {
	aliasCmd = strings.TrimSpace(aliasCmd)
	if aliasCmd == "" {
		return nil, fmt.Errorf("alias %q resolved to empty command", aliasName)
	}

	env := parseLeadingEnv(aliasCmd)
	authHome := resolveAuthHome(aliasName, env)

	harnessValue, promptMode, commandTemplate := resolveAliasCommand(aliasName, aliasCmd)
	if promptMode == "" {
		promptMode = harness.DefaultPromptMode(harnessValue)
	}

	profile := &models.Profile{
		Name:            aliasName,
		Harness:         harnessValue,
		AuthHome:        authHome,
		PromptMode:      promptMode,
		CommandTemplate: commandTemplate,
		MaxConcurrency:  1,
	}

	return profile, nil
}

func resolveAliasCommand(aliasName, aliasCmd string) (models.Harness, models.PromptMode, string) {
	switch strings.ToLower(aliasName) {
	case "oc1", "oc2", "oc3":
		return buildOpenCode(aliasCmd)
	case "codex1", "codex2":
		return buildCodex(aliasCmd)
	case "cc1", "cc2", "cc3":
		return buildClaude(aliasCmd)
	case "pi":
		return buildPi(aliasCmd)
	}

	harnessValue := inferHarness(aliasName, aliasCmd)
	switch harnessValue {
	case models.HarnessOpenCode:
		return buildOpenCode(aliasCmd)
	case models.HarnessCodex:
		return buildCodex(aliasCmd)
	case models.HarnessClaude:
		return buildClaude(aliasCmd)
	case models.HarnessPi:
		return buildPi(aliasCmd)
	default:
		return buildPi(aliasCmd)
	}
}

func buildOpenCode(aliasCmd string) (models.Harness, models.PromptMode, string) {
	model := strings.TrimSpace(os.Getenv("FORGE_OPENCODE_MODEL"))
	if model == "" {
		model = "anthropic/claude-opus-4-5"
	}
	command := fmt.Sprintf("%s run --model %s \"$FORGE_PROMPT_CONTENT\"", aliasCmd, model)
	return models.HarnessOpenCode, models.PromptModeEnv, command
}

func buildCodex(aliasCmd string) (models.Harness, models.PromptMode, string) {
	command := fmt.Sprintf("%s exec --full-auto -", aliasCmd)
	return models.HarnessCodex, models.PromptModeStdin, command
}

func buildClaude(aliasCmd string) (models.Harness, models.PromptMode, string) {
	command := fmt.Sprintf("%s -p \"$FORGE_PROMPT_CONTENT\"", aliasCmd)
	if !strings.Contains(command, "--dangerously-skip-permissions") {
		command = command + " --dangerously-skip-permissions"
	}
	return models.HarnessClaude, models.PromptModeEnv, command
}

func buildPi(aliasCmd string) (models.Harness, models.PromptMode, string) {
	command := fmt.Sprintf("%s -p \"$FORGE_PROMPT_CONTENT\"", aliasCmd)
	return models.HarnessPi, models.PromptModeEnv, command
}

func inferHarness(aliasName, aliasCmd string) models.Harness {
	candidate := strings.ToLower(aliasName + " " + aliasCmd)
	switch {
	case strings.Contains(candidate, "opencode"):
		return models.HarnessOpenCode
	case strings.Contains(candidate, "codex"):
		return models.HarnessCodex
	case strings.Contains(candidate, "claude"):
		return models.HarnessClaude
	case strings.Contains(candidate, "pi"):
		return models.HarnessPi
	default:
		return models.HarnessPi
	}
}

func parseLeadingEnv(command string) map[string]string {
	fields := strings.Fields(command)
	env := make(map[string]string)
	for _, field := range fields {
		if !strings.Contains(field, "=") || strings.HasPrefix(field, "=") {
			break
		}
		parts := strings.SplitN(field, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			break
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"'")
		env[key] = value
	}
	return env
}

func resolveAuthHome(aliasName string, env map[string]string) string {
	if value := env["PI_CODING_AGENT_DIR"]; value != "" {
		return expandHome(value)
	}
	if value := env["CODEX_HOME"]; value != "" {
		return expandHome(value)
	}
	if value := env["OPENCODE_HOME"]; value != "" {
		return expandHome(value)
	}
	if value := env["CLAUDE_HOME"]; value != "" {
		return expandHome(value)
	}
	if value := env["HOME"]; value != "" {
		return expandHome(value)
	}

	aliasLower := strings.ToLower(aliasName)
	if strings.HasPrefix(aliasLower, "codex") {
		if suffix := strings.TrimPrefix(aliasLower, "codex"); suffix != "" {
			return expandHome("~/.codex-" + suffix)
		}
	}
	if strings.HasPrefix(aliasLower, "oc") {
		if suffix := strings.TrimPrefix(aliasLower, "oc"); suffix != "" {
			return expandHome("~/.opencode-" + suffix)
		}
	}
	if strings.HasPrefix(aliasLower, "cc") {
		if suffix := strings.TrimPrefix(aliasLower, "cc"); suffix != "" {
			return expandHome("~/.claude-" + suffix)
		}
	}

	return ""
}

func expandHome(path string) string {
	path = os.ExpandEnv(path)
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}
