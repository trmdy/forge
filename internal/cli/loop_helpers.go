package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

type loopSelector struct {
	All     bool
	LoopRef string
	Repo    string
	Pool    string
	Profile string
	State   string
	Tag     string
}

func resolveRepoPath(path string) (string, error) {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
		return filepath.Abs(cwd)
	}
	return filepath.Abs(path)
}

func resolvePromptPath(repoPath, value string) (string, bool, error) {
	if strings.TrimSpace(value) == "" {
		return "", false, errors.New("prompt is required")
	}

	candidate := value
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(repoPath, value)
	}
	if exists(candidate) {
		return candidate, true, nil
	}

	if !strings.HasSuffix(value, ".md") {
		candidate = filepath.Join(repoPath, ".forge", "prompts", value+".md")
	} else {
		candidate = filepath.Join(repoPath, ".forge", "prompts", value)
	}
	if exists(candidate) {
		return candidate, true, nil
	}

	return "", false, fmt.Errorf("prompt not found: %s", value)
}

func parseTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	seen := make(map[string]struct{})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func parseDuration(value string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", value)
	}
	return parsed, nil
}

func selectLoops(ctx context.Context, loopRepo *db.LoopRepository, poolRepo *db.PoolRepository, profileRepo *db.ProfileRepository, selector loopSelector) ([]*models.Loop, error) {
	loops, err := loopRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	repoFilter := selector.Repo
	if repoFilter != "" {
		repoFilter, err = filepath.Abs(repoFilter)
		if err != nil {
			return nil, err
		}
	}

	var poolID string
	if selector.Pool != "" {
		pool, err := resolvePoolByRef(ctx, poolRepo, selector.Pool)
		if err != nil {
			return nil, err
		}
		poolID = pool.ID
	}

	var profileID string
	if selector.Profile != "" {
		profile, err := resolveProfileByRef(ctx, profileRepo, selector.Profile)
		if err != nil {
			return nil, err
		}
		profileID = profile.ID
	}

	filtered := make([]*models.Loop, 0)
	for _, loop := range loops {
		if selector.LoopRef != "" {
			if loop.Name != selector.LoopRef && loop.ID != selector.LoopRef {
				continue
			}
		}
		if repoFilter != "" && loop.RepoPath != repoFilter {
			continue
		}
		if poolID != "" && loop.PoolID != poolID {
			continue
		}
		if profileID != "" && loop.ProfileID != profileID {
			continue
		}
		if selector.State != "" && string(loop.State) != selector.State {
			continue
		}
		if selector.Tag != "" && !loopHasTag(loop, selector.Tag) {
			continue
		}
		filtered = append(filtered, loop)
	}

	if len(filtered) == 0 && selector.LoopRef != "" {
		return nil, fmt.Errorf("loop %q not found", selector.LoopRef)
	}

	return filtered, nil
}

func resolveLoopByRef(ctx context.Context, repo *db.LoopRepository, ref string) (*models.Loop, error) {
	loopEntry, err := repo.GetByName(ctx, ref)
	if err == nil {
		return loopEntry, nil
	}
	return repo.Get(ctx, ref)
}

func loopHasTag(loop *models.Loop, tag string) bool {
	for _, value := range loop.Tags {
		if value == tag {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}
