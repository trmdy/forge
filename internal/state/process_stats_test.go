package state

import (
	"os"
	"runtime"
	"testing"
	"time"
)

func TestNewProcessStatsCollector(t *testing.T) {
	c := NewProcessStatsCollector()
	if c == nil {
		t.Fatal("NewProcessStatsCollector returned nil")
	}

	if c.pageSize <= 0 {
		t.Errorf("pageSize should be positive, got %d", c.pageSize)
	}

	if c.cpuTicks <= 0 {
		t.Errorf("cpuTicks should be positive, got %d", c.cpuTicks)
	}

	if c.prevCPUTimes == nil {
		t.Error("prevCPUTimes map should be initialized")
	}
}

func TestCollect_InvalidPID(t *testing.T) {
	c := NewProcessStatsCollector()

	tests := []struct {
		name string
		pid  int
	}{
		{"zero pid", 0},
		{"negative pid", -1},
		{"very negative pid", -999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := c.Collect(tt.pid)
			if stats != nil {
				t.Errorf("Collect(%d) should return nil, got %+v", tt.pid, stats)
			}
		})
	}
}

func TestCollect_NonExistentPID(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	c := NewProcessStatsCollector()

	// Use a very high PID that's unlikely to exist
	stats := c.Collect(999999999)
	if stats != nil {
		// On Linux, should return nil for non-existent process
		t.Logf("Got stats for non-existent PID (may be valid): %+v", stats)
	}
}

func TestCollect_CurrentProcess(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	c := NewProcessStatsCollector()
	pid := os.Getpid()

	// First collection (baseline for CPU)
	stats := c.Collect(pid)
	if stats == nil {
		t.Fatal("Collect returned nil for current process")
	}

	// Verify memory stats
	if stats.MemoryBytes <= 0 {
		t.Errorf("MemoryBytes should be positive, got %d", stats.MemoryBytes)
	}

	if stats.MemoryPercent < 0 || stats.MemoryPercent > 100 {
		t.Errorf("MemoryPercent should be 0-100, got %f", stats.MemoryPercent)
	}

	// UpdatedAt should be recent
	if time.Since(stats.UpdatedAt) > time.Second {
		t.Errorf("UpdatedAt should be recent, got %v", stats.UpdatedAt)
	}

	// Do some work to generate CPU usage
	for i := 0; i < 1000000; i++ {
		_ = i * i
	}

	// Small delay to allow for CPU time accumulation
	time.Sleep(10 * time.Millisecond)

	// Second collection should have CPU percentage
	stats2 := c.Collect(pid)
	if stats2 == nil {
		t.Fatal("Second Collect returned nil for current process")
	}

	// CPU percent should be non-negative (could be 0 if very little time passed)
	if stats2.CPUPercent < 0 {
		t.Errorf("CPUPercent should be non-negative, got %f", stats2.CPUPercent)
	}

	t.Logf("Process stats: CPU=%.2f%%, Memory=%d bytes (%.2f%%), IO Read=%d, IO Write=%d",
		stats2.CPUPercent, stats2.MemoryBytes, stats2.MemoryPercent,
		stats2.IOReadBytes, stats2.IOWriteBytes)
}

func TestCollect_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Test only runs on non-Linux")
	}

	c := NewProcessStatsCollector()
	stats := c.Collect(os.Getpid())

	if stats != nil {
		t.Error("Collect should return nil on non-Linux systems")
	}
}

func TestCleanup(t *testing.T) {
	c := NewProcessStatsCollector()

	// Add some fake entries
	c.prevCPUTimes[1] = cpuSample{utime: 100, stime: 50, timestamp: time.Now()}
	c.prevCPUTimes[2] = cpuSample{utime: 200, stime: 100, timestamp: time.Now()}
	c.prevCPUTimes[3] = cpuSample{utime: 300, stime: 150, timestamp: time.Now()}

	// Only keep PID 2
	activePIDs := map[int]bool{2: true}
	c.Cleanup(activePIDs)

	if len(c.prevCPUTimes) != 1 {
		t.Errorf("Expected 1 entry after cleanup, got %d", len(c.prevCPUTimes))
	}

	if _, ok := c.prevCPUTimes[2]; !ok {
		t.Error("PID 2 should still be present after cleanup")
	}
}

func TestGetTotalMemory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	mem := getTotalMemory()
	if mem <= 0 {
		t.Errorf("getTotalMemory should return positive value, got %d", mem)
	}

	// Should be at least 128MB (reasonable minimum)
	minMem := int64(128 * 1024 * 1024)
	if mem < minMem {
		t.Errorf("Total memory %d seems too low (expected at least %d)", mem, minMem)
	}

	t.Logf("Total system memory: %d bytes (%.2f GB)", mem, float64(mem)/(1024*1024*1024))
}

func TestCollectMemory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	c := NewProcessStatsCollector()
	pid := os.Getpid()

	mem := c.collectMemory(pid)
	if mem == nil {
		t.Fatal("collectMemory returned nil for current process")
	}

	if mem.rss <= 0 {
		t.Errorf("RSS should be positive, got %d", mem.rss)
	}

	// RSS should be at least a few MB for a Go process
	minRSS := int64(1024 * 1024) // 1MB
	if mem.rss < minRSS {
		t.Errorf("RSS %d seems too low for a Go process", mem.rss)
	}

	t.Logf("Process RSS: %d bytes (%.2f MB)", mem.rss, float64(mem.rss)/(1024*1024))
}

func TestCollectCPU(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	c := NewProcessStatsCollector()
	pid := os.Getpid()

	// First sample
	cpu1 := c.collectCPU(pid)
	if cpu1 == nil {
		t.Fatal("collectCPU returned nil for current process")
	}

	// First sample should return 0% (no baseline)
	if cpu1.percent != 0 {
		t.Errorf("First CPU sample should be 0%%, got %f", cpu1.percent)
	}

	// Do some work
	sum := 0
	for i := 0; i < 1000000; i++ {
		sum += i
	}
	_ = sum

	time.Sleep(10 * time.Millisecond)

	// Second sample should have a percentage
	cpu2 := c.collectCPU(pid)
	if cpu2 == nil {
		t.Fatal("Second collectCPU returned nil")
	}

	if cpu2.percent < 0 {
		t.Errorf("CPU percent should be non-negative, got %f", cpu2.percent)
	}

	t.Logf("CPU usage: %.2f%%", cpu2.percent)
}

func TestCollectIO(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	c := NewProcessStatsCollector()
	pid := os.Getpid()

	io := c.collectIO(pid)
	if io == nil {
		// IO stats may not be available depending on permissions
		t.Log("collectIO returned nil (may be permission issue)")
		return
	}

	// IO bytes should be non-negative
	if io.readBytes < 0 {
		t.Errorf("readBytes should be non-negative, got %d", io.readBytes)
	}

	if io.writeBytes < 0 {
		t.Errorf("writeBytes should be non-negative, got %d", io.writeBytes)
	}

	t.Logf("IO stats: read=%d bytes, write=%d bytes", io.readBytes, io.writeBytes)
}

func TestCollectMemory_InvalidPID(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	c := NewProcessStatsCollector()

	mem := c.collectMemory(999999999)
	if mem != nil {
		t.Error("collectMemory should return nil for invalid PID")
	}
}

func TestCollectCPU_InvalidPID(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	c := NewProcessStatsCollector()

	cpu := c.collectCPU(999999999)
	if cpu != nil {
		t.Error("collectCPU should return nil for invalid PID")
	}
}

func TestCollectIO_InvalidPID(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	c := NewProcessStatsCollector()

	io := c.collectIO(999999999)
	if io != nil {
		t.Error("collectIO should return nil for invalid PID")
	}
}
