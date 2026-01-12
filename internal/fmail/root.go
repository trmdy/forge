package fmail

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiscoverProjectRoot determines the project root using FMAIL_ROOT or by walking upward.
// Order: FMAIL_ROOT -> .fmail -> .git -> current directory.
func DiscoverProjectRoot(startDir string) (string, error) {
	if rootEnv := strings.TrimSpace(os.Getenv(EnvRoot)); rootEnv != "" {
		return normalizeRoot(startDir, rootEnv)
	}

	start, err := resolveStartDir(startDir)
	if err != nil {
		return "", err
	}

	if root, ok := walkUp(start, ".fmail", true); ok {
		return root, nil
	}
	if root, ok := walkUp(start, ".git", false); ok {
		if sharedRoot, ok := worktreeSharedRoot(root); ok {
			return sharedRoot, nil
		}
		return root, nil
	}
	return start, nil
}

func resolveStartDir(startDir string) (string, error) {
	dir := strings.TrimSpace(startDir)
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		dir = cwd
	}
	return filepath.Abs(dir)
}

func normalizeRoot(baseDir, root string) (string, error) {
	path := strings.TrimSpace(root)
	if path == "" {
		return "", fmt.Errorf("empty %s", EnvRoot)
	}
	if !filepath.IsAbs(path) {
		base, err := resolveStartDir(baseDir)
		if err != nil {
			return "", err
		}
		path = filepath.Join(base, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory: %s", EnvRoot, abs)
	}
	return abs, nil
}

func walkUp(start, marker string, dirOnly bool) (string, bool) {
	current := start
	for {
		path := filepath.Join(current, marker)
		info, err := os.Stat(path)
		if err == nil {
			if !dirOnly || info.IsDir() {
				return current, true
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func worktreeSharedRoot(repoRoot string) (string, bool) {
	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil || info.IsDir() {
		return "", false
	}

	gitDir, err := readGitDir(gitPath, repoRoot)
	if err != nil {
		return "", false
	}

	shared, ok := sharedRootFromGitDir(gitDir)
	if !ok {
		return "", false
	}
	return shared, true
}

func readGitDir(gitFilePath, baseDir string) (string, error) {
	data, err := os.ReadFile(gitFilePath)
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "gitdir:") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
			if path == "" {
				return "", fmt.Errorf("empty gitdir")
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			return filepath.Abs(path)
		}
	}

	return "", fmt.Errorf("gitdir not found")
}

func sharedRootFromGitDir(gitDir string) (string, bool) {
	clean := filepath.Clean(gitDir)
	sep := string(filepath.Separator)
	marker := sep + ".git" + sep + "worktrees" + sep
	idx := strings.Index(clean, marker)
	if idx == -1 {
		return "", false
	}

	commonGitDir := filepath.Clean(clean[:idx+len(sep+".git")])
	info, err := os.Stat(commonGitDir)
	if err != nil || !info.IsDir() {
		return "", false
	}

	root := filepath.Dir(commonGitDir)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return "", false
	}
	return root, true
}
