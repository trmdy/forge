package ssh

import (
	"context"
	"fmt"
	"net"
	"strings"
)

const (
	defaultForwardHost = "127.0.0.1"
)

// PortForwardSpec describes a local port forward to a remote host and port.
type PortForwardSpec struct {
	LocalHost  string
	LocalPort  int
	RemoteHost string
	RemotePort int
}

// PortForward manages the lifecycle of a port forward session.
type PortForward interface {
	LocalAddr() string
	RemoteAddr() string
	Wait() error
	Close() error
}

// PortForwarder exposes port forwarding capabilities.
type PortForwarder interface {
	StartPortForward(ctx context.Context, spec PortForwardSpec) (PortForward, error)
}

func normalizePortForwardSpec(spec PortForwardSpec) (PortForwardSpec, error) {
	spec.LocalHost = strings.TrimSpace(spec.LocalHost)
	if spec.LocalHost == "" {
		spec.LocalHost = defaultForwardHost
	}
	spec.RemoteHost = strings.TrimSpace(spec.RemoteHost)
	if spec.RemoteHost == "" {
		spec.RemoteHost = defaultForwardHost
	}

	if spec.LocalPort < 0 || spec.LocalPort > 65535 {
		return spec, fmt.Errorf("local port must be between 0 and 65535")
	}
	if spec.RemotePort < 1 || spec.RemotePort > 65535 {
		return spec, fmt.Errorf("remote port must be between 1 and 65535")
	}

	return spec, nil
}

func (s PortForwardSpec) localAddr() string {
	return net.JoinHostPort(s.LocalHost, fmt.Sprintf("%d", s.LocalPort))
}

func (s PortForwardSpec) remoteAddr() string {
	return net.JoinHostPort(s.RemoteHost, fmt.Sprintf("%d", s.RemotePort))
}

func formatForwardHost(host string) string {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return trimmed
	}
	if strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "[") {
		return "[" + trimmed + "]"
	}
	return trimmed
}

func formatForwardSpec(spec PortForwardSpec) string {
	localHost := formatForwardHost(spec.LocalHost)
	remoteHost := formatForwardHost(spec.RemoteHost)
	return fmt.Sprintf("%s:%d:%s:%d", localHost, spec.LocalPort, remoteHost, spec.RemotePort)
}
