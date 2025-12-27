// Package cli provides TUI launch commands.
package cli

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/opencode-ai/swarm/internal/adapters"
	"github.com/opencode-ai/swarm/internal/db"
	"github.com/opencode-ai/swarm/internal/state"
	"github.com/opencode-ai/swarm/internal/tmux"
	"github.com/opencode-ai/swarm/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func init() {
	rootCmd.AddCommand(uiCmd)
}

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch the Forge TUI",
	Long:  "Launch the Forge terminal user interface (TUI).",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

func runTUI() error {
	if IsNonInteractive() {
		return &PreflightError{
			Message:  "TUI requires an interactive terminal",
			Hint:     "Run without --non-interactive and with a TTY, or use CLI subcommands",
			NextStep: "forge --help",
		}
	}

	// Open database for state engine
	database, err := openDatabase()
	if err != nil {
		return err
	}
	defer database.Close()

	// Create state engine dependencies
	agentRepo := db.NewAgentRepository(database)
	eventRepo := db.NewEventRepository(database)
	tmuxClient := tmux.NewClient(nil) // local tmux
	registry := adapters.NewRegistry()

	// Create and start state engine
	stateEngine := state.NewEngine(agentRepo, eventRepo, tmuxClient, registry)

	// Create and start the state poller for live agent updates
	poller := state.NewPoller(state.DefaultPollerConfig(), stateEngine, agentRepo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := poller.Start(ctx); err != nil {
		return err
	}
	defer poller.Stop()

	// Build TUI config from app config
	tuiConfig := tui.Config{
		StateEngine: stateEngine,
	}
	if cfg := GetConfig(); cfg != nil {
		tuiConfig.Theme = cfg.TUI.Theme
	}
	tuiConfig.AgentMail = agentMailConfigFromEnv()

	return tui.RunWithConfig(tuiConfig)
}

func hasTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func agentMailConfigFromEnv() tui.AgentMailConfig {
	cfg := tui.AgentMailConfig{
		URL:     getEnvWithFallback("FORGE_AGENT_MAIL_URL", "SWARM_AGENT_MAIL_URL"),
		Project: getEnvWithFallback("FORGE_AGENT_MAIL_PROJECT", "SWARM_AGENT_MAIL_PROJECT"),
		Agent:   getEnvWithFallback("FORGE_AGENT_MAIL_AGENT", "SWARM_AGENT_MAIL_AGENT"),
	}

	if value := getEnvWithFallback("FORGE_AGENT_MAIL_LIMIT", "SWARM_AGENT_MAIL_LIMIT"); value != "" {
		if limit, err := strconv.Atoi(value); err == nil && limit > 0 {
			cfg.Limit = limit
		}
	}

	if value := getEnvWithFallback("FORGE_AGENT_MAIL_POLL_INTERVAL", "SWARM_AGENT_MAIL_POLL_INTERVAL"); value != "" {
		if parsed, ok := parseEnvDuration(value); ok {
			cfg.PollInterval = parsed
		}
	}

	if value := getEnvWithFallback("FORGE_AGENT_MAIL_TIMEOUT", "SWARM_AGENT_MAIL_TIMEOUT"); value != "" {
		if parsed, ok := parseEnvDuration(value); ok {
			cfg.Timeout = parsed
		}
	}

	return cfg
}

// getEnvWithFallback returns the value of the primary env var, or falls back to legacy.
func getEnvWithFallback(primary, legacy string) string {
	if value := strings.TrimSpace(os.Getenv(primary)); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv(legacy))
}

func parseEnvDuration(value string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed, true
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second, true
	}
	return 0, false
}
