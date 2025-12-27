// Package config handles Forge configuration loading and validation.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opencode-ai/swarm/internal/models"
)

// Config is the root configuration structure for Forge.
type Config struct {
	// Global settings
	Global GlobalConfig `yaml:"global" mapstructure:"global"`

	// Database settings
	Database DatabaseConfig `yaml:"database" mapstructure:"database"`

	// Logging settings
	Logging LoggingConfig `yaml:"logging" mapstructure:"logging"`

	// Accounts contains configured provider profiles.
	Accounts []AccountConfig `yaml:"accounts" mapstructure:"accounts"`

	// Default settings for nodes
	NodeDefaults NodeConfig `yaml:"node_defaults" mapstructure:"node_defaults"`

	// Default settings for workspaces
	WorkspaceDefaults WorkspaceConfig `yaml:"workspace_defaults" mapstructure:"workspace_defaults"`

	// Workspace-specific overrides
	WorkspaceOverrides []WorkspaceOverrideConfig `yaml:"workspace_overrides" mapstructure:"workspace_overrides"`

	// Default settings for agents
	AgentDefaults AgentConfig `yaml:"agent_defaults" mapstructure:"agent_defaults"`

	// Scheduler settings
	Scheduler SchedulerConfig `yaml:"scheduler" mapstructure:"scheduler"`

	// TUI settings
	TUI TUIConfig `yaml:"tui" mapstructure:"tui"`

	// EventRetention settings
	EventRetention EventRetentionConfig `yaml:"event_retention" mapstructure:"event_retention"`
}

// GlobalConfig contains global Forge settings.
type GlobalConfig struct {
	// DataDir is where Forge stores its data (default: ~/.local/share/forge).
	DataDir string `yaml:"data_dir" mapstructure:"data_dir"`

	// ConfigDir is where config files are stored (default: ~/.config/forge).
	ConfigDir string `yaml:"config_dir" mapstructure:"config_dir"`

	// AutoRegisterLocalNode automatically registers the local machine as a node.
	AutoRegisterLocalNode bool `yaml:"auto_register_local_node" mapstructure:"auto_register_local_node"`
}

// DatabaseConfig contains database settings.
type DatabaseConfig struct {
	// Path is the SQLite database file path.
	Path string `yaml:"path" mapstructure:"path"`

	// MaxConnections is the maximum number of database connections.
	MaxConnections int `yaml:"max_connections" mapstructure:"max_connections"`

	// BusyTimeout is how long to wait for a locked database (milliseconds).
	BusyTimeoutMs int `yaml:"busy_timeout_ms" mapstructure:"busy_timeout_ms"`
}

// LoggingConfig contains logging settings.
type LoggingConfig struct {
	// Level is the minimum log level (debug, info, warn, error).
	Level string `yaml:"level" mapstructure:"level"`

	// Format is the output format (json, console).
	Format string `yaml:"format" mapstructure:"format"`

	// File is an optional log file path.
	File string `yaml:"file" mapstructure:"file"`

	// EnableCaller adds caller information to logs.
	EnableCaller bool `yaml:"enable_caller" mapstructure:"enable_caller"`
}

// AccountConfig contains provider credentials and profile settings.
type AccountConfig struct {
	// Provider identifies the AI provider.
	Provider models.Provider `yaml:"provider" mapstructure:"provider"`

	// ProfileName is the human-friendly name for this account.
	ProfileName string `yaml:"profile_name" mapstructure:"profile_name"`

	// CredentialRef is a reference to the credential (env var, file path, or vault key).
	CredentialRef string `yaml:"credential_ref" mapstructure:"credential_ref"`

	// IsActive indicates if this account is enabled for use.
	IsActive bool `yaml:"is_active" mapstructure:"is_active"`
}

// NodeConfig contains default settings for nodes.
type NodeConfig struct {
	// SSHBackend is the default SSH backend (native, system, auto).
	SSHBackend models.SSHBackend `yaml:"ssh_backend" mapstructure:"ssh_backend"`

	// SSHTimeout is the connection timeout for SSH.
	SSHTimeout time.Duration `yaml:"ssh_timeout" mapstructure:"ssh_timeout"`

	// SSHKeyPath is the default SSH private key path.
	SSHKeyPath string `yaml:"ssh_key_path" mapstructure:"ssh_key_path"`

	// HealthCheckInterval is how often to check node health.
	HealthCheckInterval time.Duration `yaml:"health_check_interval" mapstructure:"health_check_interval"`
}

// WorkspaceConfig contains default settings for workspaces.
type WorkspaceConfig struct {
	// TmuxPrefix is the prefix for generated tmux session names.
	TmuxPrefix string `yaml:"tmux_prefix" mapstructure:"tmux_prefix"`

	// DefaultAgentType is the default agent type to spawn.
	DefaultAgentType models.AgentType `yaml:"default_agent_type" mapstructure:"default_agent_type"`

	// AutoImportExisting automatically imports existing tmux sessions.
	AutoImportExisting bool `yaml:"auto_import_existing" mapstructure:"auto_import_existing"`
}

// WorkspaceOverrideConfig provides per-workspace configuration overrides.
type WorkspaceOverrideConfig struct {
	// WorkspaceID matches a specific workspace id.
	WorkspaceID string `yaml:"workspace_id" mapstructure:"workspace_id"`

	// Name matches a workspace name.
	Name string `yaml:"name" mapstructure:"name"`

	// RepoPath matches the workspace repo path (supports glob patterns).
	RepoPath string `yaml:"repo_path" mapstructure:"repo_path"`

	// ApprovalPolicy overrides the approval policy for the workspace.
	ApprovalPolicy string `yaml:"approval_policy" mapstructure:"approval_policy"`

	// ApprovalRules apply when approval_policy is custom.
	ApprovalRules []ApprovalRule `yaml:"approval_rules" mapstructure:"approval_rules"`
}

// ApprovalRule defines a rule for approval decisions.
type ApprovalRule struct {
	// RequestType matches the approval request type (supports "*" wildcard).
	RequestType string `yaml:"request_type" mapstructure:"request_type"`

	// Action is approve, deny, or prompt.
	Action string `yaml:"action" mapstructure:"action"`
}

// AgentConfig contains default settings for agents.
type AgentConfig struct {
	// DefaultType is the default agent type.
	DefaultType models.AgentType `yaml:"default_type" mapstructure:"default_type"`

	// StatePollingInterval is how often to poll agent state.
	StatePollingInterval time.Duration `yaml:"state_polling_interval" mapstructure:"state_polling_interval"`

	// IdleTimeout is how long of no activity before considering agent idle.
	IdleTimeout time.Duration `yaml:"idle_timeout" mapstructure:"idle_timeout"`

	// TranscriptBufferSize is the max lines to keep in transcript buffer.
	TranscriptBufferSize int `yaml:"transcript_buffer_size" mapstructure:"transcript_buffer_size"`

	// ApprovalPolicy is the default approval policy (strict, permissive, custom).
	ApprovalPolicy string `yaml:"approval_policy" mapstructure:"approval_policy"`

	// ApprovalRules apply when approval_policy is custom.
	ApprovalRules []ApprovalRule `yaml:"approval_rules" mapstructure:"approval_rules"`
}

// SchedulerConfig contains scheduler settings.
type SchedulerConfig struct {
	// DispatchInterval is how often the scheduler runs.
	DispatchInterval time.Duration `yaml:"dispatch_interval" mapstructure:"dispatch_interval"`

	// MaxRetries is the maximum dispatch retry count.
	MaxRetries int `yaml:"max_retries" mapstructure:"max_retries"`

	// RetryBackoff is the base backoff duration for retries.
	RetryBackoff time.Duration `yaml:"retry_backoff" mapstructure:"retry_backoff"`

	// DefaultCooldownDuration is the default cooldown after rate limiting.
	DefaultCooldownDuration time.Duration `yaml:"default_cooldown_duration" mapstructure:"default_cooldown_duration"`

	// AutoRotateOnRateLimit automatically rotates accounts on rate limit.
	AutoRotateOnRateLimit bool `yaml:"auto_rotate_on_rate_limit" mapstructure:"auto_rotate_on_rate_limit"`
}

// TUIConfig contains TUI settings.
type TUIConfig struct {
	// RefreshInterval is how often to refresh the display.
	RefreshInterval time.Duration `yaml:"refresh_interval" mapstructure:"refresh_interval"`

	// Theme is the color theme (default, high-contrast).
	Theme string `yaml:"theme" mapstructure:"theme"`

	// ShowTimestamps shows timestamps in the UI.
	ShowTimestamps bool `yaml:"show_timestamps" mapstructure:"show_timestamps"`

	// CompactMode uses a more compact layout.
	CompactMode bool `yaml:"compact_mode" mapstructure:"compact_mode"`
}

// EventRetentionConfig contains event retention policy settings.
type EventRetentionConfig struct {
	// Enabled controls whether retention cleanup runs.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// MaxAge is the maximum age of events to keep (e.g., 720h for 30 days).
	// Events older than this will be deleted. Zero means no age limit.
	MaxAge time.Duration `yaml:"max_age" mapstructure:"max_age"`

	// MaxCount is the maximum number of events to keep.
	// Oldest events beyond this count will be deleted. Zero means no count limit.
	MaxCount int `yaml:"max_count" mapstructure:"max_count"`

	// CleanupInterval is how often to run the cleanup job.
	CleanupInterval time.Duration `yaml:"cleanup_interval" mapstructure:"cleanup_interval"`

	// ArchiveBeforeDelete controls whether to archive events before deletion.
	ArchiveBeforeDelete bool `yaml:"archive_before_delete" mapstructure:"archive_before_delete"`

	// ArchiveDir is the directory to store archived events (defaults to DataDir/archives).
	ArchiveDir string `yaml:"archive_dir" mapstructure:"archive_dir"`

	// BatchSize is the number of events to process per cleanup batch.
	BatchSize int `yaml:"batch_size" mapstructure:"batch_size"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()

	return &Config{
		Global: GlobalConfig{
			DataDir:               filepath.Join(homeDir, ".local", "share", "forge"),
			ConfigDir:             filepath.Join(homeDir, ".config", "forge"),
			AutoRegisterLocalNode: true,
		},
		Database: DatabaseConfig{
			Path:           "", // Will be set to DataDir/forge.db
			MaxConnections: 10,
			BusyTimeoutMs:  5000,
		},
		Logging: LoggingConfig{
			Level:        "info",
			Format:       "console",
			EnableCaller: false,
		},
		Accounts: []AccountConfig{},
		NodeDefaults: NodeConfig{
			SSHBackend:          models.SSHBackendAuto,
			SSHTimeout:          30 * time.Second,
			HealthCheckInterval: 60 * time.Second,
		},
		WorkspaceDefaults: WorkspaceConfig{
			TmuxPrefix:         "forge",
			DefaultAgentType:   models.AgentTypeOpenCode,
			AutoImportExisting: false,
		},
		AgentDefaults: AgentConfig{
			DefaultType:          models.AgentTypeOpenCode,
			StatePollingInterval: 2 * time.Second,
			IdleTimeout:          10 * time.Second,
			TranscriptBufferSize: 10000,
			ApprovalPolicy:       "strict",
		},
		Scheduler: SchedulerConfig{
			DispatchInterval:        1 * time.Second,
			MaxRetries:              3,
			RetryBackoff:            5 * time.Second,
			DefaultCooldownDuration: 5 * time.Minute,
			AutoRotateOnRateLimit:   true,
		},
		TUI: TUIConfig{
			RefreshInterval: 500 * time.Millisecond,
			Theme:           "default",
			ShowTimestamps:  true,
			CompactMode:     false,
		},
		EventRetention: EventRetentionConfig{
			Enabled:             true,
			MaxAge:              30 * 24 * time.Hour, // 30 days
			MaxCount:            0,                   // No count limit by default
			CleanupInterval:     1 * time.Hour,
			ArchiveBeforeDelete: false,
			ArchiveDir:          "", // Will be set to DataDir/archives
			BatchSize:           1000,
		},
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Global.DataDir) == "" {
		return fmt.Errorf("global.data_dir is required")
	}
	if strings.TrimSpace(c.Global.ConfigDir) == "" {
		return fmt.Errorf("global.config_dir is required")
	}

	if c.Database.MaxConnections < 1 {
		return fmt.Errorf("database.max_connections must be at least 1")
	}
	if c.Database.BusyTimeoutMs < 0 {
		return fmt.Errorf("database.busy_timeout_ms must be zero or greater")
	}

	switch strings.ToLower(strings.TrimSpace(c.Logging.Level)) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be one of debug, info, warn, error")
	}
	switch strings.ToLower(strings.TrimSpace(c.Logging.Format)) {
	case "console", "json":
	default:
		return fmt.Errorf("logging.format must be one of console, json")
	}

	switch c.NodeDefaults.SSHBackend {
	case models.SSHBackendNative, models.SSHBackendSystem, models.SSHBackendAuto:
	default:
		return fmt.Errorf("node_defaults.ssh_backend must be native, system, or auto")
	}
	if c.NodeDefaults.SSHTimeout <= 0 {
		return fmt.Errorf("node_defaults.ssh_timeout must be greater than 0")
	}
	if c.NodeDefaults.HealthCheckInterval <= 0 {
		return fmt.Errorf("node_defaults.health_check_interval must be greater than 0")
	}

	if strings.TrimSpace(c.WorkspaceDefaults.TmuxPrefix) == "" {
		return fmt.Errorf("workspace_defaults.tmux_prefix is required")
	}
	if !isValidAgentType(c.WorkspaceDefaults.DefaultAgentType) {
		return fmt.Errorf("workspace_defaults.default_agent_type must be one of opencode, claude-code, codex, gemini, generic")
	}

	if c.AgentDefaults.StatePollingInterval < 100*time.Millisecond {
		return fmt.Errorf("agent_defaults.state_polling_interval must be at least 100ms")
	}
	if c.AgentDefaults.IdleTimeout <= 0 {
		return fmt.Errorf("agent_defaults.idle_timeout must be greater than 0")
	}
	if c.AgentDefaults.TranscriptBufferSize < 1 {
		return fmt.Errorf("agent_defaults.transcript_buffer_size must be at least 1")
	}
	if !isValidAgentType(c.AgentDefaults.DefaultType) {
		return fmt.Errorf("agent_defaults.default_type must be one of opencode, claude-code, codex, gemini, generic")
	}
	if err := validateApprovalPolicy("agent_defaults", c.AgentDefaults.ApprovalPolicy, c.AgentDefaults.ApprovalRules); err != nil {
		return err
	}

	for i, override := range c.WorkspaceOverrides {
		path := fmt.Sprintf("workspace_overrides[%d]", i)
		if strings.TrimSpace(override.WorkspaceID) == "" && strings.TrimSpace(override.Name) == "" && strings.TrimSpace(override.RepoPath) == "" {
			return fmt.Errorf("%s must include workspace_id, name, or repo_path", path)
		}
		if strings.TrimSpace(override.ApprovalPolicy) == "" && len(override.ApprovalRules) == 0 {
			return fmt.Errorf("%s must set approval_policy or approval_rules", path)
		}
		if err := validateApprovalPolicy(path, override.ApprovalPolicy, override.ApprovalRules); err != nil {
			return err
		}
	}

	if c.Scheduler.DispatchInterval < 100*time.Millisecond {
		return fmt.Errorf("scheduler.dispatch_interval must be at least 100ms")
	}
	if c.Scheduler.MaxRetries < 0 {
		return fmt.Errorf("scheduler.max_retries must be zero or greater")
	}
	if c.Scheduler.RetryBackoff <= 0 {
		return fmt.Errorf("scheduler.retry_backoff must be greater than 0")
	}
	if c.Scheduler.DefaultCooldownDuration <= 0 {
		return fmt.Errorf("scheduler.default_cooldown_duration must be greater than 0")
	}

	if c.TUI.RefreshInterval <= 0 {
		return fmt.Errorf("tui.refresh_interval must be greater than 0")
	}
	switch strings.ToLower(strings.TrimSpace(c.TUI.Theme)) {
	case "default", "high-contrast":
	default:
		return fmt.Errorf("tui.theme must be one of default, high-contrast")
	}

	// Event retention validation
	if c.EventRetention.Enabled {
		if c.EventRetention.MaxAge < 0 {
			return fmt.Errorf("event_retention.max_age must be zero or positive")
		}
		if c.EventRetention.MaxCount < 0 {
			return fmt.Errorf("event_retention.max_count must be zero or positive")
		}
		if c.EventRetention.MaxAge == 0 && c.EventRetention.MaxCount == 0 {
			return fmt.Errorf("event_retention: at least one of max_age or max_count must be set when enabled")
		}
		if c.EventRetention.CleanupInterval < 1*time.Minute {
			return fmt.Errorf("event_retention.cleanup_interval must be at least 1 minute")
		}
		if c.EventRetention.BatchSize < 1 {
			return fmt.Errorf("event_retention.batch_size must be at least 1")
		}
	}

	for i, account := range c.Accounts {
		if account.Provider == "" {
			return fmt.Errorf("accounts[%d].provider is required", i)
		}
		if account.ProfileName == "" {
			return fmt.Errorf("accounts[%d].profile_name is required", i)
		}
		if account.CredentialRef == "" {
			return fmt.Errorf("accounts[%d].credential_ref is required", i)
		}
		switch account.Provider {
		case models.ProviderAnthropic, models.ProviderOpenAI, models.ProviderGoogle, models.ProviderCustom:
			// ok
		default:
			return fmt.Errorf("accounts[%d].provider must be one of anthropic, openai, google, custom", i)
		}
	}

	return nil
}

func isValidAgentType(agentType models.AgentType) bool {
	switch agentType {
	case models.AgentTypeOpenCode,
		models.AgentTypeClaudeCode,
		models.AgentTypeCodex,
		models.AgentTypeGemini,
		models.AgentTypeGeneric:
		return true
	default:
		return false
	}
}

// EnsureDirectories creates required directories.
func (c *Config) EnsureDirectories() error {
	dirs := []string{
		c.Global.DataDir,
		c.Global.ConfigDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// DatabasePath returns the full database path.
func (c *Config) DatabasePath() string {
	if c.Database.Path != "" {
		return c.Database.Path
	}
	return filepath.Join(c.Global.DataDir, "forge.db")
}

// ArchivePath returns the full archive directory path.
func (c *Config) ArchivePath() string {
	if c.EventRetention.ArchiveDir != "" {
		return c.EventRetention.ArchiveDir
	}
	return filepath.Join(c.Global.DataDir, "archives")
}
