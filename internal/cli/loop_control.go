package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

var (
	loopStopAll     bool
	loopStopRepo    string
	loopStopPool    string
	loopStopProfile string
	loopStopState   string
	loopStopTag     string
)

var (
	loopKillAll     bool
	loopKillRepo    string
	loopKillPool    string
	loopKillProfile string
	loopKillState   string
	loopKillTag     string
)

func init() {
	rootCmd.AddCommand(loopStopCmd)
	rootCmd.AddCommand(loopKillCmd)

	loopStopCmd.Flags().BoolVar(&loopStopAll, "all", false, "stop all loops")
	loopStopCmd.Flags().StringVar(&loopStopRepo, "repo", "", "filter by repo path")
	loopStopCmd.Flags().StringVar(&loopStopPool, "pool", "", "filter by pool")
	loopStopCmd.Flags().StringVar(&loopStopProfile, "profile", "", "filter by profile")
	loopStopCmd.Flags().StringVar(&loopStopState, "state", "", "filter by state")
	loopStopCmd.Flags().StringVar(&loopStopTag, "tag", "", "filter by tag")

	loopKillCmd.Flags().BoolVar(&loopKillAll, "all", false, "kill all loops")
	loopKillCmd.Flags().StringVar(&loopKillRepo, "repo", "", "filter by repo path")
	loopKillCmd.Flags().StringVar(&loopKillPool, "pool", "", "filter by pool")
	loopKillCmd.Flags().StringVar(&loopKillProfile, "profile", "", "filter by profile")
	loopKillCmd.Flags().StringVar(&loopKillState, "state", "", "filter by state")
	loopKillCmd.Flags().StringVar(&loopKillTag, "tag", "", "filter by tag")
}

var loopStopCmd = &cobra.Command{
	Use:   "stop [loop]",
	Short: "Stop loops after current iteration",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sel := loopSelector{Repo: loopStopRepo, Pool: loopStopPool, Profile: loopStopProfile, State: loopStopState, Tag: loopStopTag}
		if len(args) > 0 {
			sel.LoopRef = args[0]
		}
		if sel.LoopRef == "" && !loopStopAll && sel.Repo == "" && sel.Pool == "" && sel.Profile == "" && sel.State == "" && sel.Tag == "" {
			return fmt.Errorf("specify a loop or selector")
		}

		return enqueueLoopControl(sel, models.LoopQueueItemStopGraceful)
	},
}

var loopKillCmd = &cobra.Command{
	Use:   "kill [loop]",
	Short: "Kill loops immediately",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sel := loopSelector{Repo: loopKillRepo, Pool: loopKillPool, Profile: loopKillProfile, State: loopKillState, Tag: loopKillTag}
		if len(args) > 0 {
			sel.LoopRef = args[0]
		}
		if sel.LoopRef == "" && !loopKillAll && sel.Repo == "" && sel.Pool == "" && sel.Profile == "" && sel.State == "" && sel.Tag == "" {
			return fmt.Errorf("specify a loop or selector")
		}

		return enqueueLoopControl(sel, models.LoopQueueItemKillNow)
	},
}

func enqueueLoopControl(selector loopSelector, itemType models.LoopQueueItemType) error {
	database, err := openDatabase()
	if err != nil {
		return err
	}
	defer database.Close()

	loopRepo := db.NewLoopRepository(database)
	poolRepo := db.NewPoolRepository(database)
	profileRepo := db.NewProfileRepository(database)
	queueRepo := db.NewLoopQueueRepository(database)

	loops, err := selectLoops(context.Background(), loopRepo, poolRepo, profileRepo, selector)
	if err != nil {
		return err
	}

	if len(loops) == 0 {
		return fmt.Errorf("no loops matched")
	}

	for _, loopEntry := range loops {
		payload, err := controlPayload(itemType)
		if err != nil {
			return err
		}
		item := &models.LoopQueueItem{Type: itemType, Payload: payload}
		if err := queueRepo.Enqueue(context.Background(), loopEntry.ID, item); err != nil {
			return err
		}

		if itemType == models.LoopQueueItemKillNow {
			_ = killLoopProcess(loopEntry)
			loopEntry.State = models.LoopStateStopped
			_ = loopRepo.Update(context.Background(), loopEntry)
		}
	}

	if IsJSONOutput() || IsJSONLOutput() {
		return WriteOutput(os.Stdout, map[string]any{"loops": len(loops), "action": string(itemType)})
	}

	if IsQuiet() {
		return nil
	}

	verb := "Stopped"
	if itemType == models.LoopQueueItemKillNow {
		verb = "Killed"
	}
	fmt.Fprintf(os.Stdout, "%s %d loop(s)\n", verb, len(loops))
	return nil
}

func controlPayload(itemType models.LoopQueueItemType) ([]byte, error) {
	switch itemType {
	case models.LoopQueueItemStopGraceful:
		return json.Marshal(models.StopPayload{Reason: "operator"})
	case models.LoopQueueItemKillNow:
		return json.Marshal(models.KillPayload{Reason: "operator"})
	default:
		return nil, fmt.Errorf("unsupported control item %q", itemType)
	}
}

func killLoopProcess(loopEntry *models.Loop) error {
	pid, ok := loopPID(loopEntry)
	if !ok {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if err := process.Signal(syscall.SIGKILL); err != nil {
		// fallback to process.Kill() on non-Unix
		_ = process.Kill()
	}
	return nil
}

func loopPID(loopEntry *models.Loop) (int, bool) {
	if loopEntry == nil || loopEntry.Metadata == nil {
		return 0, false
	}
	value, ok := loopEntry.Metadata["pid"]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case string:
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
