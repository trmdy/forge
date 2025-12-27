// Package swarmd provides the daemon scaffolding for the Swarm node service.
package swarmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	swarmdv1 "github.com/opencode-ai/swarm/gen/swarmd/v1"
	"github.com/opencode-ai/swarm/internal/config"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

// Options configure the daemon runtime.
type Options struct {
	Hostname string
	Port     int
	Version  string

	// RateLimitEnabled enables rate limiting (default: true).
	RateLimitEnabled *bool

	// CustomRateLimits allows overriding default rate limits per method.
	CustomRateLimits map[string]RateLimitConfig

	// GlobalRateLimit sets an optional global rate limit across all methods.
	GlobalRateLimit *RateLimitConfig

	// ResourceMonitorEnabled enables resource monitoring (default: true).
	ResourceMonitorEnabled *bool

	// DefaultResourceLimits sets the default resource limits for agents.
	DefaultResourceLimits *ResourceLimits

	// DiskMonitorConfig customizes disk usage monitoring.
	DiskMonitorConfig *DiskMonitorConfig
}

// Daemon is the long-running process responsible for node orchestration.
type Daemon struct {
	cfg    *config.Config
	logger zerolog.Logger
	opts   Options

	server          *Server
	grpcServer      *grpc.Server
	rateLimiter     *RateLimiter
	resourceMonitor *ResourceMonitor
}

// New constructs a daemon with the provided configuration.
func New(cfg *config.Config, logger zerolog.Logger, opts Options) (*Daemon, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	if opts.Hostname == "" {
		opts.Hostname = "127.0.0.1"
	}
	if opts.Port == 0 {
		opts.Port = DefaultPort
	}

	// Create the gRPC service implementation
	server := NewServer(logger, WithVersion(opts.Version))

	// Create rate limiter with options
	var rlOpts []RateLimiterOption
	if opts.CustomRateLimits != nil {
		rlOpts = append(rlOpts, WithMethodLimits(opts.CustomRateLimits))
	}
	if opts.GlobalRateLimit != nil {
		rlOpts = append(rlOpts, WithGlobalLimit(*opts.GlobalRateLimit))
	}
	if opts.RateLimitEnabled != nil {
		rlOpts = append(rlOpts, WithEnabled(*opts.RateLimitEnabled))
	}
	rateLimiter := NewRateLimiter(rlOpts...)

	// Create the gRPC server with rate limiting interceptors
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(rateLimiter.UnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(rateLimiter.StreamServerInterceptor()),
	)
	swarmdv1.RegisterSwarmdServiceServer(grpcServer, server)

	// Store rate limiter reference in server for status reporting
	server.SetRateLimiter(rateLimiter)

	logger.Info().
		Bool("rate_limiting_enabled", rateLimiter.IsEnabled()).
		Msg("rate limiter configured")

	// Create resource monitor if enabled (default: enabled)
	var resourceMonitor *ResourceMonitor
	resourceMonitorEnabled := opts.ResourceMonitorEnabled == nil || *opts.ResourceMonitorEnabled
	if resourceMonitorEnabled {
		rmOpts := []ResourceMonitorOption{
			WithViolationCallback(func(v ResourceViolation) {
				// Publish resource violation event
				server.publishResourceViolation(v)
			}),
			WithKillCallback(func(agentID, reason string) {
				// Kill the agent via the server
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_, err := server.KillAgent(ctx, &swarmdv1.KillAgentRequest{
					AgentId: agentID,
					Force:   true,
				})
				if err != nil {
					logger.Warn().Err(err).Str("agent_id", agentID).Msg("failed to kill agent due to resource violation")
				}
			}),
		}
		diskConfig := DefaultDiskMonitorConfig()
		if opts.DiskMonitorConfig != nil {
			diskConfig = *opts.DiskMonitorConfig
		} else if cfg.Global.DataDir != "" {
			diskConfig.Path = cfg.Global.DataDir
		}
		rmOpts = append(rmOpts, WithDiskMonitorConfig(diskConfig))
		if opts.DefaultResourceLimits != nil {
			rmOpts = append(rmOpts, WithDefaultLimits(*opts.DefaultResourceLimits))
		}
		resourceMonitor = NewResourceMonitor(logger, server, rmOpts...)
		server.SetResourceMonitor(resourceMonitor)

		logger.Info().
			Bool("resource_monitoring_enabled", true).
			Msg("resource monitor configured")
	}

	return &Daemon{
		cfg:             cfg,
		logger:          logger,
		opts:            opts,
		server:          server,
		grpcServer:      grpcServer,
		rateLimiter:     rateLimiter,
		resourceMonitor: resourceMonitor,
	}, nil
}

// Run starts the gRPC server and blocks until the context is canceled.
func (d *Daemon) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is required")
	}

	bindAddr := d.bindAddr()
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", bindAddr, err)
	}

	d.logger.Info().
		Str("bind", bindAddr).
		Str("version", d.opts.Version).
		Msg("swarmd gRPC server starting")

	// Start resource monitor if configured
	if d.resourceMonitor != nil {
		d.resourceMonitor.Start(ctx)
		defer d.resourceMonitor.Stop()
	}

	// Start gRPC server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := d.grpcServer.Serve(listener); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		d.logger.Info().Msg("swarmd shutting down...")
		d.grpcServer.GracefulStop()
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("gRPC server error: %w", err)
		}
	}

	d.logger.Info().Msg("swarmd shutdown complete")
	return nil
}

func (d *Daemon) bindAddr() string {
	return net.JoinHostPort(d.opts.Hostname, strconv.Itoa(d.opts.Port))
}

// Server returns the underlying gRPC service implementation.
// Useful for testing.
func (d *Daemon) Server() *Server {
	return d.server
}

// RateLimiter returns the rate limiter.
// Useful for testing and runtime configuration.
func (d *Daemon) RateLimiter() *RateLimiter {
	return d.rateLimiter
}

// ResourceMonitor returns the resource monitor.
// Useful for testing and runtime configuration.
func (d *Daemon) ResourceMonitor() *ResourceMonitor {
	return d.resourceMonitor
}
