// Package main is the entry point for the swarmd daemon.
// swarmd runs on each node to provide real-time agent orchestration,
// screen capture, and event streaming to the control plane.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/opencode-ai/swarm/internal/config"
	"github.com/opencode-ai/swarm/internal/logging"
	"github.com/opencode-ai/swarm/internal/swarmd"
)

// Version information (set by goreleaser)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	hostname := flag.String("hostname", swarmd.DefaultHost, "hostname to listen on")
	port := flag.Int("port", swarmd.DefaultPort, "port to listen on")
	configFile := flag.String("config", "", "config file (default is $HOME/.config/swarm/config.yaml)")
	logLevel := flag.String("log-level", "", "override logging level (debug, info, warn, error)")
	logFormat := flag.String("log-format", "", "override logging format (json, console)")
	defaultDisk := swarmd.DefaultDiskMonitorConfig()
	diskPath := flag.String("disk-path", "", "filesystem path to monitor for disk usage")
	diskWarn := flag.Float64("disk-warn", defaultDisk.WarnPercent, "disk usage percent to warn at")
	diskCritical := flag.Float64("disk-critical", defaultDisk.CriticalPercent, "disk usage percent to treat as critical")
	diskResume := flag.Float64("disk-resume", defaultDisk.ResumePercent, "disk usage percent to resume paused agents")
	diskPause := flag.Bool("disk-pause", defaultDisk.PauseAgents, "pause agent processes when disk is critically full")
	flag.Parse()

	cfg, loader, err := loadConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if *logLevel != "" {
		cfg.Logging.Level = *logLevel
	}
	if *logFormat != "" {
		cfg.Logging.Format = *logFormat
	}

	logging.Init(logging.Config{
		Level:        cfg.Logging.Level,
		Format:       cfg.Logging.Format,
		EnableCaller: cfg.Logging.EnableCaller,
	})
	logger := logging.Component("swarmd")

	if err := cfg.EnsureDirectories(); err != nil {
		logger.Warn().Err(err).Msg("failed to create directories")
	}

	if cfgUsed := loader.ConfigFileUsed(); cfgUsed != "" {
		logger.Debug().Str("config_file", cfgUsed).Msg("loaded config file")
	}

	logger.Info().
		Str("version", version).
		Str("commit", commit).
		Str("built", date).
		Msg("swarmd starting")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	diskConfig := swarmd.DefaultDiskMonitorConfig()
	if cfg.Global.DataDir != "" {
		diskConfig.Path = cfg.Global.DataDir
	}
	if *diskPath != "" {
		diskConfig.Path = *diskPath
	}
	diskConfig.WarnPercent = *diskWarn
	diskConfig.CriticalPercent = *diskCritical
	diskConfig.ResumePercent = *diskResume
	diskConfig.PauseAgents = *diskPause

	daemon, err := swarmd.New(cfg, logger, swarmd.Options{
		Hostname:          *hostname,
		Port:              *port,
		DiskMonitorConfig: &diskConfig,
	})
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize swarmd")
		os.Exit(1)
	}

	if err := daemon.Run(ctx); err != nil {
		logger.Error().Err(err).Msg("swarmd exited with error")
		os.Exit(1)
	}
}

func loadConfig(path string) (*config.Config, *config.Loader, error) {
	loader := config.NewLoader()
	if path != "" {
		loader.SetConfigFile(path)
	}
	cfg, err := loader.Load()
	if err != nil {
		return nil, nil, err
	}
	return cfg, loader, nil
}
