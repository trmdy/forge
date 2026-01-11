package fmail

import "github.com/spf13/cobra"

func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "fmail",
		Short:         "Agent-to-agent messaging via .fmail files",
		Long:          "fmail sends and receives messages via .fmail/ files.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(
		newSendCmd(),
		newLogCmd(),
		newWatchCmd(),
		newWhoCmd(),
		newStatusCmd(),
		newTopicsCmd(),
		newGCCmd(),
		newInitCmd(),
	)

	return cmd
}
