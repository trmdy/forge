package skills

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tOgg1/forge/internal/config"
	"github.com/tOgg1/forge/internal/models"
)

type InstallResult struct {
	Dest      string
	Harnesses []string
	Created   []string
	Skipped   []string
}

// Bootstrap writes the builtin skills into destDir. If force is false,
// existing files are skipped.
func Bootstrap(destDir string, force bool) (*InstallResult, error) {
	result := &InstallResult{
		Dest:    destDir,
		Created: []string{},
		Skipped: []string{},
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create skills dir: %w", err)
	}

	created, skipped, err := copyFromFS(builtinFS, "builtin", destDir, force)
	if err != nil {
		return nil, fmt.Errorf("failed to bootstrap skills: %w", err)
	}

	result.Created = append(result.Created, created...)
	result.Skipped = append(result.Skipped, skipped...)

	return result, nil
}

// InstallBuiltinToHarnesses installs embedded skills into harness-specific destinations.
func InstallBuiltinToHarnesses(baseDir string, profiles []config.ProfileConfig, force bool) ([]InstallResult, error) {
	destinations := map[string]*InstallResult{}

	for _, profile := range profiles {
		dest, ok := resolveHarnessDest(baseDir, profile.Harness, profile.AuthHome)
		if !ok {
			continue
		}
		item, exists := destinations[dest]
		if !exists {
			item = &InstallResult{
				Dest:      dest,
				Harnesses: []string{},
				Created:   []string{},
				Skipped:   []string{},
			}
			destinations[dest] = item
		}
		item.Harnesses = append(item.Harnesses, string(profile.Harness))
	}

	results := make([]InstallResult, 0, len(destinations))
	for _, dest := range destinations {
		created, skipped, err := copyFromFS(builtinFS, "builtin", dest.Dest, force)
		if err != nil {
			return nil, err
		}
		dest.Created = created
		dest.Skipped = skipped
		results = append(results, *dest)
	}

	return results, nil
}

// InstallToHarnesses installs skills from sourceDir into harness-specific destinations.
func InstallToHarnesses(baseDir, sourceDir string, profiles []config.ProfileConfig, force bool) ([]InstallResult, error) {
	destinations := map[string]*InstallResult{}

	for _, profile := range profiles {
		dest, ok := resolveHarnessDest(baseDir, profile.Harness, profile.AuthHome)
		if !ok {
			continue
		}
		item, exists := destinations[dest]
		if !exists {
			item = &InstallResult{
				Dest:      dest,
				Harnesses: []string{},
				Created:   []string{},
				Skipped:   []string{},
			}
			destinations[dest] = item
		}
		item.Harnesses = append(item.Harnesses, string(profile.Harness))
	}

	results := make([]InstallResult, 0, len(destinations))
	for _, dest := range destinations {
		created, skipped, err := copyDir(sourceDir, dest.Dest, force)
		if err != nil {
			return nil, err
		}
		dest.Created = created
		dest.Skipped = skipped
		results = append(results, *dest)
	}

	return results, nil
}

func resolveHarnessDest(baseDir string, harness models.Harness, authHome string) (string, bool) {
	if authHome != "" {
		return filepath.Join(authHome, "skills"), true
	}

	if baseDir != "" {
		switch string(harness) {
		case string(models.HarnessCodex):
			return filepath.Join(baseDir, ".codex", "skills"), true
		case string(models.HarnessClaude), "claude_code":
			return filepath.Join(baseDir, ".claude", "skills"), true
		case string(models.HarnessOpenCode):
			return filepath.Join(baseDir, ".opencode", "skills"), true
		case string(models.HarnessPi):
			return filepath.Join(baseDir, ".pi", "skills"), true
		default:
			return "", false
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}

	switch string(harness) {
	case string(models.HarnessCodex):
		return filepath.Join(home, ".codex", "skills"), true
	case string(models.HarnessClaude), "claude_code":
		return filepath.Join(home, ".claude", "skills"), true
	case string(models.HarnessOpenCode):
		return filepath.Join(home, ".config", "opencode", "skills"), true
	case string(models.HarnessPi):
		return filepath.Join(home, ".pi", "skills"), true
	default:
		return "", false
	}
}

func copyDir(sourceDir, destDir string, force bool) ([]string, []string, error) {
	created := []string{}
	skipped := []string{}

	err := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		targetPath := filepath.Join(destDir, relPath)
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		if !force {
			if _, err := os.Stat(targetPath); err == nil {
				skipped = append(skipped, targetPath)
				return nil
			}
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(targetPath), err)
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", path, err)
		}

		destFile, err := os.Create(targetPath)
		if err != nil {
			_ = srcFile.Close()
			return fmt.Errorf("failed to create %s: %w", targetPath, err)
		}

		if _, err := io.Copy(destFile, srcFile); err != nil {
			_ = destFile.Close()
			_ = srcFile.Close()
			return fmt.Errorf("failed to copy %s: %w", targetPath, err)
		}
		if err := destFile.Close(); err != nil {
			_ = srcFile.Close()
			return fmt.Errorf("failed to close %s: %w", targetPath, err)
		}
		if err := srcFile.Close(); err != nil {
			return fmt.Errorf("failed to close %s: %w", path, err)
		}

		created = append(created, targetPath)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to copy skills: %w", err)
	}

	return created, skipped, nil
}

func copyFromFS(sourceFS fs.FS, root, destDir string, force bool) ([]string, []string, error) {
	created := []string{}
	skipped := []string{}

	err := fs.WalkDir(sourceFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		relPath := strings.TrimPrefix(path, root+"/")
		targetPath := filepath.Join(destDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		if !force {
			if _, err := os.Stat(targetPath); err == nil {
				skipped = append(skipped, targetPath)
				return nil
			}
		}

		data, err := fs.ReadFile(sourceFS, path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(targetPath), err)
		}
		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", targetPath, err)
		}
		created = append(created, targetPath)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to copy skills: %w", err)
	}

	return created, skipped, nil
}
