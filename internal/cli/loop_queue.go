package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

var (
	queueAll bool
	queueTo  string
)

func init() {
	rootCmd.AddCommand(loopQueueCmd)

	loopQueueCmd.AddCommand(loopQueueListCmd)
	loopQueueCmd.AddCommand(loopQueueClearCmd)
	loopQueueCmd.AddCommand(loopQueueRemoveCmd)
	loopQueueCmd.AddCommand(loopQueueMoveCmd)

	loopQueueListCmd.Flags().BoolVar(&queueAll, "all", false, "include completed items")
	loopQueueMoveCmd.Flags().StringVar(&queueTo, "to", "front", "move target (front|back)")
}

var loopQueueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage loop queues",
}

var loopQueueListCmd = &cobra.Command{
	Use:   "ls <loop>",
	Short: "List queue items",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		queueRepo := db.NewLoopQueueRepository(database)

		loopEntry, err := resolveLoopByRef(context.Background(), loopRepo, args[0])
		if err != nil {
			return err
		}

		items, err := queueRepo.List(context.Background(), loopEntry.ID)
		if err != nil {
			return err
		}

		filtered := make([]*models.LoopQueueItem, 0)
		for _, item := range items {
			if item.Status != models.LoopQueueStatusPending && !queueAll {
				continue
			}
			filtered = append(filtered, item)
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, filtered)
		}

		if len(filtered) == 0 {
			fmt.Fprintln(os.Stdout, "No queue items")
			return nil
		}

		rows := make([][]string, 0, len(filtered))
		for _, item := range filtered {
			rows = append(rows, []string{
				item.ID,
				string(item.Type),
				string(item.Status),
				fmt.Sprintf("%d", item.Position),
				item.CreatedAt.UTC().Format(time.RFC3339),
			})
		}

		return writeTable(os.Stdout, []string{"ID", "TYPE", "STATUS", "POSITION", "CREATED"}, rows)
	},
}

var loopQueueClearCmd = &cobra.Command{
	Use:   "clear <loop>",
	Short: "Clear pending queue items",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		queueRepo := db.NewLoopQueueRepository(database)

		loopEntry, err := resolveLoopByRef(context.Background(), loopRepo, args[0])
		if err != nil {
			return err
		}

		count, err := queueRepo.Clear(context.Background(), loopEntry.ID)
		if err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{"cleared": count})
		}

		if IsQuiet() {
			return nil
		}

		fmt.Fprintf(os.Stdout, "Cleared %d item(s)\n", count)
		return nil
	},
}

var loopQueueRemoveCmd = &cobra.Command{
	Use:   "rm <loop> <item-id>",
	Short: "Remove a queue item",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		queueRepo := db.NewLoopQueueRepository(database)

		loopEntry, err := resolveLoopByRef(context.Background(), loopRepo, args[0])
		if err != nil {
			return err
		}

		itemID := args[1]
		items, err := queueRepo.List(context.Background(), loopEntry.ID)
		if err != nil {
			return err
		}
		found := false
		for _, item := range items {
			if item.ID == itemID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("queue item not found in loop")
		}

		if err := queueRepo.Remove(context.Background(), itemID); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{"removed": itemID, "loop": loopEntry.Name})
		}

		if IsQuiet() {
			return nil
		}

		fmt.Fprintf(os.Stdout, "Removed item %s\n", itemID)
		return nil
	},
}

var loopQueueMoveCmd = &cobra.Command{
	Use:   "move <loop> <item-id>",
	Short: "Move a queue item",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		queueRepo := db.NewLoopQueueRepository(database)

		loopEntry, err := resolveLoopByRef(context.Background(), loopRepo, args[0])
		if err != nil {
			return err
		}

		items, err := queueRepo.List(context.Background(), loopEntry.ID)
		if err != nil {
			return err
		}

		pending := make([]*models.LoopQueueItem, 0)
		for _, item := range items {
			if item.Status == models.LoopQueueStatusPending {
				pending = append(pending, item)
			}
		}
		if len(pending) == 0 {
			return fmt.Errorf("no pending items")
		}

		itemID := args[1]
		index := -1
		var moving *models.LoopQueueItem
		for i, item := range pending {
			if item.ID == itemID {
				index = i
				moving = item
				break
			}
		}
		if index == -1 {
			return fmt.Errorf("queue item not found")
		}

		target := strings.ToLower(queueTo)
		pending = append(pending[:index], pending[index+1:]...)
		switch target {
		case "front":
			pending = append([]*models.LoopQueueItem{moving}, pending...)
		case "back":
			pending = append(pending, moving)
		default:
			return fmt.Errorf("unknown move target %q", queueTo)
		}
		orderedIDs := make([]string, 0, len(pending))
		for _, item := range pending {
			orderedIDs = append(orderedIDs, item.ID)
		}

		if err := queueRepo.Reorder(context.Background(), loopEntry.ID, orderedIDs); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{"moved": itemID, "to": queueTo})
		}

		if IsQuiet() {
			return nil
		}

		fmt.Fprintf(os.Stdout, "Moved item %s to %s\n", itemID, queueTo)
		return nil
	},
}
