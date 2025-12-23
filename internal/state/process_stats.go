package state

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/opencode-ai/swarm/internal/models"
)

// ProcessStatsCollector collects resource metrics for processes.
type ProcessStatsCollector struct {
	// pageSize is the system page size in bytes.
	pageSize int64

	// totalMemory is the total system memory in bytes.
	totalMemory int64

	// cpuTicks is the number of clock ticks per second.
	cpuTicks int64

	// prevCPUTimes stores previous CPU times for calculating percentage.
	prevCPUTimes map[int]cpuSample
}

// cpuSample stores CPU time sample for delta calculation.
type cpuSample struct {
	utime     int64
	stime     int64
	timestamp time.Time
}

// NewProcessStatsCollector creates a new ProcessStatsCollector.
func NewProcessStatsCollector() *ProcessStatsCollector {
	c := &ProcessStatsCollector{
		pageSize:     4096, // default page size
		cpuTicks:     100,  // default clock ticks (SC_CLK_TCK)
		prevCPUTimes: make(map[int]cpuSample),
	}

	// Get actual page size on Linux
	if runtime.GOOS == "linux" {
		c.pageSize = int64(os.Getpagesize())
		c.totalMemory = getTotalMemory()
		// Note: SC_CLK_TCK is typically 100 on Linux, hardcoded for simplicity
	}

	return c
}

// Collect gathers process stats for the given PID.
// Returns nil if the process doesn't exist or stats cannot be collected.
func (c *ProcessStatsCollector) Collect(pid int) *models.ProcessStats {
	if pid <= 0 {
		return nil
	}

	if runtime.GOOS != "linux" {
		// Process stats collection only supported on Linux
		return nil
	}

	stats := &models.ProcessStats{
		UpdatedAt: time.Now(),
	}

	// Collect memory stats from /proc/[pid]/statm
	if mem := c.collectMemory(pid); mem != nil {
		stats.MemoryBytes = mem.rss
		if c.totalMemory > 0 {
			stats.MemoryPercent = float64(mem.rss) / float64(c.totalMemory) * 100
		}
	}

	// Collect CPU stats from /proc/[pid]/stat
	if cpu := c.collectCPU(pid); cpu != nil {
		stats.CPUPercent = cpu.percent
	}

	// Collect IO stats from /proc/[pid]/io
	if io := c.collectIO(pid); io != nil {
		stats.IOReadBytes = io.readBytes
		stats.IOWriteBytes = io.writeBytes
	}

	return stats
}

// memStats holds memory statistics.
type memStats struct {
	rss int64 // resident set size in bytes
}

// collectMemory reads memory stats from /proc/[pid]/statm.
func (c *ProcessStatsCollector) collectMemory(pid int) *memStats {
	path := fmt.Sprintf("/proc/%d/statm", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	// Format: size resident shared text lib data dt
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return nil
	}

	// Second field is RSS in pages
	rssPages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil
	}

	return &memStats{
		rss: rssPages * c.pageSize,
	}
}

// cpuStats holds CPU statistics.
type cpuStats struct {
	percent float64
}

// collectCPU reads CPU stats from /proc/[pid]/stat.
func (c *ProcessStatsCollector) collectCPU(pid int) *cpuStats {
	path := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	// Parse stat file - format is complex due to comm field potentially containing spaces/parens
	// Find the last ')' which ends the comm field
	str := string(data)
	lastParen := strings.LastIndex(str, ")")
	if lastParen == -1 || lastParen+2 >= len(str) {
		return nil
	}

	// Fields after comm: state(3) ppid(4) pgrp(5) session(6) tty_nr(7) tpgid(8)
	// flags(9) minflt(10) cminflt(11) majflt(12) cmajflt(13) utime(14) stime(15)
	fields := strings.Fields(str[lastParen+2:])
	if len(fields) < 13 {
		return nil
	}

	// utime is field 14 (index 11 after comm), stime is field 15 (index 12)
	utime, err := strconv.ParseInt(fields[11], 10, 64)
	if err != nil {
		return nil
	}
	stime, err := strconv.ParseInt(fields[12], 10, 64)
	if err != nil {
		return nil
	}

	now := time.Now()
	prev, hasPrev := c.prevCPUTimes[pid]

	// Store current sample for next calculation
	c.prevCPUTimes[pid] = cpuSample{
		utime:     utime,
		stime:     stime,
		timestamp: now,
	}

	if !hasPrev {
		// First sample - can't calculate percentage yet
		return &cpuStats{percent: 0}
	}

	// Calculate CPU percentage
	elapsed := now.Sub(prev.timestamp).Seconds()
	if elapsed <= 0 {
		return &cpuStats{percent: 0}
	}

	// Total CPU ticks used since last sample
	totalTicks := (utime - prev.utime) + (stime - prev.stime)
	// Convert to seconds and calculate percentage
	cpuSeconds := float64(totalTicks) / float64(c.cpuTicks)
	percent := (cpuSeconds / elapsed) * 100

	return &cpuStats{percent: percent}
}

// ioStats holds IO statistics.
type ioStats struct {
	readBytes  int64
	writeBytes int64
}

// collectIO reads IO stats from /proc/[pid]/io.
func (c *ProcessStatsCollector) collectIO(pid int) *ioStats {
	path := fmt.Sprintf("/proc/%d/io", pid)
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	stats := &ioStats{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "read_bytes:") {
			if val, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "read_bytes:")), 10, 64); err == nil {
				stats.readBytes = val
			}
		} else if strings.HasPrefix(line, "write_bytes:") {
			if val, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "write_bytes:")), 10, 64); err == nil {
				stats.writeBytes = val
			}
		}
	}

	return stats
}

// Cleanup removes stale entries from the CPU time cache.
func (c *ProcessStatsCollector) Cleanup(activePIDs map[int]bool) {
	for pid := range c.prevCPUTimes {
		if !activePIDs[pid] {
			delete(c.prevCPUTimes, pid)
		}
	}
}

// getTotalMemory reads total system memory from /proc/meminfo.
func getTotalMemory() int64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				// Value is in kB
				if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					return kb * 1024
				}
			}
			break
		}
	}

	return 0
}
