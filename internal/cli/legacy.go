package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var legacyCommandsEnabled = false

func addLegacyCommand(cmd *cobra.Command) {
	if legacyCommandsEnabled {
		rootCmd.AddCommand(cmd)
		return
	}

	cmd.Hidden = true
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%s is disabled in loop mode", cmd.CommandPath())
	}
	if cmd.RunE == nil {
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s is disabled in loop mode", cmd.CommandPath())
		}
	}

	rootCmd.AddCommand(cmd)
}
