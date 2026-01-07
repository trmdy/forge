package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Loader handles configuration loading with Viper.
type Loader struct {
	v          *viper.Viper
	configFile string
}

// NewLoader creates a new configuration loader.
func NewLoader() *Loader {
	return &Loader{
		v: viper.New(),
	}
}

// SetConfigFile sets an explicit config file path.
func (l *Loader) SetConfigFile(path string) {
	l.configFile = path
}

// Load loads configuration with proper precedence:
// defaults < config file < env vars < CLI flags
func (l *Loader) Load() (*Config, error) {
	// Start with defaults
	cfg := DefaultConfig()

	// Set up Viper
	l.setupViper(cfg)

	// Load config file
	if err := l.loadConfigFile(); err != nil {
		// Config file is optional, only error if explicitly specified
		if l.configFile != "" {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Unmarshal into config struct
	if err := l.v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Expand ~ in paths
	expandPaths(cfg)

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// expandTilde expands ~ to the user's home directory.
func expandTilde(path string) string {
	if path == "" {
		return path
	}
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// expandPaths expands ~ in all path-related config fields.
func expandPaths(cfg *Config) {
	cfg.Global.DataDir = expandTilde(cfg.Global.DataDir)
	cfg.Global.ConfigDir = expandTilde(cfg.Global.ConfigDir)
	cfg.Database.Path = expandTilde(cfg.Database.Path)
	cfg.Logging.File = expandTilde(cfg.Logging.File)
	cfg.NodeDefaults.SSHKeyPath = expandTilde(cfg.NodeDefaults.SSHKeyPath)
	cfg.EventRetention.ArchiveDir = expandTilde(cfg.EventRetention.ArchiveDir)
	cfg.LoopDefaults.Prompt = expandTilde(cfg.LoopDefaults.Prompt)
	for i := range cfg.Profiles {
		cfg.Profiles[i].AuthHome = expandTilde(cfg.Profiles[i].AuthHome)
	}
}

// setupViper configures Viper with defaults and environment bindings.
func (l *Loader) setupViper(cfg *Config) {
	v := l.v

	// Config file settings
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	// XDG config directories - check forge first, then swarm for migration
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		v.AddConfigPath(filepath.Join(xdgConfig, "forge"))
		v.AddConfigPath(filepath.Join(xdgConfig, "swarm")) // legacy fallback
	}

	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		v.AddConfigPath(filepath.Join(homeDir, ".config", "forge"))
		v.AddConfigPath(filepath.Join(homeDir, ".config", "swarm")) // legacy fallback
	}

	// Current directory
	v.AddConfigPath(".")

	// Environment variables - FORGE_ prefix, with SWARM_ as fallback
	v.SetEnvPrefix("FORGE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Bind legacy SWARM_* env vars as fallbacks
	bindLegacyEnvVars(v)

	// Set defaults from config struct
	l.setDefaults(cfg)
}

// setDefaults sets all default values in Viper.
func (l *Loader) setDefaults(cfg *Config) {
	v := l.v

	// Global
	v.SetDefault("global.data_dir", cfg.Global.DataDir)
	v.SetDefault("global.config_dir", cfg.Global.ConfigDir)
	v.SetDefault("global.auto_register_local_node", cfg.Global.AutoRegisterLocalNode)

	// Database
	v.SetDefault("database.path", cfg.Database.Path)
	v.SetDefault("database.max_connections", cfg.Database.MaxConnections)
	v.SetDefault("database.busy_timeout_ms", cfg.Database.BusyTimeoutMs)

	// Logging
	v.SetDefault("logging.level", cfg.Logging.Level)
	v.SetDefault("logging.format", cfg.Logging.Format)
	v.SetDefault("logging.file", cfg.Logging.File)
	v.SetDefault("logging.enable_caller", cfg.Logging.EnableCaller)

	// Node defaults
	v.SetDefault("node_defaults.ssh_backend", string(cfg.NodeDefaults.SSHBackend))
	v.SetDefault("node_defaults.ssh_timeout", cfg.NodeDefaults.SSHTimeout)
	v.SetDefault("node_defaults.ssh_key_path", cfg.NodeDefaults.SSHKeyPath)
	v.SetDefault("node_defaults.health_check_interval", cfg.NodeDefaults.HealthCheckInterval)

	// Workspace defaults
	v.SetDefault("workspace_defaults.tmux_prefix", cfg.WorkspaceDefaults.TmuxPrefix)
	v.SetDefault("workspace_defaults.default_agent_type", string(cfg.WorkspaceDefaults.DefaultAgentType))
	v.SetDefault("workspace_defaults.auto_import_existing", cfg.WorkspaceDefaults.AutoImportExisting)

	// Agent defaults
	v.SetDefault("agent_defaults.default_type", string(cfg.AgentDefaults.DefaultType))
	v.SetDefault("agent_defaults.state_polling_interval", cfg.AgentDefaults.StatePollingInterval)
	v.SetDefault("agent_defaults.idle_timeout", cfg.AgentDefaults.IdleTimeout)
	v.SetDefault("agent_defaults.transcript_buffer_size", cfg.AgentDefaults.TranscriptBufferSize)
	v.SetDefault("agent_defaults.approval_policy", cfg.AgentDefaults.ApprovalPolicy)

	// Scheduler
	v.SetDefault("scheduler.dispatch_interval", cfg.Scheduler.DispatchInterval)
	v.SetDefault("scheduler.max_retries", cfg.Scheduler.MaxRetries)
	v.SetDefault("scheduler.retry_backoff", cfg.Scheduler.RetryBackoff)
	v.SetDefault("scheduler.default_cooldown_duration", cfg.Scheduler.DefaultCooldownDuration)
	v.SetDefault("scheduler.auto_rotate_on_rate_limit", cfg.Scheduler.AutoRotateOnRateLimit)

	// Loop defaults
	v.SetDefault("loop_defaults.interval", cfg.LoopDefaults.Interval)
	v.SetDefault("loop_defaults.prompt", cfg.LoopDefaults.Prompt)
	v.SetDefault("loop_defaults.prompt_msg", cfg.LoopDefaults.PromptMsg)

	// Pools/default pool
	v.SetDefault("default_pool", cfg.DefaultPool)

	// TUI
	v.SetDefault("tui.refresh_interval", cfg.TUI.RefreshInterval)
	v.SetDefault("tui.theme", cfg.TUI.Theme)
	v.SetDefault("tui.show_timestamps", cfg.TUI.ShowTimestamps)
	v.SetDefault("tui.compact_mode", cfg.TUI.CompactMode)
}

// loadConfigFile attempts to load the configuration file.
func (l *Loader) loadConfigFile() error {
	if l.configFile != "" {
		l.v.SetConfigFile(l.configFile)
	}

	if err := l.v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, use defaults
			return nil
		}
		return err
	}

	return nil
}

// ConfigFileUsed returns the config file that was loaded.
func (l *Loader) ConfigFileUsed() string {
	return l.v.ConfigFileUsed()
}

// Get returns a Viper value by key.
func (l *Loader) Get(key string) interface{} {
	return l.v.Get(key)
}

// Set sets a Viper value by key.
func (l *Loader) Set(key string, value interface{}) {
	l.v.Set(key, value)
}

// BindEnv binds an environment variable to a config key.
func (l *Loader) BindEnv(key string, envVar string) error {
	return l.v.BindEnv(key, envVar)
}

// Viper returns the underlying Viper instance for advanced use.
func (l *Loader) Viper() *viper.Viper {
	return l.v
}

// LoadFromFile loads configuration from a specific file.
func LoadFromFile(path string) (*Config, error) {
	loader := NewLoader()
	loader.SetConfigFile(path)
	return loader.Load()
}

// LoadDefault loads configuration with default search paths.
func LoadDefault() (*Config, error) {
	loader := NewLoader()
	return loader.Load()
}

// MustLoad loads configuration or panics on error.
func MustLoad() *Config {
	cfg, err := LoadDefault()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}
	return cfg
}

// bindLegacyEnvVars binds SWARM_* environment variables as deprecated fallbacks.
// This allows users to migrate from swarm to forge gradually.
func bindLegacyEnvVars(v *viper.Viper) {
	legacyBindings := map[string]string{
		"global.data_dir":                       "SWARM_GLOBAL_DATA_DIR",
		"global.config_dir":                     "SWARM_GLOBAL_CONFIG_DIR",
		"database.path":                         "SWARM_DATABASE_PATH",
		"logging.level":                         "SWARM_LOGGING_LEVEL",
		"logging.format":                        "SWARM_LOGGING_FORMAT",
		"scheduler.dispatch_interval":           "SWARM_SCHEDULER_DISPATCH_INTERVAL",
		"scheduler.default_cooldown_duration":   "SWARM_SCHEDULER_DEFAULT_COOLDOWN_DURATION",
		"agent_defaults.state_polling_interval": "SWARM_AGENT_DEFAULTS_STATE_POLLING_INTERVAL",
	}

	for key, envVar := range legacyBindings {
		// Only bind if the FORGE_ equivalent is not set
		forgeEnvVar := strings.Replace(envVar, "SWARM_", "FORGE_", 1)
		if os.Getenv(forgeEnvVar) == "" && os.Getenv(envVar) != "" {
			_ = v.BindEnv(key, envVar)
		}
	}
}
