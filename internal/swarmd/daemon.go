// Package swarmd provides the daemon scaffolding for the Swarm node service.
package swarmd

import (
	"context"
	"errors"
	"net"
	"strconv"

	"github.com/opencode-ai/swarm/internal/config"
	"github.com/rs/zerolog"
)

// Options configure the daemon runtime.
type Options struct {
	Hostname string
	Port     int
}

// Daemon is the long-running process responsible for node orchestration.
type Daemon struct {
	cfg    *config.Config
	logger zerolog.Logger
	opts   Options
}

// New constructs a daemon with the provided configuration.
func New(cfg *config.Config, logger zerolog.Logger, opts Options) (*Daemon, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	if opts.Hostname == "" {
		opts.Hostname = "127.0.0.1"
	}
	return &Daemon{
		cfg:    cfg,
		logger: logger,
		opts:   opts,
	}, nil
}

// Run blocks until the context is canceled, providing a graceful shutdown path.
func (d *Daemon) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is required")
	}

	d.logger.Info().
		Str("bind", d.bindAddr()).
		Msg("swarmd running (scaffolding only)")

	<-ctx.Done()
	d.logger.Info().Msg("swarmd shutdown complete")
	return nil
}

func (d *Daemon) bindAddr() string {
	return net.JoinHostPort(d.opts.Hostname, strconv.Itoa(d.opts.Port))
}
