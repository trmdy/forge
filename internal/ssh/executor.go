// Package ssh provides abstractions for executing commands on remote nodes.
package ssh

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Executor defines a common interface for running commands over SSH.
type Executor interface {
	// Exec runs a command and returns its stdout and stderr output.
	Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error)

	// ExecInteractive runs a command, streaming stdin to the remote process.
	ExecInteractive(ctx context.Context, cmd string, stdin io.Reader) error

	// StartSession opens a long-lived SSH session for multiple commands.
	StartSession() (Session, error)

	// Close releases any resources held by the executor.
	Close() error
}

// Session represents a long-lived SSH session.
type Session interface {
	// Exec runs a command and returns its stdout and stderr output.
	Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error)

	// ExecInteractive runs a command, streaming stdin to the remote process.
	ExecInteractive(ctx context.Context, cmd string, stdin io.Reader) error

	// Close ends the session.
	Close() error
}

// ConnectionOptions configures how an SSH connection is established.
type ConnectionOptions struct {
	// Host is the target host name or IP.
	Host string

	// Port is the SSH port (defaults to 22 when unset).
	Port int

	// User is the SSH username.
	User string

	// KeyPath is an optional path to the private key.
	KeyPath string

	// AgentForwarding enables SSH agent forwarding when supported.
	AgentForwarding bool

	// ProxyJump specifies a bastion host to reach the target (user@host:port).
	ProxyJump string

	// ControlMaster configures SSH multiplexing (auto/yes/no).
	ControlMaster string

	// ControlPath is the socket path for SSH multiplexing.
	ControlPath string

	// ControlPersist controls how long master connections stay alive.
	ControlPersist string

	// Timeout controls how long to wait when establishing connections.
	Timeout time.Duration
}

// ApplySSHConfig applies settings from SSH config files to the connection options.
// It looks up the host alias and updates Host, Port, User, KeyPath, ProxyJump, and
// SSH multiplexing settings. Explicitly set fields are not overridden.
func ApplySSHConfig(opts ConnectionOptions, paths ...string) (ConnectionOptions, error) {
	host := strings.TrimSpace(opts.Host)
	if host == "" {
		return opts, nil
	}

	config, err := loadSSHConfig(paths)
	if err != nil {
		return opts, err
	}
	if config == nil {
		return opts, nil
	}

	resolved := config.Resolve(host)

	if resolved.HostName != "" {
		opts.Host = resolved.HostName
	}
	if opts.User == "" && resolved.User != "" {
		opts.User = resolved.User
	}
	if opts.Port == 0 && resolved.Port > 0 {
		opts.Port = resolved.Port
	}
	if opts.ProxyJump == "" && resolved.ProxyJump != "" {
		if proxy := normalizeProxyJump(resolved.ProxyJump); proxy != "" {
			opts.ProxyJump = proxy
		}
	}
	if opts.KeyPath == "" && len(resolved.IdentityFiles) > 0 {
		opts.KeyPath = resolved.IdentityFiles[0]
	}
	if opts.ControlMaster == "" && resolved.ControlMaster != "" {
		opts.ControlMaster = resolved.ControlMaster
	}
	if opts.ControlPath == "" && resolved.ControlPath != "" {
		opts.ControlPath = resolved.ControlPath
	}
	if opts.ControlPersist == "" && resolved.ControlPersist != "" {
		opts.ControlPersist = resolved.ControlPersist
	}

	return opts, nil
}

func expandSSHPath(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, "\"'")
	if trimmed == "" {
		return ""
	}

	expanded := os.ExpandEnv(trimmed)
	if strings.HasPrefix(expanded, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			expanded = filepath.Join(home, strings.TrimPrefix(expanded, "~"))
		}
	}
	return expanded
}

func normalizeProxyJump(value string) string {
	jumps := parseProxyJumpList(value)
	if len(jumps) == 0 {
		return ""
	}
	return strings.Join(jumps, ",")
}

func parseProxyJumpList(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "none") {
		return nil
	}

	parts := strings.Split(trimmed, ",")
	jumps := make([]string, 0, len(parts))
	for _, part := range parts {
		jump := strings.TrimSpace(part)
		if jump == "" {
			continue
		}
		if strings.EqualFold(jump, "none") {
			return nil
		}
		jumps = append(jumps, jump)
	}
	return jumps
}
