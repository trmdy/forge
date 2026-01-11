// Package workspace provides helpers for workspace lifecycle management.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/agentmail"
	"github.com/tOgg1/forge/internal/beads"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/events"
	"github.com/tOgg1/forge/internal/logging"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/node"
	"github.com/tOgg1/forge/internal/tmux"
)

// Service errors.
var (
	ErrWorkspaceNotFound      = errors.New("workspace not found")
	ErrWorkspaceAlreadyExists = errors.New("workspace already exists")
	ErrNodeNotFound           = errors.New("node not found")
	ErrTmuxSessionFailed      = errors.New("failed to create tmux session")
	ErrRepoValidationFailed   = errors.New("repository validation failed")
)

const (
	agentShutdownGracePeriod  = 5 * time.Second
	agentShutdownPollInterval = 200 * time.Millisecond
)

const (
	pulseWindow            = 60 * time.Minute
	pulseBucketCount       = 12
	pulseBucketDuration    = pulseWindow / pulseBucketCount
	pulseMaxEventsPerAgent = 1000
	pulseMaxCommits        = 500
	pulseLevels            = ".:-=+*#@"
)

// Service manages workspace operations.
type Service struct {
	repo        *db.WorkspaceRepository
	nodeService *node.Service
	agentRepo   *db.AgentRepository
	eventRepo   *db.EventRepository
	publisher   events.Publisher
	tmuxFactory func() *tmux.Client
	logger      zerolog.Logger
}

// ServiceOption configures a WorkspaceService.
type ServiceOption func(*Service)

// WithPublisher sets the event publisher for the service.
func WithPublisher(publisher events.Publisher) ServiceOption {
	return func(s *Service) {
		s.publisher = publisher
	}
}

// WithEventRepository sets the event repository for pulse calculations.
func WithEventRepository(eventRepo *db.EventRepository) ServiceOption {
	return func(s *Service) {
		s.eventRepo = eventRepo
	}
}

// WithTmuxClientFactory overrides the tmux client factory.
func WithTmuxClientFactory(factory func() *tmux.Client) ServiceOption {
	return func(s *Service) {
		s.tmuxFactory = factory
	}
}

// NewService creates a new WorkspaceService.
func NewService(repo *db.WorkspaceRepository, nodeService *node.Service, agentRepo *db.AgentRepository, opts ...ServiceOption) *Service {
	s := &Service{
		repo:        repo,
		nodeService: nodeService,
		agentRepo:   agentRepo,
		tmuxFactory: tmux.NewLocalClient,
		logger:      logging.Component("workspace"),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// CreateWorkspaceInput contains the parameters for creating a workspace.
type CreateWorkspaceInput struct {
	// NodeID is the node where the workspace will be created.
	// If empty, uses the local node.
	NodeID string

	// RepoPath is the absolute path to the repository.
	RepoPath string

	// Name is an optional human-friendly name.
	// If empty, derived from the repo path.
	Name string

	// TmuxSession is an optional session name.
	// If empty, auto-generated from repo path.
	TmuxSession string

	// CreateTmuxSession indicates whether to create a new tmux session.
	CreateTmuxSession bool
}

// CreateWorkspace creates a new workspace for a repository.
func (s *Service) CreateWorkspace(ctx context.Context, input CreateWorkspaceInput) (*models.Workspace, error) {
	s.logger.Debug().
		Str("node_id", input.NodeID).
		Str("repo_path", input.RepoPath).
		Msg("creating workspace")

	// Validate repo path exists
	if err := ValidateRepoPath(input.RepoPath); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRepoValidationFailed, err)
	}

	// Get or default node
	nodeID := input.NodeID
	if nodeID == "" {
		// Use local node - it must exist
		nodes, err := s.nodeService.ListNodes(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list nodes: %w", err)
		}
		for _, n := range nodes {
			if n.IsLocal {
				nodeID = n.ID
				break
			}
		}
		if nodeID == "" {
			return nil, fmt.Errorf("no local node found")
		}
	}

	// Verify node exists
	_, err := s.nodeService.GetNode(ctx, nodeID)
	if err != nil {
		if errors.Is(err, node.ErrNodeNotFound) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	// Generate tmux session name if not provided
	tmuxSession := input.TmuxSession
	if tmuxSession == "" {
		tmuxSession, err = GenerateTmuxSessionName("forge", input.RepoPath)
		if err != nil {
			return nil, fmt.Errorf("failed to generate tmux session name: %w", err)
		}
	}

	// Detect git info
	gitInfo, err := DetectGitInfo(input.RepoPath)
	if err != nil {
		s.logger.Warn().Err(err).Msg("failed to detect git info")
		// Not fatal - continue without git info
	}

	// Create workspace record
	workspace := &models.Workspace{
		Name:        input.Name,
		NodeID:      nodeID,
		RepoPath:    input.RepoPath,
		TmuxSession: tmuxSession,
		Status:      models.WorkspaceStatusActive,
		GitInfo:     gitInfo,
	}

	if workspace.Name == "" {
		workspace.Name = tmuxSession
	}

	// Create tmux session if requested
	if input.CreateTmuxSession {
		if err := s.createTmuxSession(ctx, workspace); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrTmuxSessionFailed, err)
		}
	}

	// Persist to database
	if err := s.repo.Create(ctx, workspace); err != nil {
		if errors.Is(err, db.ErrWorkspaceAlreadyExists) {
			return nil, ErrWorkspaceAlreadyExists
		}
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	s.logger.Info().
		Str("workspace_id", workspace.ID).
		Str("name", workspace.Name).
		Str("tmux_session", workspace.TmuxSession).
		Msg("workspace created")

	// Emit event
	s.publishEvent(ctx, models.EventTypeWorkspaceCreated, workspace.ID, nil)

	return workspace, nil
}

// ImportWorkspaceInput contains the parameters for importing an existing tmux session.
type ImportWorkspaceInput struct {
	// NodeID is the node where the session exists.
	NodeID string

	// TmuxSession is the name of the existing tmux session.
	TmuxSession string

	// Name is an optional human-friendly name.
	Name string

	// RepoPath overrides repo path detection from tmux panes.
	RepoPath string
}

// ImportWorkspace imports an existing tmux session as a workspace.
func (s *Service) ImportWorkspace(ctx context.Context, input ImportWorkspaceInput) (*models.Workspace, error) {
	s.logger.Debug().
		Str("node_id", input.NodeID).
		Str("tmux_session", input.TmuxSession).
		Msg("importing workspace")

	// Verify node exists
	nodeObj, err := s.nodeService.GetNode(ctx, input.NodeID)
	if err != nil {
		if errors.Is(err, node.ErrNodeNotFound) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	repoPath := input.RepoPath
	if repoPath == "" {
		// Get working directory from tmux session
		repoPath, err = s.getTmuxSessionWorkingDir(ctx, nodeObj, input.TmuxSession)
		if err != nil {
			return nil, fmt.Errorf("failed to get tmux session working directory: %w", err)
		}
	} else if err := ValidateRepoPath(repoPath); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRepoValidationFailed, err)
	}

	// Detect git info
	gitInfo, err := DetectGitInfo(repoPath)
	if err != nil {
		s.logger.Warn().Err(err).Msg("failed to detect git info")
	}

	// Create workspace record
	workspace := &models.Workspace{
		Name:        input.Name,
		NodeID:      input.NodeID,
		RepoPath:    repoPath,
		TmuxSession: input.TmuxSession,
		Status:      models.WorkspaceStatusActive,
		GitInfo:     gitInfo,
	}

	if workspace.Name == "" {
		workspace.Name = input.TmuxSession
	}

	// Persist to database
	if err := s.repo.Create(ctx, workspace); err != nil {
		if errors.Is(err, db.ErrWorkspaceAlreadyExists) {
			return nil, ErrWorkspaceAlreadyExists
		}
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	s.logger.Info().
		Str("workspace_id", workspace.ID).
		Str("name", workspace.Name).
		Str("tmux_session", workspace.TmuxSession).
		Msg("workspace imported")

	// Emit event
	s.publishEvent(ctx, models.EventTypeWorkspaceImported, workspace.ID, nil)

	// Best-effort discovery of existing agents in the session.
	if s.agentRepo != nil && nodeObj.IsLocal {
		if err := s.discoverAgentsInSession(ctx, workspace); err != nil {
			s.logger.Warn().Err(err).Str("workspace_id", workspace.ID).Msg("failed to discover agents in session")
		}
	}

	return workspace, nil
}

// ListWorkspacesOptions contains options for listing workspaces.
type ListWorkspacesOptions struct {
	// NodeID filters by node.
	NodeID string

	// Status filters by status.
	Status *models.WorkspaceStatus

	// IncludeAgentCounts includes agent counts in results.
	IncludeAgentCounts bool
}

// ListWorkspaces returns all workspaces matching the options.
func (s *Service) ListWorkspaces(ctx context.Context, opts ListWorkspacesOptions) ([]*models.Workspace, error) {
	var workspaces []*models.Workspace
	var err error

	if opts.NodeID != "" {
		workspaces, err = s.repo.ListByNode(ctx, opts.NodeID)
	} else if opts.Status != nil {
		workspaces, err = s.repo.ListByStatus(ctx, *opts.Status)
	} else if opts.IncludeAgentCounts {
		workspaces, err = s.repo.ListWithAgentCounts(ctx)
	} else {
		workspaces, err = s.repo.List(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	// Populate agent counts if not already included
	if !opts.IncludeAgentCounts {
		for _, ws := range workspaces {
			count, err := s.repo.GetAgentCount(ctx, ws.ID)
			if err != nil {
				s.logger.Warn().Err(err).Str("workspace_id", ws.ID).Msg("failed to get agent count")
				continue
			}
			ws.AgentCount = count
		}
	}

	return workspaces, nil
}

// GetWorkspace retrieves a workspace by ID.
func (s *Service) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	workspace, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrWorkspaceNotFound) {
			return nil, ErrWorkspaceNotFound
		}
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	// Populate agent count
	count, err := s.repo.GetAgentCount(ctx, workspace.ID)
	if err != nil {
		s.logger.Warn().Err(err).Str("workspace_id", workspace.ID).Msg("failed to get agent count")
	} else {
		workspace.AgentCount = count
	}

	return workspace, nil
}

// WorkspaceStatusResult contains comprehensive status information.
type WorkspaceStatusResult struct {
	// Workspace is the base workspace info.
	Workspace *models.Workspace

	// TmuxActive indicates if the tmux session is active.
	TmuxActive bool

	// NodeOnline indicates if the node is reachable.
	NodeOnline bool

	// GitInfo is the current git repository state.
	GitInfo *models.GitInfo

	// BeadsDetected indicates if the workspace contains a .beads directory.
	BeadsDetected bool

	// AgentMailDetected indicates if the workspace contains Agent Mail MCP config.
	AgentMailDetected bool

	// ActiveAgents is the number of active agents.
	ActiveAgents int

	// IdleAgents is the number of idle agents.
	IdleAgents int

	// BlockedAgents is the number of blocked agents.
	BlockedAgents int

	// Alerts contains current alerts.
	Alerts []models.Alert

	// Pulse captures recent activity for the workspace.
	Pulse *WorkspacePulse
}

// WorkspacePulse summarizes recent workspace activity.
type WorkspacePulse struct {
	Sparkline     string `json:"sparkline"`
	Buckets       []int  `json:"buckets"`
	WindowMinutes int    `json:"window_minutes"`
	BucketMinutes int    `json:"bucket_minutes"`
	Total         int    `json:"total"`
}

// GetWorkspaceStatus retrieves comprehensive status for a workspace.
func (s *Service) GetWorkspaceStatus(ctx context.Context, id string) (*WorkspaceStatusResult, error) {
	workspace, err := s.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}

	result := &WorkspaceStatusResult{
		Workspace: workspace,
		GitInfo:   workspace.GitInfo,
	}

	// Check if node is online
	nodeObj, err := s.nodeService.GetNode(ctx, workspace.NodeID)
	if err == nil {
		result.NodeOnline = nodeObj.Status == models.NodeStatusOnline
	}

	// Check if tmux session is active
	if result.NodeOnline {
		result.TmuxActive = s.isTmuxSessionActive(ctx, nodeObj, workspace.TmuxSession)
	}

	// Refresh git info
	if workspace.RepoPath != "" {
		if gitInfo, err := DetectGitInfo(workspace.RepoPath); err == nil {
			result.GitInfo = gitInfo
			// Update stored git info
			if err := s.repo.UpdateGitInfo(ctx, workspace.ID, gitInfo); err != nil {
				s.logger.Warn().Err(err).Msg("failed to update git info")
			}
		}
	}

	if workspace.RepoPath != "" {
		detected, err := beads.HasBeadsDir(workspace.RepoPath)
		if err != nil {
			s.logger.Warn().Err(err).Msg("failed to detect beads directory")
		} else {
			result.BeadsDetected = detected
		}
	}

	if workspace.RepoPath != "" {
		detected, err := agentmail.HasAgentMailConfig(workspace.RepoPath)
		if err != nil {
			s.logger.Warn().Err(err).Msg("failed to detect Agent Mail config")
		} else {
			result.AgentMailDetected = detected
		}
	}

	// Populate alerts from agent states when available.
	if s.agentRepo != nil {
		agents, err := s.agentRepo.ListByWorkspace(ctx, workspace.ID)
		if err != nil {
			s.logger.Warn().Err(err).Msg("failed to list agents for alerts")
		} else {
			result.Alerts = append(result.Alerts, BuildAlerts(agents)...)

			for _, agent := range agents {
				switch agent.State {
				case models.AgentStateWorking:
					result.ActiveAgents++
				case models.AgentStateIdle:
					result.IdleAgents++
				case models.AgentStateAwaitingApproval, models.AgentStateRateLimited, models.AgentStateError:
					result.BlockedAgents++
				}
			}
		}
	} else {
		// Fallback when agent repository isn't wired.
		result.ActiveAgents = workspace.AgentCount
	}

	result.Pulse = s.computeWorkspacePulse(ctx, workspace)

	return result, nil
}

// DeleteWorkspace removes a workspace from Forge.
// It does not terminate the tmux session or delete files.
func (s *Service) DeleteWorkspace(ctx context.Context, id string) error {
	s.logger.Debug().Str("workspace_id", id).Msg("deleting workspace")

	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, db.ErrWorkspaceNotFound) {
			return ErrWorkspaceNotFound
		}
		return fmt.Errorf("failed to delete workspace: %w", err)
	}

	s.logger.Info().Str("workspace_id", id).Msg("workspace deleted")
	return nil
}

// UpdateWorkspace updates a workspace's configuration.
func (s *Service) UpdateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	s.logger.Debug().Str("workspace_id", workspace.ID).Msg("updating workspace")

	if err := s.repo.Update(ctx, workspace); err != nil {
		if errors.Is(err, db.ErrWorkspaceNotFound) {
			return ErrWorkspaceNotFound
		}
		return fmt.Errorf("failed to update workspace: %w", err)
	}

	s.logger.Info().Str("workspace_id", workspace.ID).Msg("workspace updated")
	return nil
}

// RefreshGitInfo updates the git information for a workspace.
func (s *Service) RefreshGitInfo(ctx context.Context, id string) (*models.GitInfo, error) {
	workspace, err := s.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}

	gitInfo, err := DetectGitInfo(workspace.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect git info: %w", err)
	}

	if err := s.repo.UpdateGitInfo(ctx, id, gitInfo); err != nil {
		return nil, fmt.Errorf("failed to update git info: %w", err)
	}

	return gitInfo, nil
}

// AttachWorkspace returns the tmux attach command for a workspace.
func (s *Service) AttachWorkspace(ctx context.Context, id string) (string, error) {
	workspace, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrWorkspaceNotFound) {
			return "", ErrWorkspaceNotFound
		}
		return "", fmt.Errorf("failed to get workspace: %w", err)
	}

	if workspace.TmuxSession == "" {
		return "", fmt.Errorf("workspace %s has no tmux session", workspace.ID)
	}

	return fmt.Sprintf("tmux attach -t %s", workspace.TmuxSession), nil
}

// UnmanageWorkspace removes a workspace record while leaving tmux intact.
func (s *Service) UnmanageWorkspace(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, db.ErrWorkspaceNotFound) {
			return ErrWorkspaceNotFound
		}
		return fmt.Errorf("failed to unmanage workspace: %w", err)
	}

	s.logger.Info().Str("workspace_id", id).Msg("workspace unmanaged")

	// Emit event
	s.publishEvent(ctx, models.EventTypeWorkspaceUnmanaged, id, nil)

	return nil
}

// DestroyWorkspace kills the tmux session (if local) and removes the workspace record.
func (s *Service) DestroyWorkspace(ctx context.Context, id string) error {
	workspace, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrWorkspaceNotFound) {
			return ErrWorkspaceNotFound
		}
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	nodeObj, err := s.nodeService.GetNode(ctx, workspace.NodeID)
	if err != nil {
		if errors.Is(err, node.ErrNodeNotFound) {
			return ErrNodeNotFound
		}
		return fmt.Errorf("failed to get node: %w", err)
	}

	if !nodeObj.IsLocal {
		return fmt.Errorf("remote workspace destroy not yet implemented")
	}

	if workspace.TmuxSession != "" {
		client := s.tmuxClient()
		exists, err := client.HasSession(ctx, workspace.TmuxSession)
		if err != nil {
			return fmt.Errorf("failed to check tmux session: %w", err)
		}
		if exists {
			s.gracefulShutdownAgents(ctx, client, workspace.ID, workspace.TmuxSession)
			if err := client.KillSession(ctx, workspace.TmuxSession); err != nil {
				return fmt.Errorf("failed to kill tmux session %s: %w", workspace.TmuxSession, err)
			}
		}
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, db.ErrWorkspaceNotFound) {
			return ErrWorkspaceNotFound
		}
		return fmt.Errorf("failed to delete workspace: %w", err)
	}

	s.logger.Info().
		Str("workspace_id", id).
		Str("tmux_session", workspace.TmuxSession).
		Msg("workspace destroyed")

	// Emit event
	s.publishEvent(ctx, models.EventTypeWorkspaceDestroyed, id, nil)

	return nil
}

func (s *Service) gracefulShutdownAgents(ctx context.Context, client *tmux.Client, workspaceID, session string) {
	if s.agentRepo == nil || strings.TrimSpace(session) == "" {
		return
	}

	agents, err := s.agentRepo.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.Warn().Err(err).Str("workspace_id", workspaceID).Msg("failed to list agents before destroy")
		return
	}
	if len(agents) == 0 {
		return
	}

	var targets []string
	interrupted := 0
	hasMismatchedSession := false

	for _, agent := range agents {
		pane := strings.TrimSpace(agent.TmuxPane)
		if pane == "" {
			continue
		}

		if err := client.SendInterrupt(ctx, pane); err != nil {
			s.logger.Warn().
				Err(err).
				Str("agent_id", agent.ID).
				Str("pane", pane).
				Msg("failed to interrupt agent before destroy")
			continue
		}

		interrupted++
		if strings.HasPrefix(pane, session+":") {
			targets = append(targets, pane)
		} else {
			hasMismatchedSession = true
		}
	}

	if interrupted == 0 || agentShutdownGracePeriod <= 0 {
		return
	}

	if len(targets) == 0 && !hasMismatchedSession {
		return
	}

	exited := s.waitForAgentsExit(ctx, client, session, targets, hasMismatchedSession)
	if !exited {
		s.logger.Debug().
			Str("workspace_id", workspaceID).
			Str("tmux_session", session).
			Msg("grace period elapsed before agents exited")
	}
}

func (s *Service) waitForAgentsExit(ctx context.Context, client *tmux.Client, session string, targets []string, forceTimeout bool) bool {
	deadline := time.NewTimer(agentShutdownGracePeriod)
	ticker := time.NewTicker(agentShutdownPollInterval)
	defer deadline.Stop()
	defer ticker.Stop()

	for {
		if !forceTimeout {
			allExited, err := s.allAgentPanesExited(ctx, client, session, targets)
			if err != nil {
				s.logger.Warn().Err(err).Str("tmux_session", session).Msg("failed to check agent panes during shutdown")
				return false
			}
			if allExited {
				return true
			}
		}

		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}

func (s *Service) allAgentPanesExited(ctx context.Context, client *tmux.Client, session string, targets []string) (bool, error) {
	if len(targets) == 0 {
		return true, nil
	}

	panes, err := client.ListPanes(ctx, session)
	if err != nil {
		return false, err
	}
	if len(panes) == 0 {
		return true, nil
	}

	paneTargets := make(map[string]struct{}, len(panes)*3)
	for _, pane := range panes {
		paneTargets[fmt.Sprintf("%s:%s", session, pane.ID)] = struct{}{}
		paneTargets[fmt.Sprintf("%s:%d.%d", session, pane.WindowIndex, pane.Index)] = struct{}{}
		paneTargets[fmt.Sprintf("%s:%d", session, pane.Index)] = struct{}{}
	}

	for _, target := range targets {
		if _, ok := paneTargets[target]; ok {
			return false, nil
		}
	}

	return true, nil
}

func (s *Service) computeWorkspacePulse(ctx context.Context, workspace *models.Workspace) *WorkspacePulse {
	if workspace == nil {
		return nil
	}

	now := time.Now().UTC()
	since := now.Add(-pulseWindow)
	buckets := make([]int, pulseBucketCount)
	total := 0
	hasSource := false

	if s.eventRepo != nil && s.agentRepo != nil {
		agents, err := s.agentRepo.ListByWorkspace(ctx, workspace.ID)
		if err != nil {
			s.logger.Warn().Err(err).Str("workspace_id", workspace.ID).Msg("failed to list agents for pulse")
		} else {
			entityType := models.EntityTypeAgent
			hasSource = true
			for _, agent := range agents {
				if agent == nil || strings.TrimSpace(agent.ID) == "" {
					continue
				}
				agentID := agent.ID
				page, err := s.eventRepo.Query(ctx, db.EventQuery{
					EntityType: &entityType,
					EntityID:   &agentID,
					Since:      &since,
					Limit:      pulseMaxEventsPerAgent,
				})
				if err != nil {
					s.logger.Warn().
						Err(err).
						Str("agent_id", agentID).
						Msg("failed to query events for pulse")
					continue
				}
				for _, event := range page.Events {
					if event == nil || !isPulseEventType(event.Type) {
						continue
					}
					if idx := pulseBucketIndex(event.Timestamp, since); idx >= 0 {
						buckets[idx]++
						total++
					}
				}
			}
		}
	}

	if strings.TrimSpace(workspace.RepoPath) != "" {
		commitTimes, err := listCommitTimesSince(workspace.RepoPath, since, pulseMaxCommits)
		if err != nil {
			s.logger.Debug().
				Err(err).
				Str("workspace_id", workspace.ID).
				Msg("unable to read commit history for pulse")
		} else {
			hasSource = true
			for _, ts := range commitTimes {
				if idx := pulseBucketIndex(ts, since); idx >= 0 {
					buckets[idx]++
					total++
				}
			}
		}
	}

	if !hasSource {
		return nil
	}

	return &WorkspacePulse{
		Sparkline:     buildPulseSparkline(buckets),
		Buckets:       buckets,
		WindowMinutes: int(pulseWindow.Minutes()),
		BucketMinutes: int(pulseBucketDuration.Minutes()),
		Total:         total,
	}
}

func isPulseEventType(eventType models.EventType) bool {
	switch eventType {
	case models.EventTypeAgentStateChanged,
		models.EventTypeMessageQueued,
		models.EventTypeMessageDispatched,
		models.EventTypeMessageCompleted,
		models.EventTypeMessageFailed:
		return true
	default:
		return false
	}
}

func pulseBucketIndex(timestamp, since time.Time) int {
	if timestamp.Before(since) {
		return -1
	}
	if pulseBucketDuration <= 0 {
		return -1
	}
	offset := timestamp.Sub(since)
	idx := int(offset / pulseBucketDuration)
	if idx < 0 {
		return -1
	}
	if idx >= pulseBucketCount {
		return pulseBucketCount - 1
	}
	return idx
}

func buildPulseSparkline(buckets []int) string {
	if len(buckets) == 0 {
		return ""
	}

	maxValue := 0
	for _, value := range buckets {
		if value > maxValue {
			maxValue = value
		}
	}

	levels := []rune(pulseLevels)
	if len(levels) == 0 {
		return ""
	}

	if maxValue == 0 {
		return strings.Repeat(string(levels[0]), len(buckets))
	}

	var builder strings.Builder
	builder.Grow(len(buckets))
	for _, value := range buckets {
		index := value * (len(levels) - 1) / maxValue
		if index < 0 {
			index = 0
		}
		if index >= len(levels) {
			index = len(levels) - 1
		}
		builder.WriteRune(levels[index])
	}

	return builder.String()
}

// createTmuxSession creates a new tmux session for a workspace.
func (s *Service) createTmuxSession(ctx context.Context, workspace *models.Workspace) error {
	// For now, only support local node
	// TODO: Support remote nodes via SSH

	client := s.tmuxClient()
	if err := client.NewSession(ctx, workspace.TmuxSession, workspace.RepoPath); err != nil {
		return err
	}

	// Create a dedicated window for agents, reserving pane 0 for human interaction.
	if err := client.NewWindow(ctx, workspace.TmuxSession, tmux.AgentWindowName, workspace.RepoPath); err != nil {
		return fmt.Errorf("failed to create agent window: %w", err)
	}

	return nil
}

// getTmuxSessionWorkingDir gets the working directory of a tmux session.
func (s *Service) getTmuxSessionWorkingDir(ctx context.Context, nodeObj *models.Node, sessionName string) (string, error) {
	if !nodeObj.IsLocal {
		return "", fmt.Errorf("remote node tmux inspection not yet implemented")
	}

	client := s.tmuxClient()

	// Use ListPanePaths to get the working directories for the session
	paths, err := client.ListPanePaths(ctx, sessionName)
	if err != nil {
		return "", err
	}

	if len(paths) == 0 {
		return "", fmt.Errorf("no panes found in tmux session %q", sessionName)
	}

	repoPath, err := detectRepoRootFromPaths(paths)
	if err != nil {
		return "", fmt.Errorf("failed to detect repo root from tmux panes: %w", err)
	}

	return repoPath, nil
}

func (s *Service) discoverAgentsInSession(ctx context.Context, workspace *models.Workspace) error {
	if workspace == nil || workspace.TmuxSession == "" {
		return nil
	}

	client := s.tmuxClient()
	panes, err := client.ListPanes(ctx, workspace.TmuxSession)
	if err != nil {
		return err
	}

	existing, err := s.agentRepo.ListByWorkspace(ctx, workspace.ID)
	if err != nil {
		return err
	}
	existingPanes := make(map[string]struct{}, len(existing))
	for _, agent := range existing {
		existingPanes[agent.TmuxPane] = struct{}{}
	}

	for _, pane := range panes {
		paneTarget := fmt.Sprintf("%s:%s", workspace.TmuxSession, pane.ID)
		if _, ok := existingPanes[paneTarget]; ok {
			continue
		}

		agentType, reason, evidence := detectAgentType(pane.Command, "")
		if agentType == "" {
			content, err := client.CapturePane(ctx, paneTarget, false)
			if err != nil {
				continue
			}
			agentType, reason, evidence = detectAgentType(pane.Command, content)
		}
		if agentType == "" {
			continue
		}

		now := time.Now().UTC()
		agent := &models.Agent{
			WorkspaceID: workspace.ID,
			Type:        agentType,
			TmuxPane:    paneTarget,
			State:       models.AgentStateIdle,
			StateInfo: models.StateInfo{
				State:      models.AgentStateIdle,
				Confidence: models.StateConfidenceLow,
				Reason:     reason,
				Evidence:   evidence,
				DetectedAt: now,
			},
			LastActivity: &now,
		}

		if err := s.agentRepo.Create(ctx, agent); err != nil {
			s.logger.Warn().Err(err).Str("tmux_pane", paneTarget).Msg("failed to record discovered agent")
			continue
		}

		s.logger.Info().
			Str("workspace_id", workspace.ID).
			Str("agent_id", agent.ID).
			Str("tmux_pane", agent.TmuxPane).
			Str("type", string(agent.Type)).
			Msg("discovered agent in tmux pane")
	}

	return nil
}

func detectAgentType(command, screen string) (models.AgentType, string, []string) {
	lowerCmd := strings.ToLower(strings.TrimSpace(command))
	if agentType, ok := agentTypeFromHint(lowerCmd); ok {
		return agentType, fmt.Sprintf("pane command detected: %s", lowerCmd), []string{lowerCmd}
	}

	lowerScreen := strings.ToLower(screen)
	if agentType, ok := agentTypeFromHint(lowerScreen); ok {
		return agentType, "agent signature detected in pane output", []string{agentTypeHint(agentType)}
	}

	return "", "", nil
}

func agentTypeFromHint(hint string) (models.AgentType, bool) {
	switch {
	case strings.Contains(hint, "opencode"):
		return models.AgentTypeOpenCode, true
	case strings.Contains(hint, "claude"):
		return models.AgentTypeClaudeCode, true
	case strings.Contains(hint, "codex"):
		return models.AgentTypeCodex, true
	case strings.Contains(hint, "gemini"):
		return models.AgentTypeGemini, true
	case strings.Contains(hint, "aider"):
		return models.AgentTypeGeneric, true
	default:
		return "", false
	}
}

func agentTypeHint(agentType models.AgentType) string {
	switch agentType {
	case models.AgentTypeOpenCode:
		return "opencode"
	case models.AgentTypeClaudeCode:
		return "claude"
	case models.AgentTypeCodex:
		return "codex"
	case models.AgentTypeGemini:
		return "gemini"
	default:
		return "agent"
	}
}

// isTmuxSessionActive checks if a tmux session is running.
func (s *Service) isTmuxSessionActive(ctx context.Context, nodeObj *models.Node, sessionName string) bool {
	if !nodeObj.IsLocal {
		// TODO: Check remote via SSH
		return false
	}

	client := s.tmuxClient()
	exists, err := client.HasSession(ctx, sessionName)
	if err != nil {
		return false
	}

	return exists
}

// publishEvent publishes an event if a publisher is configured.
func (s *Service) publishEvent(ctx context.Context, eventType models.EventType, workspaceID string, payload any) {
	if s.publisher == nil {
		return
	}

	event := &models.Event{
		Type:       eventType,
		EntityType: models.EntityTypeWorkspace,
		EntityID:   workspaceID,
	}

	s.publisher.Publish(ctx, event)
}

func (s *Service) tmuxClient() *tmux.Client {
	if s.tmuxFactory != nil {
		return s.tmuxFactory()
	}
	return tmux.NewLocalClient()
}
