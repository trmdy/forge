package forged

import (
	"context"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/gen/forged/v1"
)

func TestDefaultResourceLimits(t *testing.T) {
	limits := DefaultResourceLimits()

	if limits.MaxMemoryBytes != 2*1024*1024*1024 {
		t.Errorf("expected 2GB memory limit, got %d", limits.MaxMemoryBytes)
	}
	if limits.MaxCPUPercent != 200 {
		t.Errorf("expected 200%% CPU limit, got %.1f", limits.MaxCPUPercent)
	}
	if limits.GracePeriodSeconds != 30 {
		t.Errorf("expected 30s grace period, got %d", limits.GracePeriodSeconds)
	}
	if limits.WarnThresholdPercent != 80 {
		t.Errorf("expected 80%% warn threshold, got %.1f", limits.WarnThresholdPercent)
	}
}

func TestDefaultDiskMonitorConfig(t *testing.T) {
	cfg := DefaultDiskMonitorConfig()
	if cfg.Path == "" {
		t.Error("expected disk monitor path to be set")
	}
	if cfg.WarnPercent <= 0 {
		t.Errorf("expected warn percent > 0, got %.1f", cfg.WarnPercent)
	}
	if cfg.CriticalPercent <= 0 {
		t.Errorf("expected critical percent > 0, got %.1f", cfg.CriticalPercent)
	}
	if cfg.ResumePercent <= 0 {
		t.Errorf("expected resume percent > 0, got %.1f", cfg.ResumePercent)
	}
}

func TestResourceMonitor_DiskLevelTransitions(t *testing.T) {
	logger := zerolog.Nop()
	usages := []DiskUsage{
		{Path: "/tmp", UsedPercent: 50, FreeBytes: 100},
		{Path: "/tmp", UsedPercent: 87, FreeBytes: 50},
		{Path: "/tmp", UsedPercent: 97, FreeBytes: 10},
		{Path: "/tmp", UsedPercent: 70, FreeBytes: 80},
	}
	index := 0

	rm := NewResourceMonitor(logger, nil,
		WithDiskMonitorConfig(DiskMonitorConfig{
			Path:            "/tmp",
			WarnPercent:     80,
			CriticalPercent: 95,
			ResumePercent:   75,
		}),
		WithDiskUsageFunc(func(path string) (DiskUsage, error) {
			if index >= len(usages) {
				return usages[len(usages)-1], nil
			}
			usage := usages[index]
			index++
			return usage, nil
		}),
	)

	rm.checkDiskUsage()
	if rm.diskState.level != diskUsageOK {
		t.Errorf("expected disk level ok, got %v", rm.diskState.level)
	}

	rm.checkDiskUsage()
	if rm.diskState.level != diskUsageWarn {
		t.Errorf("expected disk level warn, got %v", rm.diskState.level)
	}

	rm.checkDiskUsage()
	if rm.diskState.level != diskUsageCritical {
		t.Errorf("expected disk level critical, got %v", rm.diskState.level)
	}

	rm.checkDiskUsage()
	if rm.diskState.level != diskUsageOK {
		t.Errorf("expected disk level ok after recovery, got %v", rm.diskState.level)
	}
}

func TestResourceMonitor_RegisterUnregister(t *testing.T) {
	logger := zerolog.Nop()
	rm := NewResourceMonitor(logger, nil)

	// Register an agent
	rm.RegisterAgent("agent-1", "workspace-1", 12345, nil)

	// Check it's registered by accessing internal state
	rm.mu.RLock()
	state, ok := rm.agents["agent-1"]
	rm.mu.RUnlock()

	if !ok {
		t.Fatal("expected agent to be registered")
	}
	if state.pid != 12345 {
		t.Errorf("expected PID 12345, got %d", state.pid)
	}

	// Unregister
	rm.UnregisterAgent("agent-1")

	// Check it's gone
	_, ok = rm.GetAgentUsage("agent-1")
	if ok {
		t.Error("expected agent to be unregistered")
	}
}

func TestResourceMonitor_UpdatePID(t *testing.T) {
	logger := zerolog.Nop()
	rm := NewResourceMonitor(logger, nil)

	// Register with PID 0
	rm.RegisterAgent("agent-1", "workspace-1", 0, nil)

	// Update PID
	rm.UpdateAgentPID("agent-1", 54321)

	// Verify by accessing internal state
	rm.mu.RLock()
	state, ok := rm.agents["agent-1"]
	rm.mu.RUnlock()

	if !ok {
		t.Fatal("expected agent to be registered")
	}
	if state.pid != 54321 {
		t.Errorf("expected PID 54321, got %d", state.pid)
	}
}

func TestResourceMonitor_CustomLimits(t *testing.T) {
	logger := zerolog.Nop()
	rm := NewResourceMonitor(logger, nil)

	customLimits := &ResourceLimits{
		MaxMemoryBytes:       512 * 1024 * 1024, // 512 MB
		MaxCPUPercent:        50,
		GracePeriodSeconds:   10,
		WarnThresholdPercent: 70,
	}

	rm.RegisterAgent("agent-1", "workspace-1", 12345, customLimits)

	// Verify limits were set
	rm.mu.RLock()
	state := rm.agents["agent-1"]
	rm.mu.RUnlock()

	if state.limits.MaxMemoryBytes != customLimits.MaxMemoryBytes {
		t.Errorf("expected custom memory limit, got %d", state.limits.MaxMemoryBytes)
	}
	if state.limits.MaxCPUPercent != customLimits.MaxCPUPercent {
		t.Errorf("expected custom CPU limit, got %.1f", state.limits.MaxCPUPercent)
	}
}

func TestResourceMonitor_ViolationCallback(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping in CI - requires /proc filesystem")
	}
	if runtime.GOOS != "linux" {
		t.Skip("skipping - requires /proc filesystem")
	}
	if _, err := os.Stat("/proc"); err != nil {
		t.Skip("skipping - /proc not available")
	}

	logger := zerolog.Nop()

	var callbackCalled atomic.Bool
	var lastViolation ResourceViolation

	rm := NewResourceMonitor(logger, nil,
		WithMonitorInterval(50*time.Millisecond),
		WithViolationCallback(func(v ResourceViolation) {
			callbackCalled.Store(true)
			lastViolation = v
		}),
	)

	// Register current process with very low memory limit to trigger violation
	limits := &ResourceLimits{
		MaxMemoryBytes:       1, // 1 byte - guaranteed to exceed
		MaxCPUPercent:        0, // No CPU limit
		GracePeriodSeconds:   0, // Immediate action
		WarnThresholdPercent: 0,
	}

	rm.RegisterAgent("agent-1", "workspace-1", os.Getpid(), limits)
	rm.Start(context.Background())
	defer rm.Stop()

	// Wait for violation detection
	time.Sleep(200 * time.Millisecond)

	if !callbackCalled.Load() {
		t.Error("expected violation callback to be called")
	}

	if lastViolation.ViolationType != "memory" {
		t.Errorf("expected memory violation, got %s", lastViolation.ViolationType)
	}
	if lastViolation.AgentID != "agent-1" {
		t.Errorf("expected agent-1, got %s", lastViolation.AgentID)
	}
}

func TestResourceMonitor_StartStop(t *testing.T) {
	logger := zerolog.Nop()
	rm := NewResourceMonitor(logger, nil,
		WithMonitorInterval(10*time.Millisecond),
	)

	// Start
	ctx := context.Background()
	rm.Start(ctx)

	// Should be running - add an agent and check it gets measured
	rm.RegisterAgent("agent-1", "ws-1", os.Getpid(), nil)

	time.Sleep(50 * time.Millisecond)

	// Stop
	rm.Stop()

	// Verify stopped gracefully (no panic, no deadlock)
}

func TestResourceMonitor_GetAllUsage(t *testing.T) {
	logger := zerolog.Nop()
	rm := NewResourceMonitor(logger, nil)

	rm.RegisterAgent("agent-1", "ws-1", 1111, nil)
	rm.RegisterAgent("agent-2", "ws-2", 2222, nil)
	rm.RegisterAgent("agent-3", "ws-1", 3333, nil)

	usages := rm.GetAllUsage()

	if len(usages) != 3 {
		t.Errorf("expected 3 usages, got %d", len(usages))
	}

	// GetAllUsage returns ResourceUsage, not the internal state
	// Just verify all agents are tracked
	if _, ok := usages["agent-1"]; !ok {
		t.Error("expected agent-1 in usages")
	}
	if _, ok := usages["agent-2"]; !ok {
		t.Error("expected agent-2 in usages")
	}
	if _, ok := usages["agent-3"]; !ok {
		t.Error("expected agent-3 in usages")
	}
}

func TestResourceMonitor_SetAgentLimits(t *testing.T) {
	logger := zerolog.Nop()
	rm := NewResourceMonitor(logger, nil)

	// Register with default limits
	rm.RegisterAgent("agent-1", "ws-1", 1234, nil)

	// Update limits
	newLimits := ResourceLimits{
		MaxMemoryBytes:       1024 * 1024 * 1024, // 1 GB
		MaxCPUPercent:        100,
		GracePeriodSeconds:   60,
		WarnThresholdPercent: 90,
	}
	rm.SetAgentLimits("agent-1", newLimits)

	// Verify
	rm.mu.RLock()
	state := rm.agents["agent-1"]
	rm.mu.RUnlock()

	if state.limits.MaxMemoryBytes != newLimits.MaxMemoryBytes {
		t.Errorf("expected updated memory limit")
	}
}

func TestResourceMonitor_CheckWarnThreshold(t *testing.T) {
	logger := zerolog.Nop()
	rm := NewResourceMonitor(logger, nil)

	limits := &ResourceLimits{
		MaxMemoryBytes:       1000, // Very small for testing
		MaxCPUPercent:        100,
		GracePeriodSeconds:   30,
		WarnThresholdPercent: 80,
	}

	rm.RegisterAgent("agent-1", "ws-1", 1234, limits)

	// Simulate usage at 85% of limit
	rm.mu.Lock()
	rm.agents["agent-1"].usage.MemoryBytes = 850
	rm.mu.Unlock()

	warnings := rm.CheckWarnThreshold("agent-1")

	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}
}

// =============================================================================
// Proto Conversion Tests
// =============================================================================

func TestResourceLimits_ToProtoLimits(t *testing.T) {
	limits := &ResourceLimits{
		MaxMemoryBytes:       1024 * 1024 * 1024,
		MaxCPUPercent:        200,
		GracePeriodSeconds:   30,
		WarnThresholdPercent: 80,
	}

	proto := limits.ToProtoLimits()

	if proto.MaxMemoryBytes != limits.MaxMemoryBytes {
		t.Errorf("memory mismatch: got %d, want %d", proto.MaxMemoryBytes, limits.MaxMemoryBytes)
	}
	if proto.MaxCpuPercent != limits.MaxCPUPercent {
		t.Errorf("CPU mismatch: got %.1f, want %.1f", proto.MaxCpuPercent, limits.MaxCPUPercent)
	}
}

func TestFromProtoLimits(t *testing.T) {
	proto := &forgedv1.ResourceLimits{
		MaxMemoryBytes: 512 * 1024 * 1024,
		MaxCpuPercent:  150,
	}

	limits := FromProtoLimits(proto)

	if limits.MaxMemoryBytes != proto.MaxMemoryBytes {
		t.Errorf("memory mismatch")
	}
	if limits.MaxCPUPercent != proto.MaxCpuPercent {
		t.Errorf("CPU mismatch")
	}
}

func TestFromProtoLimits_Nil(t *testing.T) {
	limits := FromProtoLimits(nil)
	if limits != nil {
		t.Error("expected nil for nil input")
	}
}

func TestResourceUsage_ToProtoUsage(t *testing.T) {
	usage := &ResourceUsage{
		PID:         12345,
		MemoryBytes: 100 * 1024 * 1024,
		CPUPercent:  50.5,
		MeasuredAt:  time.Now(),
	}

	proto := usage.ToProtoUsage()

	if proto.MemoryBytes != usage.MemoryBytes {
		t.Errorf("memory mismatch")
	}
	if proto.CpuPercent != usage.CPUPercent {
		t.Errorf("CPU mismatch")
	}
}

func TestResourceViolation_ToProtoViolationEvent(t *testing.T) {
	tests := []struct {
		name           string
		violation      ResourceViolation
		wantType       forgedv1.ResourceType
		wantActionKill bool
	}{
		{
			name: "memory violation warning",
			violation: ResourceViolation{
				AgentID:       "agent-1",
				WorkspaceID:   "ws-1",
				ViolationType: "memory",
				CurrentValue:  2000,
				LimitValue:    1000,
				Severity:      "warning",
			},
			wantType:       forgedv1.ResourceType_RESOURCE_TYPE_MEMORY,
			wantActionKill: false,
		},
		{
			name: "CPU violation critical",
			violation: ResourceViolation{
				AgentID:       "agent-2",
				WorkspaceID:   "ws-2",
				ViolationType: "cpu",
				CurrentValue:  150,
				LimitValue:    100,
				Severity:      "critical",
			},
			wantType:       forgedv1.ResourceType_RESOURCE_TYPE_CPU,
			wantActionKill: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto := tt.violation.ToProtoViolationEvent()

			if proto.ResourceType != tt.wantType {
				t.Errorf("resource type: got %v, want %v", proto.ResourceType, tt.wantType)
			}

			if tt.wantActionKill {
				if proto.ActionTaken != forgedv1.ResourceLimitAction_RESOURCE_LIMIT_ACTION_KILL {
					t.Errorf("expected KILL action for critical violation")
				}
			} else {
				if proto.ActionTaken != forgedv1.ResourceLimitAction_RESOURCE_LIMIT_ACTION_WARN {
					t.Errorf("expected WARN action for warning violation")
				}
			}
		})
	}
}
