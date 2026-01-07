package cli

import (
	"os"
	"path/filepath"
)

// configDirFunc is a function that returns the config directory.
// It can be overridden in tests.
var configDirFunc = defaultConfigDir

func getConfigDir() string {
	return configDirFunc()
}

func defaultConfigDir() string {
	// Check XDG_CONFIG_HOME first
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "forge")
	}

	// Fall back to ~/.config/forge
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "forge")
	}
	return filepath.Join(homeDir, ".config", "forge")
}
