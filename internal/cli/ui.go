// Package cli provides TUI launch commands.
package cli

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/looptui"
	"golang.org/x/term"
)

func init() {
	rootCmd.AddCommand(uiCmd)
}

var uiCmd = &cobra.Command{
	Use:     "tui",
	Aliases: []string{"ui"},
	Short:   "Launch the Forge TUI",
	Long:    "Launch the Forge terminal user interface (TUI).",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

func runTUI() error {
	if IsNonInteractive() {
		return &PreflightError{
			Message:  "TUI requires an interactive terminal",
			Hint:     "Run without --non-interactive and with a TTY, or use CLI subcommands",
			NextStep: "forge --help",
		}
	}

	// Open database for loop status
	database, err := openDatabase()
	if err != nil {
		return err
	}
	defer database.Close()

	loopConfig := looptui.Config{}
	if cfg := GetConfig(); cfg != nil {
		loopConfig.DataDir = cfg.Global.DataDir
		loopConfig.RefreshInterval = cfg.TUI.RefreshInterval
	}

	return looptui.Run(database, loopConfig)
}

func hasTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// getEnvWithFallback returns the value of the primary env var, or falls back to legacy.
func getEnvWithFallback(primary, legacy string) string {
	if value := strings.TrimSpace(os.Getenv(primary)); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv(legacy))
}

func parseEnvDuration(value string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed, true
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second, true
	}
	return 0, false
}
