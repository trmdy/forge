package models

import "time"

// PoolStrategy determines how profiles are selected within a pool.
type PoolStrategy string

const (
	PoolStrategyRoundRobin PoolStrategy = "round_robin"
	PoolStrategyLRU        PoolStrategy = "lru"
)

// Pool represents a list of profiles with selection strategy.
type Pool struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Strategy  PoolStrategy   `json:"strategy"`
	IsDefault bool           `json:"is_default"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// PoolMember associates a profile with a pool.
type PoolMember struct {
	ID        string    `json:"id"`
	PoolID    string    `json:"pool_id"`
	ProfileID string    `json:"profile_id"`
	Weight    int       `json:"weight"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

// Validate checks if the pool configuration is valid.
func (p *Pool) Validate() error {
	validation := &ValidationErrors{}
	if p.Name == "" {
		validation.Add("name", ErrInvalidPoolName)
	}
	return validation.Err()
}
