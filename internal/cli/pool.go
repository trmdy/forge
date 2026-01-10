package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

var (
	poolCreateStrategy string
)

func init() {
	rootCmd.AddCommand(poolCmd)

	poolCmd.AddCommand(poolListCmd)
	poolCmd.AddCommand(poolCreateCmd)
	poolCmd.AddCommand(poolAddCmd)
	poolCmd.AddCommand(poolShowCmd)
	poolCmd.AddCommand(poolSetDefaultCmd)

	poolCreateCmd.Flags().StringVar(&poolCreateStrategy, "strategy", string(models.PoolStrategyRoundRobin), "selection strategy (round_robin)")
}

var poolCmd = &cobra.Command{
	Use:   "pool",
	Short: "Manage profile pools",
}

var poolListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List pools",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewPoolRepository(database)
		pools, err := repo.List(context.Background())
		if err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, pools)
		}

		if len(pools) == 0 {
			fmt.Fprintln(os.Stdout, "No pools found")
			return nil
		}

		rows := make([][]string, 0, len(pools))
		for _, pool := range pools {
			members, _ := repo.ListMembers(context.Background(), pool.ID)
			rows = append(rows, []string{
				pool.Name,
				string(pool.Strategy),
				formatYesNo(pool.IsDefault),
				fmt.Sprintf("%d", len(members)),
			})
		}

		return writeTable(os.Stdout, []string{"NAME", "STRATEGY", "DEFAULT", "MEMBERS"}, rows)
	},
}

var poolCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a pool",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		strategy, err := parsePoolStrategy(poolCreateStrategy)
		if err != nil {
			return err
		}

		pool := &models.Pool{
			Name:     args[0],
			Strategy: strategy,
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewPoolRepository(database)
		if _, err := repo.GetDefault(context.Background()); err != nil {
			if errors.Is(err, db.ErrPoolNotFound) {
				pool.IsDefault = true
			} else {
				return err
			}
		}
		if err := repo.Create(context.Background(), pool); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, pool)
		}

		fmt.Fprintf(os.Stdout, "Pool %q created\n", pool.Name)
		return nil
	},
}

var poolAddCmd = &cobra.Command{
	Use:   "add <pool> <profile...>",
	Short: "Add profiles to a pool",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		poolRef := args[0]
		profileRefs := args[1:]

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		poolRepo := db.NewPoolRepository(database)
		profileRepo := db.NewProfileRepository(database)

		pool, err := resolvePoolByRef(context.Background(), poolRepo, poolRef)
		if err != nil {
			return err
		}

		members, _ := poolRepo.ListMembers(context.Background(), pool.ID)
		position := 0
		for _, member := range members {
			if member.Position > position {
				position = member.Position
			}
		}

		added := make([]string, 0, len(profileRefs))
		for _, ref := range profileRefs {
			profile, err := resolveProfileByRef(context.Background(), profileRepo, ref)
			if err != nil {
				return err
			}
			position++
			member := &models.PoolMember{
				PoolID:    pool.ID,
				ProfileID: profile.ID,
				Position:  position,
			}
			if err := poolRepo.AddMember(context.Background(), member); err != nil {
				return err
			}
			added = append(added, profile.Name)
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{"pool": pool.Name, "added": added})
		}

		fmt.Fprintf(os.Stdout, "Added %s to pool %q\n", strings.Join(added, ", "), pool.Name)
		return nil
	},
}

var poolShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show pool details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		poolRepo := db.NewPoolRepository(database)
		profileRepo := db.NewProfileRepository(database)

		pool, err := resolvePoolByRef(context.Background(), poolRepo, args[0])
		if err != nil {
			return err
		}

		members, err := poolRepo.ListMembers(context.Background(), pool.ID)
		if err != nil {
			return err
		}

		view := poolView{Pool: pool, Members: make([]poolMemberView, 0, len(members))}
		for _, member := range members {
			profile, err := profileRepo.Get(context.Background(), member.ProfileID)
			if err != nil {
				continue
			}
			view.Members = append(view.Members, poolMemberView{ProfileID: profile.ID, ProfileName: profile.Name, Harness: string(profile.Harness), AuthKind: profile.AuthKind})
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, view)
		}

		fmt.Fprintf(os.Stdout, "Pool %s\n", pool.Name)
		fmt.Fprintf(os.Stdout, "Strategy: %s\n", pool.Strategy)
		fmt.Fprintf(os.Stdout, "Default: %s\n\n", formatYesNo(pool.IsDefault))

		if len(view.Members) == 0 {
			fmt.Fprintln(os.Stdout, "No members")
			return nil
		}

		rows := make([][]string, 0, len(view.Members))
		for _, member := range view.Members {
			rows = append(rows, []string{member.ProfileName, member.Harness, member.AuthKind})
		}
		return writeTable(os.Stdout, []string{"PROFILE", "HARNESS", "AUTH_KIND"}, rows)
	},
}

var poolSetDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Set the default pool",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewPoolRepository(database)
		pool, err := resolvePoolByRef(context.Background(), repo, args[0])
		if err != nil {
			return err
		}

		if err := repo.SetDefault(context.Background(), pool.ID); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{"default_pool": pool.Name})
		}

		fmt.Fprintf(os.Stdout, "Default pool set to %q\n", pool.Name)
		return nil
	},
}

type poolMemberView struct {
	ProfileID   string `json:"profile_id"`
	ProfileName string `json:"profile_name"`
	Harness     string `json:"harness"`
	AuthKind    string `json:"auth_kind"`
}

type poolView struct {
	Pool    *models.Pool     `json:"pool"`
	Members []poolMemberView `json:"members"`
}

func resolvePoolByRef(ctx context.Context, repo *db.PoolRepository, ref string) (*models.Pool, error) {
	pool, err := repo.GetByName(ctx, ref)
	if err == nil {
		return pool, nil
	}
	return repo.Get(ctx, ref)
}

func resolveProfileByRef(ctx context.Context, repo *db.ProfileRepository, ref string) (*models.Profile, error) {
	profile, err := repo.GetByName(ctx, ref)
	if err == nil {
		return profile, nil
	}
	return repo.Get(ctx, ref)
}

func parsePoolStrategy(value string) (models.PoolStrategy, error) {
	switch strings.ToLower(value) {
	case "round_robin", "round-robin", "rr":
		return models.PoolStrategyRoundRobin, nil
	default:
		return "", fmt.Errorf("unknown pool strategy %q", value)
	}
}
