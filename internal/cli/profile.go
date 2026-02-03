package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/harness"
	"github.com/tOgg1/forge/internal/models"
)

var (
	profileAddName           string
	profileAddAuthKind       string
	profileAddAuthHome       string
	profileAddPromptMode     string
	profileAddCommand        string
	profileAddModel          string
	profileAddExtraArgs      []string
	profileAddEnv            []string
	profileAddMaxConcurrency int

	profileEditName           string
	profileEditAuthKind       string
	profileEditAuthHome       string
	profileEditPromptMode     string
	profileEditCommand        string
	profileEditModel          string
	profileEditExtraArgs      []string
	profileEditEnv            []string
	profileEditMaxConcurrency int

	profileCooldownUntil string
)

func init() {
	rootCmd.AddCommand(profileCmd)

	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileAddCmd)
	profileCmd.AddCommand(profileEditCmd)
	profileCmd.AddCommand(profileRemoveCmd)
	profileCmd.AddCommand(profileInitCmd)
	profileCmd.AddCommand(profileDoctorCmd)
	profileCmd.AddCommand(profileCooldownCmd)

	profileCooldownCmd.AddCommand(profileCooldownSetCmd)
	profileCooldownCmd.AddCommand(profileCooldownClearCmd)

	profileAddCmd.Flags().StringVar(&profileAddName, "name", "", "profile name")
	profileAddCmd.Flags().StringVar(&profileAddAuthKind, "auth-kind", "", "auth kind (claude, codex, etc)")
	profileAddCmd.Flags().StringVar(&profileAddAuthHome, "home", "", "auth home directory")
	profileAddCmd.Flags().StringVar(&profileAddPromptMode, "prompt-mode", "", "prompt mode (env, stdin, path)")
	profileAddCmd.Flags().StringVar(&profileAddCommand, "command", "", "command template override")
	profileAddCmd.Flags().StringVar(&profileAddModel, "model", "", "model name")
	profileAddCmd.Flags().StringSliceVar(&profileAddExtraArgs, "extra-arg", nil, "extra argument (repeatable)")
	profileAddCmd.Flags().StringSliceVar(&profileAddEnv, "env", nil, "environment variable (KEY=VALUE)")
	profileAddCmd.Flags().IntVar(&profileAddMaxConcurrency, "max-concurrency", 0, "max concurrent runs for this profile")

	profileEditCmd.Flags().StringVar(&profileEditName, "name", "", "new profile name")
	profileEditCmd.Flags().StringVar(&profileEditAuthKind, "auth-kind", "", "auth kind (claude, codex, etc)")
	profileEditCmd.Flags().StringVar(&profileEditAuthHome, "home", "", "auth home directory")
	profileEditCmd.Flags().StringVar(&profileEditPromptMode, "prompt-mode", "", "prompt mode (env, stdin, path)")
	profileEditCmd.Flags().StringVar(&profileEditCommand, "command", "", "command template override")
	profileEditCmd.Flags().StringVar(&profileEditModel, "model", "", "model name")
	profileEditCmd.Flags().StringSliceVar(&profileEditExtraArgs, "extra-arg", nil, "extra argument (repeatable)")
	profileEditCmd.Flags().StringSliceVar(&profileEditEnv, "env", nil, "environment variable (KEY=VALUE)")
	profileEditCmd.Flags().IntVar(&profileEditMaxConcurrency, "max-concurrency", 0, "max concurrent runs for this profile")

	profileCooldownSetCmd.Flags().StringVar(&profileCooldownUntil, "until", "", "time or duration (e.g. 1h, 2025-01-01T00:00:00Z)")
}

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage harness profiles",
}

var profileListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List profiles",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewProfileRepository(database)
		profiles, err := repo.List(context.Background())
		if err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, profiles)
		}

		if len(profiles) == 0 {
			fmt.Fprintln(os.Stdout, "No profiles found")
			return nil
		}

		rows := make([][]string, 0, len(profiles))
		for _, profile := range profiles {
			cooldown := ""
			if profile.CooldownUntil != nil {
				cooldown = profile.CooldownUntil.UTC().Format(time.RFC3339)
			}
			rows = append(rows, []string{
				profile.Name,
				string(profile.Harness),
				profile.AuthKind,
				profile.AuthHome,
				fmt.Sprintf("%d", profile.MaxConcurrency),
				cooldown,
			})
		}

		return writeTable(os.Stdout, []string{"NAME", "HARNESS", "AUTH_KIND", "AUTH_HOME", "MAX_CONCURRENCY", "COOLDOWN"}, rows)
	},
}

var profileAddCmd = &cobra.Command{
	Use:   "add <harness>",
	Short: "Add a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		harnessName := args[0]
		if profileAddName == "" {
			return fmt.Errorf("--name is required")
		}

		harnessValue, err := parseHarness(harnessName)
		if err != nil {
			return err
		}

		promptMode := models.PromptMode(profileAddPromptMode)
		if promptMode == "" {
			promptMode = harness.DefaultPromptMode(harnessValue)
		} else if !isValidPromptMode(promptMode) {
			return fmt.Errorf("invalid prompt mode %q", profileAddPromptMode)
		}

		commandTemplate := strings.TrimSpace(profileAddCommand)
		if commandTemplate == "" {
			commandTemplate = harness.DefaultCommandTemplate(harnessValue, profileAddModel)
		}

		maxConcurrency := profileAddMaxConcurrency
		if !cmd.Flags().Changed("max-concurrency") {
			maxConcurrency = 1
		}

		profile := &models.Profile{
			Name:            profileAddName,
			Harness:         harnessValue,
			AuthKind:        profileAddAuthKind,
			AuthHome:        profileAddAuthHome,
			PromptMode:      promptMode,
			CommandTemplate: commandTemplate,
			Model:           profileAddModel,
			ExtraArgs:       profileAddExtraArgs,
			Env:             parseEnvPairs(profileAddEnv),
			MaxConcurrency:  maxConcurrency,
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewProfileRepository(database)
		if err := repo.Create(context.Background(), profile); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, profile)
		}

		fmt.Fprintf(os.Stdout, "Profile %q created\n", profile.Name)
		return nil
	},
}

var profileEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewProfileRepository(database)
		profile, err := repo.GetByName(context.Background(), name)
		if err != nil {
			return err
		}

		if cmd.Flags().Changed("name") {
			profile.Name = profileEditName
		}
		if cmd.Flags().Changed("auth-kind") {
			profile.AuthKind = profileEditAuthKind
		}
		if cmd.Flags().Changed("home") {
			profile.AuthHome = profileEditAuthHome
		}
		if cmd.Flags().Changed("prompt-mode") {
			promptMode := models.PromptMode(profileEditPromptMode)
			if !isValidPromptMode(promptMode) {
				return fmt.Errorf("invalid prompt mode %q", profileEditPromptMode)
			}
			profile.PromptMode = promptMode
		}
		if cmd.Flags().Changed("command") {
			profile.CommandTemplate = profileEditCommand
		}
		if cmd.Flags().Changed("model") {
			profile.Model = profileEditModel
		}
		if cmd.Flags().Changed("extra-arg") {
			profile.ExtraArgs = profileEditExtraArgs
		}
		if cmd.Flags().Changed("env") {
			profile.Env = parseEnvPairs(profileEditEnv)
		}
		if cmd.Flags().Changed("max-concurrency") {
			profile.MaxConcurrency = profileEditMaxConcurrency
		}

		if err := repo.Update(context.Background(), profile); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, profile)
		}

		fmt.Fprintf(os.Stdout, "Profile %q updated\n", profile.Name)
		return nil
	},
}

var profileRemoveCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Remove a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if !ConfirmDestructiveAction("profile", name, "This will remove the profile from Forge.") {
			return nil
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewProfileRepository(database)
		profile, err := repo.GetByName(context.Background(), name)
		if err != nil {
			return err
		}

		if err := repo.Delete(context.Background(), profile.ID); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{"deleted": true, "profile": name})
		}

		fmt.Fprintf(os.Stdout, "Profile %q removed\n", name)
		return nil
	},
}

var profileDoctorCmd = &cobra.Command{
	Use:   "doctor <name>",
	Short: "Check profile configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewProfileRepository(database)
		profile, err := repo.GetByName(context.Background(), name)
		if err != nil {
			return err
		}

		report := runProfileDoctor(profile)

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, report)
		}

		fmt.Fprintf(os.Stdout, "Profile %s\n", profile.Name)
		for _, check := range report.Checks {
			status := "OK"
			if !check.OK {
				status = "FAIL"
			}
			fmt.Fprintf(os.Stdout, "- [%s] %s: %s\n", status, check.Name, check.Details)
		}
		return nil
	},
}

var profileCooldownCmd = &cobra.Command{
	Use:   "cooldown",
	Short: "Manage profile cooldowns",
}

var profileCooldownSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Set profile cooldown",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if profileCooldownUntil == "" {
			return fmt.Errorf("--until is required")
		}

		until, err := parseTimeOrDuration(profileCooldownUntil)
		if err != nil {
			return err
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewProfileRepository(database)
		profile, err := repo.GetByName(context.Background(), args[0])
		if err != nil {
			return err
		}
		profile.CooldownUntil = &until
		if err := repo.Update(context.Background(), profile); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, profile)
		}

		fmt.Fprintf(os.Stdout, "Profile %q cooldown set to %s\n", profile.Name, until.UTC().Format(time.RFC3339))
		return nil
	},
}

var profileCooldownClearCmd = &cobra.Command{
	Use:   "clear <name>",
	Short: "Clear profile cooldown",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewProfileRepository(database)
		profile, err := repo.GetByName(context.Background(), args[0])
		if err != nil {
			return err
		}

		profile.CooldownUntil = nil
		if err := repo.Update(context.Background(), profile); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, profile)
		}

		fmt.Fprintf(os.Stdout, "Profile %q cooldown cleared\n", profile.Name)
		return nil
	},
}

type doctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Details string `json:"details"`
}

type profileDoctorReport struct {
	Profile string        `json:"profile"`
	Checks  []doctorCheck `json:"checks"`
}

func runProfileDoctor(profile *models.Profile) profileDoctorReport {
	checks := make([]doctorCheck, 0)

	if profile.AuthHome != "" {
		if _, err := os.Stat(profile.AuthHome); err == nil {
			checks = append(checks, doctorCheck{Name: "auth_home", OK: true, Details: profile.AuthHome})
		} else {
			checks = append(checks, doctorCheck{Name: "auth_home", OK: false, Details: err.Error()})
		}
	}

	command := strings.Fields(profile.CommandTemplate)
	if len(command) > 0 {
		if _, err := exec.LookPath(command[0]); err == nil {
			checks = append(checks, doctorCheck{Name: "command", OK: true, Details: command[0]})
		} else {
			checks = append(checks, doctorCheck{Name: "command", OK: false, Details: err.Error()})
		}
	}

	return profileDoctorReport{Profile: profile.Name, Checks: checks}
}

func parseHarness(value string) (models.Harness, error) {
	switch strings.ToLower(value) {
	case "pi":
		return models.HarnessPi, nil
	case "opencode":
		return models.HarnessOpenCode, nil
	case "codex":
		return models.HarnessCodex, nil
	case "claude", "claude-code":
		return models.HarnessClaude, nil
	case "droid", "factory":
		return models.HarnessDroid, nil
	default:
		return "", fmt.Errorf("unknown harness %q", value)
	}
}

func isValidPromptMode(mode models.PromptMode) bool {
	switch mode {
	case models.PromptModeEnv, models.PromptModeStdin, models.PromptModePath:
		return true
	default:
		return false
	}
}

func parseEnvPairs(pairs []string) map[string]string {
	if len(pairs) == 0 {
		return nil
	}
	values := make(map[string]string)
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		values[parts[0]] = parts[1]
	}
	return values
}

func parseTimeOrDuration(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("time value is required")
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return time.Now().UTC().Add(duration), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time %q", value)
	}
	return parsed.UTC(), nil
}
