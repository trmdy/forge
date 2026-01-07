package loop

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/tOgg1/forge/internal/config"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

var (
	ErrProfileUnavailable = errors.New("profile unavailable")
	ErrPoolUnavailable    = errors.New("pool unavailable")
)

func (r *Runner) selectProfile(ctx context.Context, loop *models.Loop, profileRepo *db.ProfileRepository, poolRepo *db.PoolRepository, runRepo *db.LoopRunRepository) (*models.Profile, *time.Time, error) {
	now := time.Now().UTC()
	if loop.ProfileID != "" {
		profile, err := profileRepo.Get(ctx, loop.ProfileID)
		if err != nil {
			return nil, nil, err
		}
		available, _, err := profileAvailable(ctx, runRepo, profile, now)
		if err != nil {
			return nil, nil, err
		}
		if !available {
			return nil, nil, fmt.Errorf("pinned profile %s unavailable", profile.Name)
		}
		return profile, nil, nil
	}

	pool, err := resolvePool(ctx, loop, r.Config, poolRepo)
	if err != nil {
		return nil, nil, err
	}

	members, err := poolRepo.ListMembers(ctx, pool.ID)
	if err != nil {
		return nil, nil, err
	}
	if len(members) == 0 {
		return nil, nil, ErrPoolUnavailable
	}

	startIndex := poolLastIndex(pool)
	var earliest *time.Time

	for i := 0; i < len(members); i++ {
		idx := (startIndex + 1 + i) % len(members)
		member := members[idx]
		profile, err := profileRepo.Get(ctx, member.ProfileID)
		if err != nil {
			continue
		}
		available, next, err := profileAvailable(ctx, runRepo, profile, now)
		if err != nil {
			continue
		}
		if available {
			setPoolLastIndex(pool, idx)
			_ = poolRepo.Update(ctx, pool)
			return profile, nil, nil
		}
		if next != nil {
			if earliest == nil || next.Before(*earliest) {
				copy := *next
				earliest = &copy
			}
		}
	}

	if earliest == nil {
		wait := now.Add(defaultWaitInterval)
		earliest = &wait
	}

	return nil, earliest, nil
}

func profileAvailable(ctx context.Context, runRepo *db.LoopRunRepository, profile *models.Profile, now time.Time) (bool, *time.Time, error) {
	if profile.CooldownUntil != nil && profile.CooldownUntil.After(now) {
		next := *profile.CooldownUntil
		return false, &next, nil
	}

	if profile.MaxConcurrency > 0 {
		count, err := runRepo.CountRunningByProfile(ctx, profile.ID)
		if err != nil {
			return false, nil, err
		}
		if count >= profile.MaxConcurrency {
			return false, nil, nil
		}
	}

	return true, nil, nil
}

func resolvePool(ctx context.Context, loop *models.Loop, cfg *config.Config, poolRepo *db.PoolRepository) (*models.Pool, error) {
	if loop.PoolID != "" {
		return poolRepo.Get(ctx, loop.PoolID)
	}

	if cfg != nil && cfg.DefaultPool != "" {
		pool, err := poolRepo.GetByName(ctx, cfg.DefaultPool)
		if err == nil {
			return pool, nil
		}
	}

	pool, err := poolRepo.GetDefault(ctx)
	if err != nil {
		return nil, ErrPoolUnavailable
	}
	return pool, nil
}

func poolLastIndex(pool *models.Pool) int {
	if pool.Metadata == nil {
		return -1
	}
	value, ok := pool.Metadata["last_index"]
	if !ok {
		return -1
	}

	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		parsed, err := strconv.Atoi(v)
		if err == nil {
			return parsed
		}
	}

	return -1
}

func setPoolLastIndex(pool *models.Pool, idx int) {
	if pool.Metadata == nil {
		pool.Metadata = make(map[string]any)
	}
	pool.Metadata["last_index"] = idx
}
