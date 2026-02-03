//go:build windows

package forged

import forgedv1 "github.com/tOgg1/forge/gen/forged/v1"

func (s *Server) getResourceUsage() *forgedv1.ResourceUsage {
	return &forgedv1.ResourceUsage{}
}
