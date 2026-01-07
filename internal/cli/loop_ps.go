package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

var (
	loopPsRepo    string
	loopPsPool    string
	loopPsProfile string
	loopPsState   string
	loopPsTag     string
)

func init() {
	rootCmd.AddCommand(loopPsCmd)

	loopPsCmd.Flags().StringVar(&loopPsRepo, "repo", "", "filter by repo path")
	loopPsCmd.Flags().StringVar(&loopPsPool, "pool", "", "filter by pool")
	loopPsCmd.Flags().StringVar(&loopPsProfile, "profile", "", "filter by profile")
	loopPsCmd.Flags().StringVar(&loopPsState, "state", "", "filter by state")
	loopPsCmd.Flags().StringVar(&loopPsTag, "tag", "", "filter by tag")
}

var loopPsCmd = &cobra.Command{
	Use:   "ps",
	Short: "List loops",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		poolRepo := db.NewPoolRepository(database)
		profileRepo := db.NewProfileRepository(database)
		queueRepo := db.NewLoopQueueRepository(database)

		repoPath := loopPsRepo
		if repoPath == "" && chdirPath != "" {
			repoPath = chdirPath
		}
		if repoPath != "" {
			repoPath, err = resolveRepoPath(repoPath)
			if err != nil {
				return err
			}
		}

		selector := loopSelector{
			Repo:    repoPath,
			Pool:    loopPsPool,
			Profile: loopPsProfile,
			State:   loopPsState,
			Tag:     loopPsTag,
		}

		loops, err := selectLoops(context.Background(), loopRepo, poolRepo, profileRepo, selector)
		if err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, loops)
		}

		if len(loops) == 0 {
			fmt.Fprintln(os.Stdout, "No loops found")
			return nil
		}

		sort.Slice(loops, func(i, j int) bool { return loops[i].CreatedAt.Before(loops[j].CreatedAt) })

		rows := make([][]string, 0, len(loops))
		for _, loopEntry := range loops {
			queueItems, _ := queueRepo.List(context.Background(), loopEntry.ID)
			pending := 0
			for _, item := range queueItems {
				if item.Status == models.LoopQueueStatusPending {
					pending++
				}
			}

			lastRun := ""
			if loopEntry.LastRunAt != nil {
				lastRun = loopEntry.LastRunAt.UTC().Format(time.RFC3339)
			}

			waitUntil := ""
			if loopEntry.State == models.LoopStateWaiting && loopEntry.Metadata != nil {
				if value, ok := loopEntry.Metadata["wait_until"]; ok {
					waitUntil = fmt.Sprintf("%v", value)
				}
			}

			rows = append(rows, []string{
				loopEntry.Name,
				string(loopEntry.State),
				waitUntil,
				loopEntry.ProfileID,
				loopEntry.PoolID,
				fmt.Sprintf("%d", pending),
				lastRun,
				loopEntry.RepoPath,
			})
		}

		return writeTable(os.Stdout, []string{"NAME", "STATE", "WAIT_UNTIL", "PROFILE", "POOL", "QUEUE", "LAST_RUN", "REPO"}, rows)
	},
}
