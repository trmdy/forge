// Package cli provides TUI launch commands.
package cli

import (
	"os"

	"github.com/opencode-ai/swarm/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func init() {
	rootCmd.AddCommand(uiCmd)
}

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch the Swarm TUI",
	Long:  "Launch the Swarm terminal user interface (TUI).",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

func runTUI() error {
	if IsNonInteractive() {
		return &PreflightError{
			Message:  "TUI requires an interactive terminal",
			Hint:     "Run without --non-interactive and with a TTY, or use CLI subcommands",
			NextStep: "swarm --help",
		}
	}

	return tui.Run()
}

func hasTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
