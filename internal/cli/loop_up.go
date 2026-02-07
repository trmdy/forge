package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/loop"
	"github.com/tOgg1/forge/internal/models"
)

var (
	loopUpCount         int
	loopUpName          string
	loopUpNamePrefix    string
	loopUpPool          string
	loopUpProfile       string
	loopUpPrompt        string
	loopUpPromptMsg     string
	loopUpInterval      string
	loopUpMaxRuntime    string
	loopUpMaxIterations int
	loopUpTags          string

	startLoopProcessFunc = startLoopProcess
)

func init() {
	rootCmd.AddCommand(loopUpCmd)

	loopUpCmd.Flags().IntVarP(&loopUpCount, "count", "n", 1, "number of loops to start")
	loopUpCmd.Flags().StringVar(&loopUpName, "name", "", "loop name (single loop)")
	loopUpCmd.Flags().StringVar(&loopUpNamePrefix, "name-prefix", "", "loop name prefix")
	loopUpCmd.Flags().StringVar(&loopUpPool, "pool", "", "pool name or ID")
	loopUpCmd.Flags().StringVar(&loopUpProfile, "profile", "", "profile name or ID")
	loopUpCmd.Flags().StringVar(&loopUpPrompt, "prompt", "", "base prompt path or prompt name")
	loopUpCmd.Flags().StringVar(&loopUpPromptMsg, "prompt-msg", "", "base prompt content for each iteration")
	loopUpCmd.Flags().StringVar(&loopUpInterval, "interval", "", "sleep interval (e.g., 30s, 2m)")
	loopUpCmd.Flags().StringVarP(&loopUpMaxRuntime, "max-runtime", "r", "", "max runtime before stopping (e.g., 30m, 2h)")
	loopUpCmd.Flags().IntVarP(&loopUpMaxIterations, "max-iterations", "i", 0, "max iterations before stopping (> 0 required)")
	loopUpCmd.Flags().StringVar(&loopUpTags, "tags", "", "comma-separated tags")
}

var loopUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start loop(s) for a repo",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if loopUpCount < 1 {
			return fmt.Errorf("--count must be at least 1")
		}
		if loopUpName != "" && loopUpCount > 1 {
			return fmt.Errorf("--name requires --count=1")
		}
		if loopUpPool != "" && loopUpProfile != "" {
			return fmt.Errorf("use either --pool or --profile, not both")
		}

		repoPath, err := resolveRepoPath("")
		if err != nil {
			return err
		}

		cfg := GetConfig()
		interval, err := parseDuration(loopUpInterval, cfg.LoopDefaults.Interval)
		if err != nil {
			return err
		}
		if interval < 0 {
			return fmt.Errorf("interval must be >= 0")
		}
		if loopUpMaxIterations < 0 {
			return fmt.Errorf("max iterations must be >= 0")
		}
		maxRuntime, err := parseDuration(loopUpMaxRuntime, 0)
		if err != nil {
			return err
		}
		if maxRuntime < 0 {
			return fmt.Errorf("max runtime must be >= 0")
		}
		if loopUpMaxIterations == 0 || maxRuntime == 0 {
			return fmt.Errorf("max iterations and max runtime must be > 0 to create loops")
		}

		basePromptMsg := strings.TrimSpace(loopUpPromptMsg)
		if basePromptMsg == "" {
			basePromptMsg = strings.TrimSpace(cfg.LoopDefaults.PromptMsg)
		}

		basePromptPath := ""
		if loopUpPrompt != "" {
			resolved, _, err := resolvePromptPath(repoPath, loopUpPrompt)
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

		tags := parseTags(loopUpTags)

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		poolRepo := db.NewPoolRepository(database)
		profileRepo := db.NewProfileRepository(database)

		var poolID string
		if loopUpPool != "" {
			pool, err := resolvePoolByRef(context.Background(), poolRepo, loopUpPool)
			if err != nil {
				return err
			}
			poolID = pool.ID
		}

		var profileID string
		if loopUpProfile != "" {
			profile, err := resolveProfileByRef(context.Background(), profileRepo, loopUpProfile)
			if err != nil {
				return err
			}
			profileID = profile.ID
		}

		existing, err := loopRepo.List(context.Background())
		if err != nil {
			return err
		}
		existingNames := make(map[string]struct{}, len(existing))
		for _, item := range existing {
			existingNames[item.Name] = struct{}{}
		}

		created := make([]*models.Loop, 0, loopUpCount)
		for i := 0; i < loopUpCount; i++ {
			name := loopUpName
			if name == "" {
				if loopUpNamePrefix != "" {
					name = fmt.Sprintf("%s-%d", loopUpNamePrefix, i+1)
				} else {
					name = generateLoopName(existingNames)
				}
			}
			if _, exists := existingNames[name]; exists {
				return fmt.Errorf("loop name %q already exists", name)
			}
			existingNames[name] = struct{}{}

			loopEntry := &models.Loop{
				Name:              name,
				RepoPath:          repoPath,
				BasePromptPath:    basePromptPath,
				BasePromptMsg:     basePromptMsg,
				IntervalSeconds:   int(interval.Round(time.Second).Seconds()),
				MaxIterations:     loopUpMaxIterations,
				MaxRuntimeSeconds: int(maxRuntime.Round(time.Second).Seconds()),
				PoolID:            poolID,
				ProfileID:         profileID,
				Tags:              tags,
				State:             models.LoopStateStopped,
			}
			if err := loopRepo.Create(context.Background(), loopEntry); err != nil {
				return err
			}

			loopEntry.LogPath = loop.LogPath(cfg.Global.DataDir, loopEntry.Name, loopEntry.ID)
			loopEntry.LedgerPath = loop.LedgerPath(repoPath, loopEntry.Name, loopEntry.ID)
			if err := loopRepo.Update(context.Background(), loopEntry); err != nil {
				return err
			}

			if err := startLoopProcessFunc(loopEntry.ID); err != nil {
				return err
			}

			created = append(created, loopEntry)
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, created)
		}

		if IsQuiet() {
			return nil
		}

		for _, loopEntry := range created {
			fmt.Fprintf(os.Stdout, "Loop %q started (%s)\n", loopEntry.Name, loopShortID(loopEntry))
		}

		return nil
	},
}

func startLoopProcess(loopID string) error {
	args := []string{"loop", "run", loopID}
	if cfgFile != "" {
		args = append([]string{"--config", cfgFile}, args...)
	}

	cmd := exec.Command(os.Args[0], args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start loop process: %w", err)
	}

	return nil
}
