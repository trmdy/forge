// Package cli provides the doctor command for environment diagnostics.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/config"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/node"
)

// DoctorCheckStatus indicates the result of a diagnostic check.
type DoctorCheckStatus string

const (
	DoctorPass DoctorCheckStatus = "pass"
	DoctorWarn DoctorCheckStatus = "warn"
	DoctorFail DoctorCheckStatus = "fail"
	DoctorSkip DoctorCheckStatus = "skip"
)

// DoctorCheck represents a single diagnostic result.
type DoctorCheck struct {
	Category string            `json:"category"`
	Name     string            `json:"name"`
	Status   DoctorCheckStatus `json:"status"`
	Details  string            `json:"details,omitempty"`
	Error    string            `json:"error,omitempty"`
}

// DoctorReport aggregates diagnostic results.
type DoctorReport struct {
	Checks    []DoctorCheck `json:"checks"`
	Summary   DoctorSummary `json:"summary"`
	CheckedAt time.Time     `json:"checked_at"`
}

// DoctorSummary provides a quick overview.
type DoctorSummary struct {
	Total    int `json:"total"`
	Passed   int `json:"passed"`
	Warnings int `json:"warnings"`
	Failed   int `json:"failed"`
	Skipped  int `json:"skipped"`
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run environment diagnostics",
	Long: `Run comprehensive diagnostics on your Forge environment.

Checks include:
- Dependencies: tmux, opencode, ssh, git
- Configuration: config file, database, migrations
- Nodes: connectivity and health
- Accounts: vault access and profiles`,
	Example: `  forge doctor
  forge doctor --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		checks := make([]DoctorCheck, 0)

		// Dependency checks
		checks = append(checks, checkDependencies()...)

		// Configuration checks
		checks = append(checks, checkConfiguration()...)

		// Database checks
		dbChecks, database := checkDatabaseHealth()
		checks = append(checks, dbChecks...)

		// Node checks (only if DB is available)
		if database != nil {
			checks = append(checks, checkNodes(ctx, database)...)
			database.Close()
		}

		// Build summary
		summary := buildSummary(checks)

		report := &DoctorReport{
			Checks:    checks,
			Summary:   summary,
			CheckedAt: time.Now().UTC(),
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, report)
		}

		// Pretty print
		printDoctorReport(report)

		// Exit with error if any checks failed
		if summary.Failed > 0 {
			os.Exit(1)
		}

		return nil
	},
}

func checkDependencies() []DoctorCheck {
	checks := make([]DoctorCheck, 0)

	// tmux
	checks = append(checks, checkBinary("dependencies", "tmux", "tmux -V", func(output string) (DoctorCheckStatus, string) {
		output = strings.TrimSpace(output)
		if strings.HasPrefix(output, "tmux ") {
			version := strings.TrimPrefix(output, "tmux ")
			// Check version >= 3.0
			if version < "3.0" {
				return DoctorWarn, fmt.Sprintf("version %s (3.0+ recommended)", version)
			}
			return DoctorPass, version
		}
		return DoctorWarn, output
	}))

	// opencode
	checks = append(checks, checkBinary("dependencies", "opencode", "opencode --version 2>/dev/null || opencode version 2>/dev/null", func(output string) (DoctorCheckStatus, string) {
		output = strings.TrimSpace(output)
		if output == "" {
			return DoctorFail, "not found in PATH"
		}
		// Try to extract version
		lines := strings.Split(output, "\n")
		if len(lines) > 0 {
			return DoctorPass, strings.TrimSpace(lines[0])
		}
		return DoctorPass, "installed"
	}))

	// git
	checks = append(checks, checkBinary("dependencies", "git", "git --version", func(output string) (DoctorCheckStatus, string) {
		output = strings.TrimSpace(output)
		if strings.HasPrefix(output, "git version ") {
			version := strings.TrimPrefix(output, "git version ")
			return DoctorPass, version
		}
		return DoctorWarn, output
	}))

	// ssh
	checks = append(checks, checkBinary("dependencies", "ssh", "ssh -V 2>&1", func(output string) (DoctorCheckStatus, string) {
		output = strings.TrimSpace(output)
		if strings.Contains(output, "OpenSSH") || strings.Contains(output, "SSH") {
			return DoctorPass, output
		}
		return DoctorWarn, output
	}))

	return checks
}

func checkBinary(category, name, cmd string, parser func(string) (DoctorCheckStatus, string)) DoctorCheck {
	check := DoctorCheck{
		Category: category,
		Name:     name,
	}

	output, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		// Try to check if binary exists at all
		_, lookErr := exec.LookPath(name)
		if lookErr != nil {
			check.Status = DoctorFail
			check.Error = "not found in PATH"
			return check
		}
		check.Status = DoctorWarn
		check.Error = fmt.Sprintf("command failed: %v", err)
		return check
	}

	check.Status, check.Details = parser(string(output))
	return check
}

func checkConfiguration() []DoctorCheck {
	checks := make([]DoctorCheck, 0)

	// Config file
	home, err := os.UserHomeDir()
	if err != nil {
		checks = append(checks, DoctorCheck{
			Category: "config",
			Name:     "home_directory",
			Status:   DoctorFail,
			Error:    err.Error(),
		})
		return checks
	}

	configPath := filepath.Join(home, ".config", "forge", "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		checks = append(checks, DoctorCheck{
			Category: "config",
			Name:     "config_file",
			Status:   DoctorPass,
			Details:  configPath,
		})
	} else if os.IsNotExist(err) {
		checks = append(checks, DoctorCheck{
			Category: "config",
			Name:     "config_file",
			Status:   DoctorWarn,
			Details:  "not found (using defaults)",
		})
	} else {
		checks = append(checks, DoctorCheck{
			Category: "config",
			Name:     "config_file",
			Status:   DoctorFail,
			Error:    err.Error(),
		})
	}

	// Data directory
	dataDir := filepath.Join(home, ".local", "share", "forge")
	if info, err := os.Stat(dataDir); err == nil && info.IsDir() {
		checks = append(checks, DoctorCheck{
			Category: "config",
			Name:     "data_directory",
			Status:   DoctorPass,
			Details:  dataDir,
		})
	} else if os.IsNotExist(err) {
		// Try to create it
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			checks = append(checks, DoctorCheck{
				Category: "config",
				Name:     "data_directory",
				Status:   DoctorFail,
				Error:    fmt.Sprintf("cannot create: %v", err),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Category: "config",
				Name:     "data_directory",
				Status:   DoctorPass,
				Details:  fmt.Sprintf("%s (created)", dataDir),
			})
		}
	} else {
		checks = append(checks, DoctorCheck{
			Category: "config",
			Name:     "data_directory",
			Status:   DoctorFail,
			Error:    err.Error(),
		})
	}

	return checks
}

func checkDatabaseHealth() ([]DoctorCheck, *db.DB) {
	checks := make([]DoctorCheck, 0)

	// Try to open database
	cfg, err := config.LoadDefault()
	var dbPath string
	if err != nil {
		checks = append(checks, DoctorCheck{
			Category: "database",
			Name:     "config_load",
			Status:   DoctorWarn,
			Details:  "using default config",
		})
		// Fall back to default path
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".local", "share", "forge", "forge.db")
	} else {
		dbPath = cfg.Database.Path
	}
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".local", "share", "forge", "forge.db")
	}

	database, err := db.Open(db.Config{Path: dbPath})
	if err != nil {
		checks = append(checks, DoctorCheck{
			Category: "database",
			Name:     "connection",
			Status:   DoctorFail,
			Error:    err.Error(),
		})
		return checks, nil
	}

	checks = append(checks, DoctorCheck{
		Category: "database",
		Name:     "connection",
		Status:   DoctorPass,
		Details:  dbPath,
	})

	// Check migrations
	ctx := context.Background()
	migrations, err := database.MigrationStatus(ctx)
	if err != nil {
		checks = append(checks, DoctorCheck{
			Category: "database",
			Name:     "migrations",
			Status:   DoctorWarn,
			Error:    err.Error(),
		})
	} else {
		// Count applied and pending migrations
		applied := 0
		pending := 0
		for _, m := range migrations {
			if m.Applied {
				applied++
			} else {
				pending++
			}
		}
		if pending > 0 {
			checks = append(checks, DoctorCheck{
				Category: "database",
				Name:     "migrations",
				Status:   DoctorWarn,
				Details:  fmt.Sprintf("%d pending (run 'forge migrate up')", pending),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Category: "database",
				Name:     "migrations",
				Status:   DoctorPass,
				Details:  fmt.Sprintf("%d applied", applied),
			})
		}
	}

	return checks, database
}

func checkNodes(ctx context.Context, database *db.DB) []DoctorCheck {
	checks := make([]DoctorCheck, 0)

	nodeRepo := db.NewNodeRepository(database)
	nodeService := node.NewService(nodeRepo, node.WithPublisher(newEventPublisher(database)))

	nodes, err := nodeRepo.List(ctx, nil)
	if err != nil {
		checks = append(checks, DoctorCheck{
			Category: "nodes",
			Name:     "list",
			Status:   DoctorFail,
			Error:    err.Error(),
		})
		return checks
	}

	if len(nodes) == 0 {
		checks = append(checks, DoctorCheck{
			Category: "nodes",
			Name:     "count",
			Status:   DoctorWarn,
			Details:  "no nodes configured",
		})
		return checks
	}

	checks = append(checks, DoctorCheck{
		Category: "nodes",
		Name:     "count",
		Status:   DoctorPass,
		Details:  fmt.Sprintf("%d node(s)", len(nodes)),
	})

	// Check each node (but limit to avoid timeout)
	for _, n := range nodes {
		if len(checks) > 20 {
			checks = append(checks, DoctorCheck{
				Category: "nodes",
				Name:     "truncated",
				Status:   DoctorSkip,
				Details:  "too many nodes, remaining skipped",
			})
			break
		}

		report, err := nodeService.Doctor(ctx, n)
		if err != nil {
			checks = append(checks, DoctorCheck{
				Category: "nodes",
				Name:     fmt.Sprintf("node:%s", n.Name),
				Status:   DoctorFail,
				Error:    err.Error(),
			})
			continue
		}

		status := DoctorPass
		details := "all checks passed"
		if !report.Success {
			status = DoctorWarn
			// Find first failing check
			for _, c := range report.Checks {
				if c.Status == node.CheckFail {
					details = fmt.Sprintf("%s: %s", c.Name, c.Error)
					status = DoctorFail
					break
				}
				if c.Status == node.CheckWarn {
					details = fmt.Sprintf("%s: %s", c.Name, c.Details)
					break
				}
			}
		}

		checks = append(checks, DoctorCheck{
			Category: "nodes",
			Name:     fmt.Sprintf("node:%s", n.Name),
			Status:   status,
			Details:  details,
		})
	}

	return checks
}

func buildSummary(checks []DoctorCheck) DoctorSummary {
	summary := DoctorSummary{Total: len(checks)}
	for _, c := range checks {
		switch c.Status {
		case DoctorPass:
			summary.Passed++
		case DoctorWarn:
			summary.Warnings++
		case DoctorFail:
			summary.Failed++
		case DoctorSkip:
			summary.Skipped++
		}
	}
	return summary
}

func printDoctorReport(report *DoctorReport) {
	fmt.Println("Forge Doctor")
	fmt.Println("============")
	fmt.Println()

	// Group by category
	categories := []string{"dependencies", "config", "database", "nodes"}
	categoryChecks := make(map[string][]DoctorCheck)
	for _, c := range report.Checks {
		categoryChecks[c.Category] = append(categoryChecks[c.Category], c)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	for _, cat := range categories {
		checks, ok := categoryChecks[cat]
		if !ok || len(checks) == 0 {
			continue
		}

		fmt.Fprintf(w, "\n%s:\n", strings.ToUpper(cat))
		for _, c := range checks {
			icon := "?"
			switch c.Status {
			case DoctorPass:
				icon = "✓"
			case DoctorWarn:
				icon = "!"
			case DoctorFail:
				icon = "✗"
			case DoctorSkip:
				icon = "-"
			}

			detail := c.Details
			if c.Error != "" {
				detail = c.Error
			}

			fmt.Fprintf(w, "  [%s] %s\t%s\n", icon, c.Name, detail)
		}
	}
	w.Flush()

	fmt.Println()
	fmt.Printf("Summary: %d passed, %d warnings, %d failed\n",
		report.Summary.Passed, report.Summary.Warnings, report.Summary.Failed)
}
