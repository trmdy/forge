// Package cli provides migration CLI commands.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
)

var (
	migrateSteps   int
	migrateVersion int
)

func init() {
	// Add migrate command group
	rootCmd.AddCommand(migrateCmd)

	// Subcommands
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateVersionCmd)

	// Flags
	migrateDownCmd.Flags().IntVarP(&migrateSteps, "steps", "n", 1, "number of migrations to roll back")
	migrateUpCmd.Flags().IntVar(&migrateVersion, "to", 0, "migrate to specific version (0 = latest)")
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Manage database migrations",
	Long: `Manage database schema migrations.

Commands:
  up       Apply pending migrations
  down     Roll back migrations
  status   Show migration status
  version  Show current schema version`,
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply pending migrations",
	Long:  `Apply all pending database migrations, or migrate to a specific version.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabaseNoMigrate()
		if err != nil {
			return err
		}
		defer database.Close()

		if migrateVersion > 0 {
			// Migrate to specific version
			if err := database.MigrateTo(ctx, migrateVersion); err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}
			cmd.Printf("Migrated to version %d\n", migrateVersion)
		} else {
			// Apply all pending
			applied, err := database.MigrateUp(ctx)
			if err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			if applied == 0 {
				cmd.Println("No pending migrations")
			} else {
				cmd.Printf("Applied %d migration(s)\n", applied)
			}
		}

		return nil
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Roll back migrations",
	Long:  `Roll back the last N migrations (default: 1).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabaseNoMigrate()
		if err != nil {
			return err
		}
		defer database.Close()

		rolledBack, err := database.MigrateDown(ctx, migrateSteps)
		if err != nil {
			return fmt.Errorf("rollback failed: %w", err)
		}

		if rolledBack == 0 {
			cmd.Println("No migrations to roll back")
		} else {
			cmd.Printf("Rolled back %d migration(s)\n", rolledBack)
		}

		return nil
	},
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status",
	Long:  `Display the status of all migrations.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabaseNoMigrate()
		if err != nil {
			return err
		}
		defer database.Close()

		status, err := database.MigrationStatus(ctx)
		if err != nil {
			return fmt.Errorf("failed to get migration status: %w", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(status)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "VERSION\tDESCRIPTION\tSTATUS\tAPPLIED AT")
		fmt.Fprintln(w, "-------\t-----------\t------\t----------")

		for _, s := range status {
			statusStr := "pending"
			appliedAt := "-"
			if s.Applied {
				statusStr = "applied"
				appliedAt = s.AppliedAt
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", s.Version, s.Description, statusStr, appliedAt)
		}

		return w.Flush()
	},
}

var migrateVersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show current schema version",
	Long:  `Display the current database schema version.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabaseNoMigrate()
		if err != nil {
			return err
		}
		defer database.Close()

		version, err := database.SchemaVersion(ctx)
		if err != nil {
			return fmt.Errorf("failed to get schema version: %w", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			return enc.Encode(map[string]int{"version": version})
		}

		cmd.Printf("Schema version: %d\n", version)
		return nil
	},
}

// openDatabase opens the database using the current configuration.
func openDatabase() (*db.DB, error) {
	return openDatabaseWithMigration(true)
}

func openDatabaseNoMigrate() (*db.DB, error) {
	return openDatabaseWithMigration(false)
}

func openDatabaseWithMigration(autoMigrate bool) (*db.DB, error) {
	if appConfig == nil {
		return nil, fmt.Errorf("configuration not loaded")
	}

	cfg := db.Config{
		Path:          appConfig.DatabasePath(),
		MaxOpenConns:  10,
		BusyTimeoutMs: 5000,
	}

	database, err := db.Open(cfg)
	if err != nil {
		return nil, err
	}

	if autoMigrate {
		if err := autoMigrateDatabase(database); err != nil {
			_ = database.Close()
			return nil, err
		}
	}

	return database, nil
}

func autoMigrateDatabase(database *db.DB) error {
	if database == nil {
		return fmt.Errorf("database is required")
	}

	ctx := context.Background()
	beforeVersion := 0

	version, err := database.SchemaVersion(ctx)
	if err != nil {
		if !isMissingSchemaTable(err) {
			return fmt.Errorf("failed to read schema version: %w", err)
		}
	} else {
		beforeVersion = version
	}

	applied, err := database.MigrateUp(ctx)
	if err != nil {
		return fmt.Errorf("auto-migrate failed: %w", err)
	}

	if applied > 0 {
		afterVersion := beforeVersion
		if version, err := database.SchemaVersion(ctx); err == nil {
			afterVersion = version
		}
		logger.Info().
			Int("from_version", beforeVersion).
			Int("to_version", afterVersion).
			Msg("database migrated")
	}

	return nil
}
