package models

import (
	"errors"
	"time"
)

// Harness identifies a loop harness implementation.
type Harness string

const (
	HarnessPi       Harness = "pi"
	HarnessOpenCode Harness = "opencode"
	HarnessCodex    Harness = "codex"
	HarnessClaude   Harness = "claude"
	HarnessDroid    Harness = "droid"
)

// PromptMode controls how prompts are delivered to a harness.
type PromptMode string

const (
	PromptModeEnv   PromptMode = "env"
	PromptModeStdin PromptMode = "stdin"
	PromptModePath  PromptMode = "path"
)

// Profile represents a harness+auth combination.
type Profile struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Harness         Harness           `json:"harness"`
	AuthKind        string            `json:"auth_kind,omitempty"`
	AuthHome        string            `json:"auth_home,omitempty"`
	PromptMode      PromptMode        `json:"prompt_mode"`
	CommandTemplate string            `json:"command_template"`
	Model           string            `json:"model,omitempty"`
	ExtraArgs       []string          `json:"extra_args,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	MaxConcurrency  int               `json:"max_concurrency"`
	CooldownUntil   *time.Time        `json:"cooldown_until,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// Validate checks if the profile configuration is valid.
func (p *Profile) Validate() error {
	validation := &ValidationErrors{}
	if p.Name == "" {
		validation.Add("name", ErrInvalidProfileName)
	}
	if p.CommandTemplate == "" {
		validation.Add("command_template", ErrInvalidCommandTemplate)
	}
	if p.MaxConcurrency < 0 {
		validation.AddMessage("max_concurrency", "max_concurrency must be >= 0")
	}
	if validation.Err() != nil {
		return validation.Err()
	}

	switch p.Harness {
	case "", HarnessPi, HarnessOpenCode, HarnessCodex, HarnessClaude, HarnessDroid:
		// ok
	default:
		return ErrInvalidProfileHarness
	}

	switch p.PromptMode {
	case "", PromptModeEnv, PromptModeStdin, PromptModePath:
		return nil
	default:
		return errors.New("invalid prompt_mode")
	}
}

// DefaultPromptMode returns the default prompt mode for profiles.
func DefaultPromptMode() PromptMode {
	return PromptModeEnv
}
