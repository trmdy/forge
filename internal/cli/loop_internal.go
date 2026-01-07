package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/loop"
)

func init() {
	rootCmd.AddCommand(loopInternalCmd)
	loopInternalCmd.AddCommand(loopRunCmd)
}

var loopInternalCmd = &cobra.Command{
	Use:    "loop",
	Hidden: true,
}

var loopRunCmd = &cobra.Command{
	Use:    "run <loop-id>",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		runner := loop.NewRunner(database, GetConfig())

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if err := runner.RunLoop(ctx, args[0]); err != nil {
			return fmt.Errorf("loop run failed: %w", err)
		}

		return nil
	},
}
