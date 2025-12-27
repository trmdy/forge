// Package swarmd provides the daemon scaffolding for the Swarm node service.
package swarmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/opencode-ai/swarm/gen/swarmd/v1"
	"github.com/rs/zerolog"
)

// ResourceLimits defines the resource limits for an agent.
type ResourceLimits struct {
	// MaxMemoryBytes is the maximum memory usage in bytes (0 = unlimited).
	MaxMemoryBytes int64

	// MaxCPUPercent is the maximum CPU usage percentage (0 = unlimited).
	// Note: This is relative to one CPU core (100% = 1 core).
	MaxCPUPercent float64

	// GracePeriodSeconds is how long to allow limit violation before killing.
	GracePeriodSeconds int

	// WarnThresholdPercent triggers a warning at this percentage of the limit.
	// E.g., 80 means warn at 80% of MaxMemoryBytes.
	WarnThresholdPercent float64
}

// DefaultResourceLimits returns sensible defaults for resource limits.
func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{
		MaxMemoryBytes:       2 * 1024 * 1024 * 1024, // 2 GB
		MaxCPUPercent:        200,                    // 2 CPU cores
		GracePeriodSeconds:   30,
		WarnThresholdPercent: 80,
	}
}

// DiskMonitorConfig defines disk usage monitoring thresholds.
type DiskMonitorConfig struct {
	// Path is the filesystem path to monitor.
	Path string

	// WarnPercent triggers warning at or above this percent (0 = disabled).
	WarnPercent float64

	// CriticalPercent triggers critical state at or above this percent (0 = disabled).
	CriticalPercent float64

	// ResumePercent resumes paused agents when usage drops below this percent (0 = defaults to WarnPercent).
	ResumePercent float64

	// PauseAgents pauses agent processes when disk is critically full.
	PauseAgents bool
}

// DefaultDiskMonitorConfig returns sensible defaults for disk monitoring.
func DefaultDiskMonitorConfig() DiskMonitorConfig {
	return DiskMonitorConfig{
		Path:            "/",
		WarnPercent:     85,
		CriticalPercent: 95,
		ResumePercent:   90,
		PauseAgents:     false,
	}
}

// DiskUsage represents filesystem usage.
type DiskUsage struct {
	Path        string
	TotalBytes  uint64
	FreeBytes   uint64
	UsedBytes   uint64
	UsedPercent float64
}

type diskUsageLevel int

const (
	diskUsageOK diskUsageLevel = iota
	diskUsageWarn
	diskUsageCritical
)

type diskMonitorState struct {
	level         diskUsageLevel
	lastUsage     DiskUsage
	lastCheckedAt time.Time
	pausedAgents  map[string]int
}

// ResourceUsage represents the current resource usage of an agent.
type ResourceUsage struct {
	PID           int
	MemoryBytes   int64
	CPUPercent    float64
	MeasuredAt    time.Time
	ViolationTime *time.Time // When limit was first exceeded, nil if within limits
}

// agentResourceState tracks resource monitoring state for an agent.
type agentResourceState struct {
	agentID       string
	workspaceID   string
	pid           int
	limits        ResourceLimits
	usage         ResourceUsage
	violationTime *time.Time // When limit violation started
	warningSent   bool       // Whether warning has been sent for current violation
}

// ResourceViolation represents a resource limit violation.
type ResourceViolation struct {
	AgentID       string
	WorkspaceID   string
	PID           int
	ViolationType string // "memory", "cpu"
	CurrentValue  float64
	LimitValue    float64
	Duration      time.Duration
	Severity      string // "warning", "critical"
}

// ResourceMonitor tracks and enforces resource limits for agents.
type ResourceMonitor struct {
	logger   zerolog.Logger
	server   *Server
	interval time.Duration

	mu     sync.RWMutex
	agents map[string]*agentResourceState // keyed by agent ID
	limits ResourceLimits                 // default limits

	diskConfig    DiskMonitorConfig
	diskState     diskMonitorState
	diskUsageFunc func(string) (DiskUsage, error)

	// Callbacks for violations
	onViolation func(violation ResourceViolation)
	onKill      func(agentID string, reason string)

	// Control
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ResourceMonitorOption configures the ResourceMonitor.
type ResourceMonitorOption func(*ResourceMonitor)

// WithMonitorInterval sets the monitoring interval.
func WithMonitorInterval(d time.Duration) ResourceMonitorOption {
	return func(rm *ResourceMonitor) {
		rm.interval = d
	}
}

// WithDefaultLimits sets the default resource limits.
func WithDefaultLimits(limits ResourceLimits) ResourceMonitorOption {
	return func(rm *ResourceMonitor) {
		rm.limits = limits
	}
}

// WithViolationCallback sets the callback for resource violations.
func WithViolationCallback(cb func(ResourceViolation)) ResourceMonitorOption {
	return func(rm *ResourceMonitor) {
		rm.onViolation = cb
	}
}

// WithKillCallback sets the callback for agent kills.
func WithKillCallback(cb func(agentID, reason string)) ResourceMonitorOption {
	return func(rm *ResourceMonitor) {
		rm.onKill = cb
	}
}

// WithDiskMonitorConfig sets disk monitoring configuration.
func WithDiskMonitorConfig(cfg DiskMonitorConfig) ResourceMonitorOption {
	return func(rm *ResourceMonitor) {
		rm.diskConfig = cfg
	}
}

// WithDiskUsageFunc overrides disk usage measurement (useful for tests).
func WithDiskUsageFunc(fn func(string) (DiskUsage, error)) ResourceMonitorOption {
	return func(rm *ResourceMonitor) {
		if fn != nil {
			rm.diskUsageFunc = fn
		}
	}
}

// NewResourceMonitor creates a new resource monitor.
func NewResourceMonitor(logger zerolog.Logger, server *Server, opts ...ResourceMonitorOption) *ResourceMonitor {
	rm := &ResourceMonitor{
		logger:   logger,
		server:   server,
		interval: 5 * time.Second, // Default: check every 5 seconds
		agents:   make(map[string]*agentResourceState),
		limits:   DefaultResourceLimits(),
		diskConfig: func() DiskMonitorConfig {
			cfg := DefaultDiskMonitorConfig()
			return cfg
		}(),
		diskUsageFunc: getDiskUsage,
		diskState: diskMonitorState{
			pausedAgents: make(map[string]int),
		},
	}

	for _, opt := range opts {
		opt(rm)
	}

	rm.normalizeDiskConfig()

	return rm
}

func (rm *ResourceMonitor) normalizeDiskConfig() {
	if rm.diskUsageFunc == nil {
		rm.diskUsageFunc = getDiskUsage
	}
	if strings.TrimSpace(rm.diskConfig.Path) == "" {
		rm.diskConfig.Path = "/"
	}
	if rm.diskConfig.WarnPercent < 0 {
		rm.diskConfig.WarnPercent = 0
	}
	if rm.diskConfig.CriticalPercent < 0 {
		rm.diskConfig.CriticalPercent = 0
	}
	if rm.diskConfig.ResumePercent <= 0 {
		rm.diskConfig.ResumePercent = rm.diskConfig.WarnPercent
	}
	if rm.diskConfig.CriticalPercent > 0 && rm.diskConfig.WarnPercent > 0 && rm.diskConfig.WarnPercent > rm.diskConfig.CriticalPercent {
		rm.diskConfig.WarnPercent = rm.diskConfig.CriticalPercent
	}
}

// Start begins the resource monitoring loop.
func (rm *ResourceMonitor) Start(ctx context.Context) {
	ctx, rm.cancel = context.WithCancel(ctx)

	rm.wg.Add(1)
	go func() {
		defer rm.wg.Done()
		rm.monitorLoop(ctx)
	}()

	rm.logger.Info().
		Dur("interval", rm.interval).
		Int64("memory_limit_mb", rm.limits.MaxMemoryBytes/(1024*1024)).
		Float64("cpu_limit_pct", rm.limits.MaxCPUPercent).
		Float64("disk_warn_pct", rm.diskConfig.WarnPercent).
		Float64("disk_critical_pct", rm.diskConfig.CriticalPercent).
		Str("disk_path", rm.diskConfig.Path).
		Msg("resource monitor started")
}

// Stop stops the resource monitoring loop.
func (rm *ResourceMonitor) Stop() {
	if rm.cancel != nil {
		rm.cancel()
	}
	rm.wg.Wait()
	rm.logger.Info().Msg("resource monitor stopped")
}

// RegisterAgent registers an agent for resource monitoring.
func (rm *ResourceMonitor) RegisterAgent(agentID, workspaceID string, pid int, limits *ResourceLimits) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	agentLimits := rm.limits
	if limits != nil {
		agentLimits = *limits
	}

	rm.agents[agentID] = &agentResourceState{
		agentID:     agentID,
		workspaceID: workspaceID,
		pid:         pid,
		limits:      agentLimits,
	}

	rm.logger.Debug().
		Str("agent_id", agentID).
		Int("pid", pid).
		Int64("memory_limit_mb", agentLimits.MaxMemoryBytes/(1024*1024)).
		Msg("agent registered for resource monitoring")
}

// UnregisterAgent removes an agent from resource monitoring.
func (rm *ResourceMonitor) UnregisterAgent(agentID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	delete(rm.agents, agentID)
	if rm.diskState.pausedAgents != nil {
		delete(rm.diskState.pausedAgents, agentID)
	}

	rm.logger.Debug().
		Str("agent_id", agentID).
		Msg("agent unregistered from resource monitoring")
}

// UpdateAgentPID updates the PID for an agent (useful when PID becomes known).
func (rm *ResourceMonitor) UpdateAgentPID(agentID string, pid int) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if state, exists := rm.agents[agentID]; exists {
		state.pid = pid
		rm.logger.Debug().
			Str("agent_id", agentID).
			Int("pid", pid).
			Msg("agent PID updated")
	}
}

// SetAgentLimits sets custom resource limits for an agent.
func (rm *ResourceMonitor) SetAgentLimits(agentID string, limits ResourceLimits) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if state, exists := rm.agents[agentID]; exists {
		state.limits = limits
	}
}

// GetAgentUsage returns the current resource usage for an agent.
func (rm *ResourceMonitor) GetAgentUsage(agentID string) (ResourceUsage, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if state, exists := rm.agents[agentID]; exists {
		usage := state.usage
		usage.PID = state.pid // Ensure PID is always set from state
		return usage, true
	}
	return ResourceUsage{}, false
}

// GetAllUsage returns resource usage for all monitored agents.
func (rm *ResourceMonitor) GetAllUsage() map[string]ResourceUsage {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make(map[string]ResourceUsage, len(rm.agents))
	for id, state := range rm.agents {
		usage := state.usage
		usage.PID = state.pid // Ensure PID is always set from state
		result[id] = usage
	}
	return result
}

// monitorLoop is the main monitoring loop.
func (rm *ResourceMonitor) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(rm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rm.checkAllAgents()
			rm.checkDiskUsage()
		}
	}
}

// checkAllAgents checks resource usage for all registered agents.
func (rm *ResourceMonitor) checkAllAgents() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for _, state := range rm.agents {
		if state.pid <= 0 {
			continue // No PID yet
		}

		usage, err := rm.measureUsage(state.pid)
		if err != nil {
			rm.logger.Debug().
				Err(err).
				Str("agent_id", state.agentID).
				Int("pid", state.pid).
				Msg("failed to measure resource usage")
			continue
		}

		state.usage = usage
		rm.checkLimits(state)
	}
}

func (rm *ResourceMonitor) checkDiskUsage() {
	if !rm.diskMonitoringEnabled() {
		return
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	usage, err := rm.diskUsageFunc(rm.diskConfig.Path)
	if err != nil {
		rm.logger.Debug().Err(err).Str("path", rm.diskConfig.Path).Msg("failed to measure disk usage")
		return
	}

	rm.diskState.lastUsage = usage
	rm.diskState.lastCheckedAt = time.Now()

	prevLevel := rm.diskState.level
	level := rm.diskUsageLevel(usage.UsedPercent)
	rm.diskState.level = level

	if level != prevLevel {
		rm.handleDiskLevelChange(prevLevel, level, usage)
	}

	if rm.diskConfig.PauseAgents {
		rm.handleDiskPause(level, usage)
	}
}

func (rm *ResourceMonitor) diskMonitoringEnabled() bool {
	return rm.diskConfig.WarnPercent > 0 || rm.diskConfig.CriticalPercent > 0
}

func (rm *ResourceMonitor) diskUsageLevel(usedPercent float64) diskUsageLevel {
	if rm.diskConfig.CriticalPercent > 0 && usedPercent >= rm.diskConfig.CriticalPercent {
		return diskUsageCritical
	}
	if rm.diskConfig.WarnPercent > 0 && usedPercent >= rm.diskConfig.WarnPercent {
		return diskUsageWarn
	}
	return diskUsageOK
}

func (rm *ResourceMonitor) handleDiskLevelChange(prev, current diskUsageLevel, usage DiskUsage) {
	switch current {
	case diskUsageCritical:
		msg := fmt.Sprintf("disk usage critical: %.1f%% used (%s free) on %s", usage.UsedPercent, formatBytes(usage.FreeBytes), usage.Path)
		rm.logger.Warn().Float64("used_pct", usage.UsedPercent).Str("path", usage.Path).Msg(msg)
		rm.publishDiskAlert("disk_critical", msg, false)
	case diskUsageWarn:
		msg := fmt.Sprintf("disk usage high: %.1f%% used (%s free) on %s", usage.UsedPercent, formatBytes(usage.FreeBytes), usage.Path)
		rm.logger.Warn().Float64("used_pct", usage.UsedPercent).Str("path", usage.Path).Msg(msg)
		rm.publishDiskAlert("disk_low", msg, true)
	case diskUsageOK:
		if prev != diskUsageOK {
			msg := fmt.Sprintf("disk usage recovered: %.1f%% used (%s free) on %s", usage.UsedPercent, formatBytes(usage.FreeBytes), usage.Path)
			rm.logger.Info().Float64("used_pct", usage.UsedPercent).Str("path", usage.Path).Msg(msg)
		}
	}
}

func (rm *ResourceMonitor) handleDiskPause(level diskUsageLevel, usage DiskUsage) {
	if level == diskUsageCritical {
		rm.pauseAgentsForDisk()
		return
	}

	resumeAt := rm.diskConfig.ResumePercent
	if resumeAt <= 0 {
		resumeAt = rm.diskConfig.WarnPercent
	}
	if usage.UsedPercent <= resumeAt {
		rm.resumeAgentsForDisk()
	}
}

func (rm *ResourceMonitor) pauseAgentsForDisk() {
	if rm.diskState.pausedAgents == nil {
		rm.diskState.pausedAgents = make(map[string]int)
	}

	for _, state := range rm.agents {
		if state.pid <= 0 {
			continue
		}
		if _, alreadyPaused := rm.diskState.pausedAgents[state.agentID]; alreadyPaused {
			continue
		}
		if err := signalProcess(state.pid, syscall.SIGSTOP); err != nil {
			rm.logger.Warn().Err(err).Str("agent_id", state.agentID).Int("pid", state.pid).Msg("failed to pause agent due to disk pressure")
			continue
		}
		rm.diskState.pausedAgents[state.agentID] = state.pid
		rm.logger.Warn().Str("agent_id", state.agentID).Int("pid", state.pid).Msg("paused agent due to disk pressure")
	}
}

func (rm *ResourceMonitor) resumeAgentsForDisk() {
	if len(rm.diskState.pausedAgents) == 0 {
		return
	}

	for agentID, pid := range rm.diskState.pausedAgents {
		if state, ok := rm.agents[agentID]; ok && state.pid > 0 {
			pid = state.pid
		}
		if err := signalProcess(pid, syscall.SIGCONT); err != nil {
			rm.logger.Warn().Err(err).Str("agent_id", agentID).Int("pid", pid).Msg("failed to resume agent after disk pressure")
			continue
		}
		delete(rm.diskState.pausedAgents, agentID)
		rm.logger.Info().Str("agent_id", agentID).Int("pid", pid).Msg("resumed agent after disk pressure")
	}
}

func (rm *ResourceMonitor) publishDiskAlert(code, message string, recoverable bool) {
	if rm.server == nil {
		return
	}
	rm.server.publishError("", "", code, message, recoverable)
}

func signalProcess(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(sig)
}

// measureUsage measures the resource usage of a process.
func (rm *ResourceMonitor) measureUsage(pid int) (ResourceUsage, error) {
	usage := ResourceUsage{
		PID:        pid,
		MeasuredAt: time.Now(),
	}

	// Try to read from /proc on Linux
	memBytes, err := rm.getMemoryUsage(pid)
	if err != nil {
		return usage, fmt.Errorf("failed to get memory usage: %w", err)
	}
	usage.MemoryBytes = memBytes

	// CPU usage is trickier - we'd need to track over time
	// For now, we'll get a snapshot from /proc/[pid]/stat
	cpuPercent, err := rm.getCPUUsage(pid)
	if err != nil {
		rm.logger.Debug().Err(err).Int("pid", pid).Msg("failed to get CPU usage")
		// Don't fail completely if CPU measurement fails
	} else {
		usage.CPUPercent = cpuPercent
	}

	return usage, nil
}

// getMemoryUsage reads memory usage from /proc/[pid]/status.
func (rm *ResourceMonitor) getMemoryUsage(pid int) (int64, error) {
	statusPath := filepath.Join("/proc", strconv.Itoa(pid), "status")
	file, err := os.Open(statusPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			// Format: "VmRSS:     12345 kB"
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err != nil {
					return 0, err
				}
				return kb * 1024, nil // Convert to bytes
			}
		}
	}

	return 0, fmt.Errorf("VmRSS not found in /proc/%d/status", pid)
}

// getCPUUsage calculates CPU usage percentage.
// This is a simplified version that reads from /proc/[pid]/stat.
func (rm *ResourceMonitor) getCPUUsage(pid int) (float64, error) {
	statPath := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, err
	}

	// Parse /proc/[pid]/stat - format is complex but we need fields 14 (utime) and 15 (stime)
	// The comm field (2) can contain spaces and parentheses, so we need to be careful
	content := string(data)

	// Find the closing paren that ends the comm field
	closeParenIdx := strings.LastIndex(content, ")")
	if closeParenIdx < 0 {
		return 0, fmt.Errorf("invalid /proc/%d/stat format", pid)
	}

	// Fields after the comm field
	fields := strings.Fields(content[closeParenIdx+1:])
	if len(fields) < 13 {
		return 0, fmt.Errorf("not enough fields in /proc/%d/stat", pid)
	}

	// utime is field 14 (index 11 after comm), stime is field 15 (index 12)
	utime, err := strconv.ParseInt(fields[11], 10, 64)
	if err != nil {
		return 0, err
	}
	stime, err := strconv.ParseInt(fields[12], 10, 64)
	if err != nil {
		return 0, err
	}

	// Get system uptime
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	uptimeFields := strings.Fields(string(uptimeData))
	if len(uptimeFields) < 1 {
		return 0, fmt.Errorf("invalid /proc/uptime format")
	}
	uptime, err := strconv.ParseFloat(uptimeFields[0], 64)
	if err != nil {
		return 0, err
	}

	// Get process start time (field 22, index 19 after comm)
	if len(fields) < 20 {
		return 0, fmt.Errorf("not enough fields for starttime in /proc/%d/stat", pid)
	}
	starttime, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return 0, err
	}

	// Get clock ticks per second (usually 100 on Linux)
	clockTicks := int64(100) // sysconf(_SC_CLK_TCK)

	// Calculate CPU usage
	totalTime := utime + stime
	seconds := uptime - (float64(starttime) / float64(clockTicks))
	if seconds <= 0 {
		return 0, nil
	}

	cpuUsage := 100.0 * (float64(totalTime) / float64(clockTicks)) / seconds
	return cpuUsage, nil
}

func getDiskUsage(path string) (DiskUsage, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return DiskUsage{}, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used := total - free

	var usedPercent float64
	if total > 0 {
		usedPercent = (float64(used) / float64(total)) * 100
	}

	return DiskUsage{
		Path:        path,
		TotalBytes:  total,
		FreeBytes:   free,
		UsedBytes:   used,
		UsedPercent: usedPercent,
	}, nil
}

func formatBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := uint64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffixes := []string{"KB", "MB", "GB", "TB", "PB", "EB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(value)/float64(div), suffixes[exp])
}

// checkLimits checks if an agent exceeds resource limits and takes action.
func (rm *ResourceMonitor) checkLimits(state *agentResourceState) {
	now := time.Now()
	var violation *ResourceViolation

	// Check memory limit
	if state.limits.MaxMemoryBytes > 0 && state.usage.MemoryBytes > state.limits.MaxMemoryBytes {
		violation = &ResourceViolation{
			AgentID:       state.agentID,
			WorkspaceID:   state.workspaceID,
			PID:           state.pid,
			ViolationType: "memory",
			CurrentValue:  float64(state.usage.MemoryBytes),
			LimitValue:    float64(state.limits.MaxMemoryBytes),
		}
	}

	// Check CPU limit
	if state.limits.MaxCPUPercent > 0 && state.usage.CPUPercent > state.limits.MaxCPUPercent {
		violation = &ResourceViolation{
			AgentID:       state.agentID,
			WorkspaceID:   state.workspaceID,
			PID:           state.pid,
			ViolationType: "cpu",
			CurrentValue:  state.usage.CPUPercent,
			LimitValue:    state.limits.MaxCPUPercent,
		}
	}

	if violation == nil {
		// No violation - reset tracking
		if state.violationTime != nil {
			rm.logger.Info().
				Str("agent_id", state.agentID).
				Msg("agent resource usage returned to normal")
		}
		state.violationTime = nil
		state.warningSent = false
		return
	}

	// Track violation start time
	if state.violationTime == nil {
		state.violationTime = &now
		state.warningSent = false
	}
	violation.Duration = now.Sub(*state.violationTime)

	// Determine severity
	gracePeriod := time.Duration(state.limits.GracePeriodSeconds) * time.Second
	if violation.Duration >= gracePeriod {
		violation.Severity = "critical"
	} else {
		violation.Severity = "warning"
	}

	// Send warning callback if not sent yet
	if !state.warningSent && rm.onViolation != nil {
		rm.onViolation(*violation)
		state.warningSent = true
	}

	// Kill if grace period exceeded
	if violation.Severity == "critical" {
		reason := fmt.Sprintf("%s limit exceeded: %.2f > %.2f for %v",
			violation.ViolationType,
			violation.CurrentValue,
			violation.LimitValue,
			violation.Duration)

		rm.logger.Warn().
			Str("agent_id", state.agentID).
			Int("pid", state.pid).
			Str("violation_type", violation.ViolationType).
			Float64("current", violation.CurrentValue).
			Float64("limit", violation.LimitValue).
			Dur("duration", violation.Duration).
			Msg("killing agent due to resource limit violation")

		if rm.onKill != nil {
			rm.onKill(state.agentID, reason)
		}

		// Report final violation
		if rm.onViolation != nil {
			rm.onViolation(*violation)
		}
	}
}

// CheckWarnThreshold checks if usage is approaching the limit.
func (rm *ResourceMonitor) CheckWarnThreshold(agentID string) (warnings []string) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	state, exists := rm.agents[agentID]
	if !exists {
		return nil
	}

	threshold := state.limits.WarnThresholdPercent / 100.0

	if state.limits.MaxMemoryBytes > 0 {
		warnBytes := int64(float64(state.limits.MaxMemoryBytes) * threshold)
		if state.usage.MemoryBytes > warnBytes {
			warnings = append(warnings, fmt.Sprintf(
				"memory usage at %.1f%% of limit (%d MB / %d MB)",
				100*float64(state.usage.MemoryBytes)/float64(state.limits.MaxMemoryBytes),
				state.usage.MemoryBytes/(1024*1024),
				state.limits.MaxMemoryBytes/(1024*1024)))
		}
	}

	if state.limits.MaxCPUPercent > 0 {
		warnCPU := state.limits.MaxCPUPercent * threshold
		if state.usage.CPUPercent > warnCPU {
			warnings = append(warnings, fmt.Sprintf(
				"CPU usage at %.1f%% of limit (%.1f%% / %.1f%%)",
				100*state.usage.CPUPercent/state.limits.MaxCPUPercent,
				state.usage.CPUPercent,
				state.limits.MaxCPUPercent))
		}
	}

	return warnings
}

// =============================================================================
// Proto Conversions
// =============================================================================

// ToProtoLimits converts ResourceLimits to proto format.
func (l *ResourceLimits) ToProtoLimits() *swarmdv1.ResourceLimits {
	if l == nil {
		return nil
	}

	action := swarmdv1.ResourceLimitAction_RESOURCE_LIMIT_ACTION_KILL
	if l.GracePeriodSeconds > 0 {
		// If there's a grace period, we warn first
		action = swarmdv1.ResourceLimitAction_RESOURCE_LIMIT_ACTION_WARN
	}

	return &swarmdv1.ResourceLimits{
		MaxCpuPercent:      l.MaxCPUPercent,
		MaxMemoryBytes:     l.MaxMemoryBytes,
		Action:             action,
		ViolationThreshold: 3, // Default: 3 violations before action
	}
}

// FromProtoLimits converts proto ResourceLimits to internal format.
func FromProtoLimits(pl *swarmdv1.ResourceLimits) *ResourceLimits {
	if pl == nil {
		return nil
	}

	gracePeriod := 30 // Default grace period
	if pl.GracePeriod != nil {
		gracePeriod = int(pl.GracePeriod.AsDuration().Seconds())
	}

	return &ResourceLimits{
		MaxMemoryBytes:       pl.MaxMemoryBytes,
		MaxCPUPercent:        pl.MaxCpuPercent,
		GracePeriodSeconds:   gracePeriod,
		WarnThresholdPercent: 80, // Default warn threshold
	}
}

// ToProtoUsage converts ResourceUsage to proto format.
func (u *ResourceUsage) ToProtoUsage() *swarmdv1.AgentResourceUsage {
	if u == nil {
		return nil
	}

	return &swarmdv1.AgentResourceUsage{
		CpuPercent:      u.CPUPercent,
		MemoryBytes:     u.MemoryBytes,
		PeakMemoryBytes: u.MemoryBytes, // We don't track peak separately yet
		ViolationCount:  0,             // Will be set by caller if needed
	}
}

// ToProtoViolationEvent converts ResourceViolation to proto event format.
func (v *ResourceViolation) ToProtoViolationEvent() *swarmdv1.ResourceViolationEvent {
	if v == nil {
		return nil
	}

	resourceType := swarmdv1.ResourceType_RESOURCE_TYPE_MEMORY
	if v.ViolationType == "cpu" {
		resourceType = swarmdv1.ResourceType_RESOURCE_TYPE_CPU
	}

	action := swarmdv1.ResourceLimitAction_RESOURCE_LIMIT_ACTION_WARN
	if v.Severity == "critical" {
		action = swarmdv1.ResourceLimitAction_RESOURCE_LIMIT_ACTION_KILL
	}

	return &swarmdv1.ResourceViolationEvent{
		ResourceType:   resourceType,
		CurrentValue:   v.CurrentValue,
		LimitValue:     v.LimitValue,
		ViolationCount: int32(v.Duration.Seconds()), // Use duration as rough count
		ActionTaken:    action,
	}
}
