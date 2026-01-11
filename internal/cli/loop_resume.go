package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

func init() {
	rootCmd.AddCommand(loopResumeCmd)
}

var loopResumeCmd = &cobra.Command{
	Use:   "resume <loop>",
	Short: "Resume a stopped loop",
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

		switch loopEntry.State {
		case models.LoopStateStopped, models.LoopStateError:
		default:
			return fmt.Errorf("loop %q is %s; only stopped or errored loops can be resumed", loopEntry.Name, loopEntry.State)
		}

		if err := startLoopProcess(loopEntry.ID); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{
				"resumed": true,
				"loop_id": loopEntry.ID,
				"name":    loopEntry.Name,
			})
		}

		if IsQuiet() {
			return nil
		}

		fmt.Fprintf(os.Stdout, "Loop %q resumed (%s)\n", loopEntry.Name, loopShortID(loopEntry))
		return nil
	},
}
