// Package forged provides the daemon scaffolding for the Forge node service.
package forged

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	forgedv1 "github.com/tOgg1/forge/gen/forged/v1"
	"github.com/tOgg1/forge/internal/adapters"
	"github.com/tOgg1/forge/internal/config"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/queue"
	"github.com/tOgg1/forge/internal/state"
	"github.com/tOgg1/forge/internal/tmux"
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

	// DisableDatabase skips database initialization (for testing).
	DisableDatabase bool
}

// SchedulerRunner provides lifecycle management for an external scheduler.
// This interface allows the scheduler to be created outside of the forged package
// (avoiding import cycles) while still being managed by the daemon's Run loop.
type SchedulerRunner interface {
	// Start begins the scheduler's background processing loop.
	Start(ctx context.Context) error
	// Stop halts the scheduler and waits for in-progress work to complete.
	Stop() error
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
	mailServer      *mailServer
	mailListeners   []mailListener
	mailRelay       *mailRelayManager

	// Database and repositories
	database  *db.DB
	agentRepo *db.AgentRepository
	queueRepo *db.QueueRepository
	wsRepo    *db.WorkspaceRepository
	eventRepo *db.EventRepository
	nodeRepo  *db.NodeRepository
	portRepo  *db.PortRepository

	// Services (initialized when database is available)
	tmuxClient   *tmux.Client
	registry     *adapters.Registry
	stateEngine  *state.Engine
	queueService *queue.Service
	statePoller  *state.Poller
	eventWatcher *adapters.OpenCodeEventWatcher

	// Scheduler (set externally via SetScheduler to avoid import cycles)
	scheduler SchedulerRunner
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

	// Initialize database and repositories (unless disabled for testing)
	var (
		database  *db.DB
		agentRepo *db.AgentRepository
		queueRepo *db.QueueRepository
		wsRepo    *db.WorkspaceRepository
		eventRepo *db.EventRepository
		nodeRepo  *db.NodeRepository
		portRepo  *db.PortRepository
	)

	if !opts.DisableDatabase {
		// Determine database path
		dbPath := cfg.Database.Path
		if dbPath == "" {
			dbPath = filepath.Join(cfg.Global.DataDir, "forge.db")
		}

		// Open database
		dbCfg := db.Config{
			Path:          dbPath,
			MaxOpenConns:  cfg.Database.MaxConnections,
			BusyTimeoutMs: cfg.Database.BusyTimeoutMs,
		}
		if dbCfg.MaxOpenConns == 0 {
			dbCfg.MaxOpenConns = 10
		}
		if dbCfg.BusyTimeoutMs == 0 {
			dbCfg.BusyTimeoutMs = 5000
		}

		var err error
		database, err = db.Open(dbCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}

		// Run migrations
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := database.Migrate(ctx); err != nil {
			database.Close()
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}

		// Create repositories
		agentRepo = db.NewAgentRepository(database)
		queueRepo = db.NewQueueRepository(database)
		wsRepo = db.NewWorkspaceRepository(database)
		eventRepo = db.NewEventRepository(database)
		nodeRepo = db.NewNodeRepository(database)
		portRepo = db.NewPortRepository(database)

		logger.Info().
			Str("path", dbPath).
			Msg("database initialized")
	}

	// Initialize tmux client and adapter registry
	tmuxClient := tmux.NewLocalClient()
	registry := adapters.NewRegistry()

	// Initialize services (only if database is available)
	var (
		stateEngine  *state.Engine
		queueService *queue.Service
		statePoller  *state.Poller
		eventWatcher *adapters.OpenCodeEventWatcher
	)
	if !opts.DisableDatabase {
		stateEngine = state.NewEngine(agentRepo, eventRepo, tmuxClient, registry)
		queueService = queue.NewService(queueRepo)
		statePoller = state.NewPoller(state.DefaultPollerConfig(), stateEngine, agentRepo)

		// Create SSE event watcher for OpenCode agents with state update handler
		// When SSE events indicate state changes, update the database via StateEngine
		onStateUpdate := func(agentID string, newState models.AgentState, info models.StateInfo) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := stateEngine.UpdateState(ctx, agentID, newState, info, nil, nil); err != nil {
				logger.Warn().
					Err(err).
					Str("agent_id", agentID).
					Str("new_state", string(newState)).
					Msg("failed to update agent state from SSE event")
				return
			}

			logger.Debug().
				Str("agent_id", agentID).
				Str("new_state", string(newState)).
				Str("confidence", string(info.Confidence)).
				Str("reason", info.Reason).
				Msg("agent state updated from SSE event")
		}
		eventWatcher = adapters.NewOpenCodeEventWatcher(adapters.DefaultEventWatcherConfig(), onStateUpdate)

		logger.Info().Msg("services initialized")
	}

	// Create the gRPC service implementation
	server := NewServer(logger, WithVersion(opts.Version))
	mailServer := newMailServer(logger)
	var mailRelay *mailRelayManager
	if cfg.Mail.Relay.Enabled && len(cfg.Mail.Relay.Peers) > 0 {
		mailRelay = newMailRelayManager(
			logger,
			mailServer,
			mailServer.host,
			cfg.Mail.Relay.Peers,
			cfg.Mail.Relay.DialTimeout,
			cfg.Mail.Relay.ReconnectInterval,
		)
	}

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
	forgedv1.RegisterForgedServiceServer(grpcServer, server)

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
				_, err := server.KillAgent(ctx, &forgedv1.KillAgentRequest{
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
		mailServer:      mailServer,
		database:        database,
		mailRelay:       mailRelay,
		agentRepo:       agentRepo,
		queueRepo:       queueRepo,
		wsRepo:          wsRepo,
		eventRepo:       eventRepo,
		nodeRepo:        nodeRepo,
		portRepo:        portRepo,
		tmuxClient:      tmuxClient,
		registry:        registry,
		stateEngine:     stateEngine,
		queueService:    queueService,
		statePoller:     statePoller,
		eventWatcher:    eventWatcher,
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
		Msg("forged gRPC server starting")

	// Start resource monitor if configured
	if d.resourceMonitor != nil {
		d.resourceMonitor.Start(ctx)
	}

	// Start state poller if available
	if d.statePoller != nil {
		if err := d.statePoller.Start(ctx); err != nil {
			d.logger.Error().Err(err).Msg("failed to start state poller")
			return fmt.Errorf("failed to start state poller: %w", err)
		}
		d.logger.Info().Msg("state poller started")
	}

	// Start scheduler if registered
	if d.scheduler != nil {
		if err := d.scheduler.Start(ctx); err != nil {
			d.logger.Error().Err(err).Msg("failed to start scheduler")
			// Stop poller before returning
			if d.statePoller != nil {
				if stopErr := d.statePoller.Stop(); stopErr != nil {
					d.logger.Warn().Err(stopErr).Msg("failed to stop state poller")
				}
			}
			return fmt.Errorf("failed to start scheduler: %w", err)
		}
		d.logger.Info().Msg("scheduler started")
	}

	// Start gRPC server in a goroutine
	errCh := make(chan error, 8)
	go func() {
		if err := d.grpcServer.Serve(listener); err != nil {
			errCh <- err
		}
	}()

	if err := d.startMailServers(errCh); err != nil {
		d.shutdown()
		return err
	}
	if err := d.startMailRelay(ctx); err != nil {
		d.shutdown()
		return err
	}

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		d.logger.Info().Msg("forged shutting down...")
		d.shutdown()
	case err := <-errCh:
		if err != nil {
			d.shutdown()
			return fmt.Errorf("server error: %w", err)
		}
	}

	d.logger.Info().Msg("forged shutdown complete")
	return nil
}

// shutdown performs ordered cleanup of all daemon components.
// Shutdown order: scheduler -> event watcher -> state poller -> gRPC server -> mail servers -> resource monitor -> database
func (d *Daemon) shutdown() {
	// 1. Stop scheduler first (waits for in-progress dispatches)
	if d.scheduler != nil {
		d.logger.Debug().Msg("stopping scheduler...")
		if err := d.scheduler.Stop(); err != nil {
			d.logger.Warn().Err(err).Msg("scheduler stop returned error")
		}
		d.logger.Debug().Msg("scheduler stopped")
	}

	// 2. Stop SSE event watcher (closes all SSE connections)
	if d.eventWatcher != nil {
		d.logger.Debug().Msg("stopping event watcher...")
		d.eventWatcher.UnwatchAll()
		d.logger.Debug().Msg("event watcher stopped")
	}

	// 3. Stop state poller
	if d.statePoller != nil {
		d.logger.Debug().Msg("stopping state poller...")
		if err := d.statePoller.Stop(); err != nil {
			d.logger.Warn().Err(err).Msg("state poller stop returned error")
		}
		d.logger.Debug().Msg("state poller stopped")
	}

	// 4. Stop gRPC server
	d.logger.Debug().Msg("stopping gRPC server...")
	d.grpcServer.GracefulStop()
	d.logger.Debug().Msg("gRPC server stopped")

	// 5. Stop mail servers
	d.logger.Debug().Msg("stopping mail servers...")
	if d.mailRelay != nil {
		d.logger.Debug().Msg("stopping mail relay...")
		d.mailRelay.Stop()
		d.logger.Debug().Msg("mail relay stopped")
	}
	d.shutdownMailServers()
	d.logger.Debug().Msg("mail servers stopped")

	// 6. Stop resource monitor
	if d.resourceMonitor != nil {
		d.logger.Debug().Msg("stopping resource monitor...")
		d.resourceMonitor.Stop()
		d.logger.Debug().Msg("resource monitor stopped")
	}

	// 7. Close database
	if d.database != nil {
		d.logger.Debug().Msg("closing database...")
		if err := d.database.Close(); err != nil {
			d.logger.Warn().Err(err).Msg("failed to close database")
		}
		d.logger.Debug().Msg("database closed")
	}
}

func (d *Daemon) bindAddr() string {
	return net.JoinHostPort(d.opts.Hostname, strconv.Itoa(d.opts.Port))
}

// Server returns the underlying gRPC service implementation.
// Useful for testing.
func (d *Daemon) Server() *Server {
	return d.server
}

// Config returns the daemon configuration.
func (d *Daemon) Config() *config.Config {
	return d.cfg
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

// Database returns the database connection.
// Useful for testing and service initialization.
func (d *Daemon) Database() *db.DB {
	return d.database
}

// AgentRepository returns the agent repository.
func (d *Daemon) AgentRepository() *db.AgentRepository {
	return d.agentRepo
}

// QueueRepository returns the queue repository.
func (d *Daemon) QueueRepository() *db.QueueRepository {
	return d.queueRepo
}

// WorkspaceRepository returns the workspace repository.
func (d *Daemon) WorkspaceRepository() *db.WorkspaceRepository {
	return d.wsRepo
}

// EventRepository returns the event repository.
func (d *Daemon) EventRepository() *db.EventRepository {
	return d.eventRepo
}

// NodeRepository returns the node repository.
func (d *Daemon) NodeRepository() *db.NodeRepository {
	return d.nodeRepo
}

// PortRepository returns the port repository.
func (d *Daemon) PortRepository() *db.PortRepository {
	return d.portRepo
}

// TmuxClient returns the tmux client.
func (d *Daemon) TmuxClient() *tmux.Client {
	return d.tmuxClient
}

// AdapterRegistry returns the adapter registry.
func (d *Daemon) AdapterRegistry() *adapters.Registry {
	return d.registry
}

// StateEngine returns the state engine.
func (d *Daemon) StateEngine() *state.Engine {
	return d.stateEngine
}

// QueueService returns the queue service.
func (d *Daemon) QueueService() *queue.Service {
	return d.queueService
}

// StatePoller returns the state poller.
func (d *Daemon) StatePoller() *state.Poller {
	return d.statePoller
}

// EventWatcher returns the OpenCode SSE event watcher.
func (d *Daemon) EventWatcher() *adapters.OpenCodeEventWatcher {
	return d.eventWatcher
}

// SetScheduler registers an external scheduler for lifecycle management.
// The scheduler will be started when Run() is called and stopped on shutdown.
// This method must be called before Run().
func (d *Daemon) SetScheduler(s SchedulerRunner) {
	d.scheduler = s
}

// Scheduler returns the registered scheduler (may be nil if not set).
func (d *Daemon) Scheduler() SchedulerRunner {
	return d.scheduler
}

// Close releases all daemon resources including the database connection.
func (d *Daemon) Close() error {
	if d.database != nil {
		if err := d.database.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
	}
	return nil
}
