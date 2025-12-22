package ssh

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// HostConfig contains SSH configuration resolved for a host.
type HostConfig struct {
	HostName       string
	User           string
	Port           int
	ProxyJump      string
	IdentityFiles  []string
	ControlMaster  string
	ControlPath    string
	ControlPersist string
}

type sshConfig struct {
	entries []sshConfigEntry
}

type sshConfigEntry struct {
	patterns []string
	options  map[string][]string
}

func loadSSHConfig(paths []string) (*sshConfig, error) {
	if len(paths) == 0 {
		defaults, err := defaultSSHConfigPaths()
		if err != nil {
			return nil, err
		}
		paths = defaults
	}

	var entries []sshConfigEntry
	for _, cfgPath := range paths {
		if strings.TrimSpace(cfgPath) == "" || !fileExists(cfgPath) {
			continue
		}

		parsed, err := parseSSHConfigFile(cfgPath)
		if err != nil {
			return nil, err
		}
		entries = append(entries, parsed...)
	}

	if len(entries) == 0 {
		return nil, nil
	}

	return &sshConfig{entries: entries}, nil
}

func (c *sshConfig) Resolve(host string) HostConfig {
	host = normalizeHost(host)
	result := HostConfig{}

	for _, entry := range c.entries {
		if !entry.matches(host) {
			continue
		}

		result.apply(entry)
	}

	return result
}

func (e sshConfigEntry) matches(host string) bool {
	if len(e.patterns) == 0 {
		return false
	}

	matched := false
	for _, pattern := range e.patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		negated := strings.HasPrefix(pattern, "!")
		if negated {
			pattern = strings.TrimPrefix(pattern, "!")
		}

		if matchHostPattern(pattern, host) {
			if negated {
				return false
			}
			matched = true
		}
	}

	return matched
}

func (cfg *HostConfig) apply(entry sshConfigEntry) {
	if cfg.HostName == "" {
		cfg.HostName = entry.first("hostname")
	}
	if cfg.User == "" {
		cfg.User = entry.first("user")
	}
	if cfg.Port == 0 {
		if value := entry.first("port"); value != "" {
			if port, err := strconv.Atoi(value); err == nil {
				cfg.Port = port
			}
		}
	}
	if cfg.ProxyJump == "" {
		cfg.ProxyJump = entry.first("proxyjump")
	}
	for _, value := range entry.options["identityfile"] {
		expanded := expandSSHPath(value)
		if expanded != "" {
			cfg.IdentityFiles = append(cfg.IdentityFiles, expanded)
		}
	}
	if cfg.ControlMaster == "" {
		cfg.ControlMaster = entry.first("controlmaster")
	}
	if cfg.ControlPath == "" {
		if value := entry.first("controlpath"); value != "" {
			cfg.ControlPath = expandSSHPath(value)
		}
	}
	if cfg.ControlPersist == "" {
		cfg.ControlPersist = entry.first("controlpersist")
	}
}

func (e sshConfigEntry) first(key string) string {
	values := e.options[strings.ToLower(key)]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func parseSSHConfigFile(path string) ([]sshConfigEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ssh config: %w", err)
	}
	defer file.Close()

	var entries []sshConfigEntry
	var current *sshConfigEntry

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := stripComment(scanner.Text())
		if strings.TrimSpace(line) == "" {
			continue
		}

		key, value := splitSSHConfigLine(line)
		if key == "" || value == "" {
			continue
		}

		switch strings.ToLower(key) {
		case "host":
			patterns := strings.Fields(value)
			if len(patterns) == 0 {
				current = nil
				continue
			}
			entry := sshConfigEntry{
				patterns: patterns,
				options:  make(map[string][]string),
			}
			entries = append(entries, entry)
			current = &entries[len(entries)-1]
		case "match":
			current = nil
		default:
			if current == nil {
				continue
			}
			lower := strings.ToLower(strings.TrimSpace(key))
			current.options[lower] = append(current.options[lower], strings.TrimSpace(value))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read ssh config: %w", err)
	}

	return entries, nil
}

func stripComment(line string) string {
	if idx := strings.Index(line, "#"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func splitSSHConfigLine(line string) (string, string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", ""
	}

	if strings.Contains(line, "=") {
		parts := strings.SplitN(line, "=", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}

	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", ""
	}
	return fields[0], strings.Join(fields[1:], " ")
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return host
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func matchHostPattern(pattern, host string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	host = strings.ToLower(strings.TrimSpace(host))
	if pattern == "" || host == "" {
		return false
	}

	matched, err := path.Match(pattern, host)
	if err != nil {
		return pattern == host
	}
	return matched
}

func defaultSSHConfigPaths() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	return []string{
		filepath.Join(home, ".ssh", "config"),
		"/etc/ssh/ssh_config",
	}, nil
}
