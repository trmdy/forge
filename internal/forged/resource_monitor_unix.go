//go:build !windows

package forged

import (
	"fmt"
	"os"
	"syscall"
)

func pauseProcess(pid int) error {
	return signalProcess(pid, syscall.SIGSTOP)
}

func resumeProcess(pid int) error {
	return signalProcess(pid, syscall.SIGCONT)
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
