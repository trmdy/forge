package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/loop"
	"github.com/tOgg1/forge/internal/models"
)

var (
	loopScaleCount      int
	loopScalePool       string
	loopScaleProfile    string
	loopScalePrompt     string
	loopScalePromptMsg  string
	loopScaleInterval   string
	loopScaleTags       string
	loopScaleNamePrefix string
	loopScaleKill       bool
)

func init() {
	rootCmd.AddCommand(loopScaleCmd)

	loopScaleCmd.Flags().IntVarP(&loopScaleCount, "count", "n", 1, "target loop count")
	loopScaleCmd.Flags().StringVar(&loopScalePool, "pool", "", "pool name or ID")
	loopScaleCmd.Flags().StringVar(&loopScaleProfile, "profile", "", "profile name or ID")
	loopScaleCmd.Flags().StringVar(&loopScalePrompt, "prompt", "", "base prompt path or name")
	loopScaleCmd.Flags().StringVar(&loopScalePromptMsg, "prompt-msg", "", "base prompt content for each iteration")
	loopScaleCmd.Flags().StringVar(&loopScaleInterval, "interval", "", "sleep interval")
	loopScaleCmd.Flags().StringVar(&loopScaleTags, "tags", "", "comma-separated tags")
	loopScaleCmd.Flags().StringVar(&loopScaleNamePrefix, "name-prefix", "", "name prefix for new loops")
	loopScaleCmd.Flags().BoolVar(&loopScaleKill, "kill", false, "kill extra loops instead of stopping")
}

var loopScaleCmd = &cobra.Command{
	Use:   "scale",
	Short: "Scale loops to a target count",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if loopScaleCount < 0 {
			return fmt.Errorf("--count must be >= 0")
		}
		if loopScalePool != "" && loopScaleProfile != "" {
			return fmt.Errorf("use either --pool or --profile, not both")
		}

		repoPath, err := resolveRepoPath("")
		if err != nil {
			return err
		}

		cfg := GetConfig()
		interval, err := parseDuration(loopScaleInterval, cfg.LoopDefaults.Interval)
		if err != nil {
			return err
		}

		basePromptMsg := loopScalePromptMsg
		if basePromptMsg == "" {
			basePromptMsg = cfg.LoopDefaults.PromptMsg
		}

		basePromptPath := ""
		if loopScalePrompt != "" {
			resolved, _, err := resolvePromptPath(repoPath, loopScalePrompt)
			if err != nil {
				return err
			}
			basePromptPath = resolved
		} else if cfg.LoopDefaults.Prompt != "" {
			resolved, _, err := resolvePromptPath(repoPath, cfg.LoopDefaults.Prompt)
			if err != nil {
				return err
			}
			basePromptPath = resolved
		}

		tags := parseTags(loopScaleTags)

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		poolRepo := db.NewPoolRepository(database)
		profileRepo := db.NewProfileRepository(database)
		queueRepo := db.NewLoopQueueRepository(database)

		var poolID string
		if loopScalePool != "" {
			pool, err := resolvePoolByRef(context.Background(), poolRepo, loopScalePool)
			if err != nil {
				return err
			}
			poolID = pool.ID
		}

		var profileID string
		if loopScaleProfile != "" {
			profile, err := resolveProfileByRef(context.Background(), profileRepo, loopScaleProfile)
			if err != nil {
				return err
			}
			profileID = profile.ID
		}

		selector := loopSelector{Repo: repoPath, Pool: loopScalePool, Profile: loopScaleProfile}
		loops, err := selectLoops(context.Background(), loopRepo, poolRepo, profileRepo, selector)
		if err != nil {
			return err
		}

		sort.Slice(loops, func(i, j int) bool { return loops[i].CreatedAt.Before(loops[j].CreatedAt) })

		if len(loops) > loopScaleCount {
			extra := loops[loopScaleCount:]
			itemType := models.LoopQueueItemStopGraceful
			if loopScaleKill {
				itemType = models.LoopQueueItemKillNow
			}
			for _, loopEntry := range extra {
				payload, _ := controlPayload(itemType)
				item := &models.LoopQueueItem{Type: itemType, Payload: payload}
				if err := queueRepo.Enqueue(context.Background(), loopEntry.ID, item); err != nil {
					return err
				}
			}
		}

		if len(loops) < loopScaleCount {
			toCreate := loopScaleCount - len(loops)
			existingNames := make(map[string]struct{}, len(loops))
			for _, entry := range loops {
				existingNames[entry.Name] = struct{}{}
			}
			for i := 0; i < toCreate; i++ {
				name := generateLoopName(existingNames)
				if loopScaleNamePrefix != "" {
					name = fmt.Sprintf("%s-%d", loopScaleNamePrefix, i+1)
				}
				if _, exists := existingNames[name]; exists {
					return fmt.Errorf("loop name %q already exists", name)
				}
				existingNames[name] = struct{}{}

				loopEntry := &models.Loop{
					Name:            name,
					RepoPath:        repoPath,
					BasePromptPath:  basePromptPath,
					BasePromptMsg:   basePromptMsg,
					IntervalSeconds: int(interval.Round(time.Second).Seconds()),
					PoolID:          poolID,
					ProfileID:       profileID,
					Tags:            tags,
					State:           models.LoopStateStopped,
				}
				if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
					return err
				}

				loopEntry.LogPath = loop.LogPath(cfg.Global.DataDir, loopEntry.Name, loopEntry.ID)
				loopEntry.LedgerPath = loop.LedgerPath(repoPath, loopEntry.Name, loopEntry.ID)
				if err := loopRepo.Update(context.Background(), loopEntry); err != nil {
					return err
				}

				if err := startLoopProcess(loopEntry.ID); err != nil {
					return err
				}
			}
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{"target": loopScaleCount, "current": len(loops)})
		}

		if IsQuiet() {
			return nil
		}

		fmt.Fprintf(os.Stdout, "Scaled loops to %d\n", loopScaleCount)
		return nil
	},
}
