package models

import (
	"errors"
	"time"
)

// LoopState represents the current loop status.
type LoopState string

const (
	LoopStateRunning  LoopState = "running"
	LoopStateSleeping LoopState = "sleeping"
	LoopStateWaiting  LoopState = "waiting"
	LoopStateStopped  LoopState = "stopped"
	LoopStateError    LoopState = "error"
)

// Loop represents a background agent loop tied to a repo.
type Loop struct {
	ID             string         `json:"id"`
	ShortID        string         `json:"short_id"`
	Name           string         `json:"name"`
	RepoPath       string         `json:"repo_path"`
	BasePromptPath string         `json:"base_prompt_path,omitempty"`
	BasePromptMsg  string         `json:"base_prompt_msg,omitempty"`
	IntervalSeconds int           `json:"interval_seconds"`
	PoolID         string         `json:"pool_id,omitempty"`
	ProfileID      string         `json:"profile_id,omitempty"`
	State          LoopState      `json:"state"`
	LastRunAt      *time.Time     `json:"last_run_at,omitempty"`
	LastExitCode   *int           `json:"last_exit_code,omitempty"`
	LastError      string         `json:"last_error,omitempty"`
	LogPath        string         `json:"log_path,omitempty"`
	LedgerPath     string         `json:"ledger_path,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// Validate checks if the loop is valid.
func (l *Loop) Validate() error {
	validation := &ValidationErrors{}
	if l.Name == "" {
		validation.Add("name", ErrInvalidLoopName)
	}
	if l.RepoPath == "" {
		validation.Add("repo_path", ErrInvalidLoopRepoPath)
	}
	if !isValidLoopShortID(l.ShortID) {
		validation.Add("short_id", ErrInvalidLoopShortID)
	}
	if l.IntervalSeconds < 0 {
		validation.AddMessage("interval_seconds", "interval_seconds must be >= 0")
	}
	if validation.Err() != nil {
		return validation.Err()
	}

	switch l.State {
	case "", LoopStateRunning, LoopStateSleeping, LoopStateWaiting, LoopStateStopped, LoopStateError:
		return nil
	default:
		return errors.New("invalid loop state")
	}
}

// DefaultLoopState returns the default loop state.
func DefaultLoopState() LoopState {
	return LoopStateStopped
}

func isValidLoopShortID(value string) bool {
	if len(value) < 6 || len(value) > 9 {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}
