package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/loop"
)

func init() {
	rootCmd.AddCommand(loopRunOnceCmd)
}

var loopRunOnceCmd = &cobra.Command{
	Use:   "run <loop>",
	Short: "Run a single loop iteration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		loopEntry, err := resolveLoopByRef(context.Background(), loopRepo, args[0])
		if err != nil {
			return err
		}

		runner := loop.NewRunner(database, GetConfig())
		if err := runner.RunOnce(context.Background(), loopEntry.ID); err != nil {
			return fmt.Errorf("loop run failed: %w", err)
		}

		return nil
	},
}
