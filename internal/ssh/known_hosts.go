package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/ssh/knownhosts"
	xssh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// HostKeyPrompt asks the user whether to trust an unknown host key.
type HostKeyPrompt func(hostname string, remote net.Addr, key xssh.PublicKey) (bool, error)

// DefaultHostKeyPrompt prompts on stdin for host key acceptance.
func DefaultHostKeyPrompt(hostname string, remote net.Addr, key xssh.PublicKey) (bool, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return false, ErrHostKeyPromptUnavailable
	}

	displayHost := strings.TrimSpace(hostname)
	if displayHost == "" && remote != nil {
		displayHost = remote.String()
	}
	if displayHost == "" {
		displayHost = "unknown"
	}

	fmt.Fprintf(os.Stderr, "The authenticity of host '%s' can't be established.\n", displayHost)
	if remote != nil && remote.String() != "" && remote.String() != displayHost {
		fmt.Fprintf(os.Stderr, "Remote address: %s\n", remote.String())
	}
	fmt.Fprintf(os.Stderr, "Key fingerprint SHA256:%s.\n", xssh.FingerprintSHA256(key))
	fmt.Fprint(os.Stderr, "Add host to known_hosts? (yes/no): ")

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))

	switch answer {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func buildKnownHostsCallback(files []string, writePath string, prompt HostKeyPrompt, logger zerolog.Logger) (xssh.HostKeyCallback, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("known_hosts files are required")
	}

	if writePath == "" {
		writePath = files[0]
	}

	knownHostsFiles, err := prepareKnownHostsFiles(files, writePath)
	if err != nil {
		return nil, err
	}

	baseCallback, err := knownhosts.New(knownHostsFiles...)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}

	return func(hostname string, remote net.Addr, key xssh.PublicKey) error {
		if err := baseCallback(hostname, remote, key); err != nil {
			var keyErr *knownhosts.KeyError
			if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
				if prompt == nil {
					return err
				}
				accepted, promptErr := prompt(hostname, remote, key)
				if promptErr != nil {
					return promptErr
				}
				if !accepted {
					return ErrHostKeyRejected
				}
				addresses := hostnamesForKnownHosts(hostname, remote)
				if len(addresses) == 0 {
					return fmt.Errorf("unable to determine host for known_hosts entry")
				}
				if addErr := appendKnownHostsEntry(writePath, addresses, key); addErr != nil {
					return addErr
				}
				logger.Info().Str("host", addresses[0]).Msg("added host key to known_hosts")
				return nil
			}
			return err
		}
		return nil
	}, nil
}

func defaultKnownHostsFiles() ([]string, error) {
	path, err := defaultKnownHostsPath()
	if err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func defaultKnownHostsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".ssh", "known_hosts"), nil
}

func prepareKnownHostsFiles(files []string, writePath string) ([]string, error) {
	writePath = strings.TrimSpace(writePath)
	if writePath == "" {
		return nil, fmt.Errorf("known_hosts path is required")
	}

	if err := ensureKnownHostsFile(writePath); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	seen[writePath] = struct{}{}
	candidates := []string{writePath}
	for _, path := range files {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		if path != writePath && !fileExists(path) {
			continue
		}
		seen[path] = struct{}{}
		candidates = append(candidates, path)
	}

	return candidates, nil
}

func ensureKnownHostsFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create known_hosts directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("create known_hosts file: %w", err)
	}
	return file.Close()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hostnamesForKnownHosts(hostname string, remote net.Addr) []string {
	var addresses []string

	trimmed := strings.TrimSpace(hostname)
	if trimmed != "" {
		addresses = append(addresses, trimmed)
	}

	if remote != nil {
		addr := strings.TrimSpace(remote.String())
		if addr != "" && addr != trimmed {
			addresses = append(addresses, addr)
		}
	}

	return addresses
}

func appendKnownHostsEntry(path string, addresses []string, key xssh.PublicKey) error {
	if err := ensureKnownHostsFile(path); err != nil {
		return err
	}

	line := knownhosts.Line(addresses, key)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open known_hosts file: %w", err)
	}
	defer file.Close()

	if _, err := fmt.Fprintln(file, line); err != nil {
		return fmt.Errorf("write known_hosts entry: %w", err)
	}

	return nil
}
