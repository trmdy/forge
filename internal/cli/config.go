package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	configInitForce bool
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configPathCmd)

	configInitCmd.Flags().BoolVarP(&configInitForce, "force", "f", false, "overwrite existing config file")
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage global configuration",
	Long:  `Manage Forge global configuration at ~/.config/forge/config.yaml.`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a default global config file",
	Long: `Create a default global configuration file at ~/.config/forge/config.yaml.

The generated config includes all available options with their default values
and explanatory comments. You can then edit it to customize Forge behavior.

Use --force to overwrite an existing config file.`,
	Example: `  forge config init
  forge config init --force`,
	RunE: runConfigInit,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the global config file path",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath := filepath.Join(home, ".config", "forge", "config.yaml")

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]string{"path": configPath})
		}

		fmt.Println(configPath)
		return nil
	},
}

type configInitResult struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
	Message string `json:"message,omitempty"`
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(home, ".config", "forge")
	configPath := filepath.Join(configDir, "config.yaml")

	// Check if file exists
	if !configInitForce {
		if _, err := os.Stat(configPath); err == nil {
			result := configInitResult{
				Path:    configPath,
				Created: false,
				Message: "config file already exists (use --force to overwrite)",
			}
			if IsJSONOutput() || IsJSONLOutput() {
				data, _ := json.Marshal(result)
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Config file already exists: %s\n", configPath)
			fmt.Println("Use --force to overwrite.")
			return nil
		}
	}

	// Create directory if needed
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config file
	if err := os.WriteFile(configPath, []byte(defaultGlobalConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	result := configInitResult{
		Path:    configPath,
		Created: true,
	}

	if IsJSONOutput() || IsJSONLOutput() {
		data, _ := json.Marshal(result)
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Created config file: %s\n", configPath)
	fmt.Println("\nEdit this file to customize Forge behavior.")
	return nil
}

const defaultGlobalConfig = `# Forge Global Configuration
# Location: ~/.config/forge/config.yaml
#
# This file configures global Forge behavior. All settings have sensible defaults,
# so you only need to uncomment and modify the ones you want to change.
#
# Forge also supports environment variables with the FORGE_ prefix:
#   FORGE_LOGGING_LEVEL=debug
#   FORGE_DATABASE_PATH=/custom/path/forge.db

# =============================================================================
# Global Settings
# =============================================================================
global:
  # Where Forge stores runtime data (database, logs, etc.)
  # Default: ~/.local/share/forge
  # data_dir: ~/.local/share/forge

  # Where config files are stored
  # Default: ~/.config/forge
  # config_dir: ~/.config/forge

  # Automatically register the local machine as a node
  # Default: true
  # auto_register_local_node: true

# =============================================================================
# Database Settings
# =============================================================================
database:
  # SQLite database file path (empty = data_dir/forge.db)
  # path: ""

  # Maximum number of database connections
  # Default: 10
  # max_connections: 10

  # How long to wait for a locked database (milliseconds)
  # Default: 5000
  # busy_timeout_ms: 5000

# =============================================================================
# Logging Settings
# =============================================================================
logging:
  # Minimum log level: debug, info, warn, error
  # Default: info
  # level: info

  # Output format: console, json
  # Default: console
  # format: console

  # Optional log file path (empty = stdout only)
  # file: ""

  # Add caller information (file:line) to logs
  # Default: false
  # enable_caller: false

# =============================================================================
# Loop Defaults
# =============================================================================
loop_defaults:
  # Sleep duration between loop iterations
  # Default: 30s
  # interval: 30s

  # Default base prompt path (relative to repo root)
  # prompt: ""

  # Default base prompt message content
  # prompt_msg: ""

# =============================================================================
# Scheduler Settings
# =============================================================================
scheduler:
  # How often the scheduler runs
  # Default: 1s
  # dispatch_interval: 1s

  # Maximum dispatch retry count
  # Default: 3
  # max_retries: 3

  # Base backoff duration for retries
  # Default: 5s
  # retry_backoff: 5s

  # Default cooldown after rate limiting
  # Default: 5m
  # default_cooldown_duration: 5m

  # Automatically rotate accounts on rate limit
  # Default: true
  # auto_rotate_on_rate_limit: true

# =============================================================================
# TUI Settings
# =============================================================================
tui:
  # How often to refresh the display
  # Default: 500ms
  # refresh_interval: 500ms

  # Color theme: default, high-contrast
  # Default: default
  # theme: default

  # Show timestamps in the UI
  # Default: true
  # show_timestamps: true

  # Use a more compact layout
  # Default: false
  # compact_mode: false

# =============================================================================
# Node Defaults (for remote SSH nodes)
# =============================================================================
node_defaults:
  # SSH backend: native (Go), system (ssh command), auto
  # Default: auto
  # ssh_backend: auto

  # SSH connection timeout
  # Default: 30s
  # ssh_timeout: 30s

  # Default SSH private key path
  # ssh_key_path: ~/.ssh/id_rsa

  # How often to check node health
  # Default: 60s
  # health_check_interval: 60s

# =============================================================================
# Workspace Defaults
# =============================================================================
workspace_defaults:
  # Prefix for generated tmux session names
  # Default: forge
  # tmux_prefix: forge

  # Default agent type: opencode, claude-code, codex, gemini, generic
  # Default: opencode
  # default_agent_type: opencode

  # Automatically import existing tmux sessions
  # Default: false
  # auto_import_existing: false

# =============================================================================
# Agent Defaults
# =============================================================================
agent_defaults:
  # Default agent type
  # Default: opencode
  # default_type: opencode

  # How often to poll agent state
  # Default: 2s
  # state_polling_interval: 2s

  # How long of no activity before considering agent idle
  # Default: 10s
  # idle_timeout: 10s

  # Max lines to keep in transcript buffer
  # Default: 10000
  # transcript_buffer_size: 10000

  # Approval policy: strict, permissive, custom
  # Default: strict
  # approval_policy: strict

# =============================================================================
# Event Retention
# =============================================================================
event_retention:
  # Enable automatic event cleanup
  # Default: true
  # enabled: true

  # Maximum age of events to keep (e.g., 720h = 30 days)
  # Default: 720h (30 days)
  # max_age: 720h

  # Maximum number of events to keep (0 = no limit)
  # Default: 0
  # max_count: 0

  # How often to run cleanup
  # Default: 1h
  # cleanup_interval: 1h

  # Archive events before deletion
  # Default: false
  # archive_before_delete: false

  # Events per cleanup batch
  # Default: 1000
  # batch_size: 1000

# =============================================================================
# Profiles (harness + auth combinations)
# =============================================================================
# Profiles are typically managed via 'forge profile' commands or imported
# from shell aliases via 'forge profile init'.
#
# Example profile definition:
# profiles:
#   - name: claude-main
#     harness: claude
#     auth_home: ~/.claude
#     prompt_mode: env
#     command_template: claude -p "$FORGE_PROMPT_CONTENT" --dangerously-skip-permissions
#     max_concurrency: 1

# =============================================================================
# Pools (groups of profiles for load balancing)
# =============================================================================
# Pools let you group profiles and distribute work across them.
#
# Example pool definition:
# pools:
#   - name: anthropic
#     strategy: round-robin
#     profiles:
#       - claude-main
#       - claude-backup
#
# default_pool: anthropic
`
