//go:build !windows

package forged

import (
	"syscall"

	forgedv1 "github.com/tOgg1/forge/gen/forged/v1"
)

func (s *Server) getResourceUsage() *forgedv1.ResourceUsage {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		return &forgedv1.ResourceUsage{}
	}

	return &forgedv1.ResourceUsage{
		MemoryBytes: rusage.Maxrss * 1024, // maxrss is in KB on Linux
	}
}
