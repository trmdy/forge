//go:build windows

package forged

import "fmt"

func pauseProcess(pid int) error {
	return fmt.Errorf("pause not supported on windows")
}

func resumeProcess(pid int) error {
	return fmt.Errorf("resume not supported on windows")
}

func getDiskUsage(path string) (DiskUsage, error) {
	return DiskUsage{}, fmt.Errorf("disk usage not supported on windows")
}
