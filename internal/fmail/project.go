package fmail

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Project struct {
	ID      string    `json:"id"`
	Created time.Time `json:"created"`
}

// DeriveProjectID returns a stable project ID based on env override, git remote, or directory name.
func DeriveProjectID(projectRoot string) (string, error) {
	if override := strings.TrimSpace(os.Getenv(EnvProject)); override != "" {
		return override, nil
	}

	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return "", fmt.Errorf("project root required")
	}

	if remote, err := gitRemoteURL(root); err == nil && remote != "" {
		return hashProjectID(remote), nil
	}

	base := filepath.Base(root)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "", fmt.Errorf("invalid project root: %s", root)
	}
	return hashProjectID(base), nil
}

func hashProjectID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "proj-" + hex.EncodeToString(sum[:])[:12]
}

func gitRemoteURL(root string) (string, error) {
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", err
		}
		return "", nil
	}
	return strings.TrimSpace(string(output)), nil
}
