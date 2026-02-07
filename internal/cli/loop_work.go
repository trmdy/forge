package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

var (
	workLoopRef string
	workAgentID string
	workStatus  string
	workDetail  string
)

func init() {
	rootCmd.AddCommand(workCmd)
	workCmd.Flags().StringVar(&workLoopRef, "loop", "", "loop ref (defaults to FORGE_LOOP_ID)")
	workCmd.Flags().StringVar(&workAgentID, "agent", "", "agent id (defaults to FMAIL_AGENT/SV_ACTOR/FORGE_LOOP_NAME)")

	workCmd.AddCommand(workSetCmd)
	workCmd.AddCommand(workClearCmd)
	workCmd.AddCommand(workCurrentCmd)
	workCmd.AddCommand(workListCmd)

	workSetCmd.Flags().StringVar(&workStatus, "status", "in_progress", "opaque status string (suggested: todo|in_progress|blocked|done)")
	workSetCmd.Flags().StringVar(&workDetail, "detail", "", "detail (optional)")
}

var workCmd = &cobra.Command{
	Use:   "work",
	Short: "Persist loop work context (task id + status)",
}

var workSetCmd = &cobra.Command{
	Use:   "set <task-id>",
	Short: "Set current task id/status for a loop",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		loopRef, err := requireLoopRef(workLoopRef)
		if err != nil {
			return err
		}
		agentID, err := resolveAgentID(workAgentID)
		if err != nil {
			return err
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		loopEntry, err := resolveLoopByRef(context.Background(), loopRepo, loopRef)
		if err != nil {
			return err
		}

		iter := loopIterationFromMetadata(loopEntry.Metadata)

		repo := db.NewLoopWorkStateRepository(database)
		state := &models.LoopWorkState{
			LoopID:        loopEntry.ID,
			AgentID:       agentID,
			TaskID:        args[0],
			Status:        strings.TrimSpace(workStatus),
			Detail:        workDetail,
			LoopIteration: iter,
			IsCurrent:     true,
		}
		if err := repo.SetCurrent(context.Background(), state); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, state)
		}
		if IsQuiet() {
			return nil
		}
		fmt.Fprintln(os.Stdout, "ok")
		return nil
	},
}

var workClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear current task pointer for a loop",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		loopRef, err := requireLoopRef(workLoopRef)
		if err != nil {
			return err
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		loopEntry, err := resolveLoopByRef(context.Background(), loopRepo, loopRef)
		if err != nil {
			return err
		}

		repo := db.NewLoopWorkStateRepository(database)
		if err := repo.ClearCurrent(context.Background(), loopEntry.ID); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{"loop": loopEntry.Name, "ok": true})
		}
		if IsQuiet() {
			return nil
		}
		fmt.Fprintln(os.Stdout, "ok")
		return nil
	},
}

var workCurrentCmd = &cobra.Command{
	Use:     "current",
	Aliases: []string{"status", "show"},
	Short:   "Show current task pointer for a loop",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		loopRef, err := requireLoopRef(workLoopRef)
		if err != nil {
			return err
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		loopEntry, err := resolveLoopByRef(context.Background(), loopRepo, loopRef)
		if err != nil {
			return err
		}

		repo := db.NewLoopWorkStateRepository(database)
		cur, err := repo.GetCurrent(context.Background(), loopEntry.ID)
		if err != nil {
			if errors.Is(err, db.ErrLoopWorkStateNotFound) {
				if IsJSONOutput() || IsJSONLOutput() {
					return WriteOutput(os.Stdout, map[string]any{"current": nil})
				}
				fmt.Fprintln(os.Stdout, "(none)")
				return nil
			}
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, cur)
		}
		line := fmt.Sprintf("%s [%s] agent=%s iter=%d updated=%s", cur.TaskID, cur.Status, cur.AgentID, cur.LoopIteration, cur.UpdatedAt.UTC().Format(time.RFC3339))
		if strings.TrimSpace(cur.Detail) != "" {
			line += " | " + strings.TrimSpace(cur.Detail)
		}
		fmt.Fprintln(os.Stdout, line)
		return nil
	},
}

var workListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List recent work context updates for a loop",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		loopRef, err := requireLoopRef(workLoopRef)
		if err != nil {
			return err
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		loopEntry, err := resolveLoopByRef(context.Background(), loopRepo, loopRef)
		if err != nil {
			return err
		}

		repo := db.NewLoopWorkStateRepository(database)
		items, err := repo.ListByLoop(context.Background(), loopEntry.ID, 100)
		if err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, items)
		}
		if len(items) == 0 {
			fmt.Fprintln(os.Stdout, "(empty)")
			return nil
		}

		sort.Slice(items, func(i, j int) bool {
			if items[i].IsCurrent != items[j].IsCurrent {
				return items[i].IsCurrent
			}
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		})

		for _, it := range items {
			prefix := " "
			if it.IsCurrent {
				prefix = "*"
			}
			line := fmt.Sprintf("%s %s [%s] agent=%s iter=%d updated=%s", prefix, it.TaskID, it.Status, it.AgentID, it.LoopIteration, it.UpdatedAt.UTC().Format(time.RFC3339))
			if strings.TrimSpace(it.Detail) != "" {
				line += " | " + strings.TrimSpace(it.Detail)
			}
			fmt.Fprintln(os.Stdout, line)
		}
		return nil
	},
}

func resolveAgentID(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit), nil
	}
	if v := strings.TrimSpace(os.Getenv("FMAIL_AGENT")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(os.Getenv("SV_ACTOR")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(os.Getenv("FORGE_LOOP_NAME")); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("agent id required (pass --agent or set FMAIL_AGENT)")
}

func loopIterationFromMetadata(metadata map[string]any) int {
	if metadata == nil {
		return 0
	}
	v, ok := metadata["iteration_count"]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, err := strconv.Atoi(x)
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}
