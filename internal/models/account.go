package models

import (
	"time"
)

// Provider identifies an AI provider.
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderGoogle    Provider = "google"
	ProviderCustom    Provider = "custom"
)

// Account represents a provider account/profile for authentication.
type Account struct {
	// ID is the unique identifier for the account.
	ID string `json:"id"`

	// Provider identifies the AI provider.
	Provider Provider `json:"provider"`

	// ProfileName is the human-friendly name for this account.
	ProfileName string `json:"profile_name"`

	// CredentialRef is a reference to the credential (env var, file path, or vault key).
	CredentialRef string `json:"credential_ref"`

	// IsActive indicates if this account is enabled for use.
	IsActive bool `json:"is_active"`

	// CooldownUntil is when the cooldown expires (if rate-limited).
	CooldownUntil *time.Time `json:"cooldown_until,omitempty"`

	// UsageStats contains usage information for this account.
	UsageStats *UsageStats `json:"usage_stats,omitempty"`

	// CreatedAt is when the account was added.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the account was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// UsageStats contains usage metrics for an account.
type UsageStats struct {
	// TotalTokens is the total tokens used.
	TotalTokens int64 `json:"total_tokens"`

	// TotalCost is the estimated total cost (in cents).
	TotalCostCents int64 `json:"total_cost_cents"`

	// LastUsed is when the account was last used.
	LastUsed *time.Time `json:"last_used,omitempty"`

	// RequestCount is the number of API requests made.
	RequestCount int64 `json:"request_count"`

	// RateLimitCount is how many times this account hit rate limits.
	RateLimitCount int64 `json:"rate_limit_count"`
}

// UsageLimits defines thresholds for usage warnings.
type UsageLimits struct {
	// MaxTokensPerDay is the daily token limit (0 = no limit).
	MaxTokensPerDay int64 `json:"max_tokens_per_day,omitempty"`

	// MaxCostPerDayCents is the daily cost limit in cents (0 = no limit).
	MaxCostPerDayCents int64 `json:"max_cost_per_day_cents,omitempty"`

	// MaxRequestsPerHour is the hourly request limit (0 = no limit).
	MaxRequestsPerHour int64 `json:"max_requests_per_hour,omitempty"`

	// WarningThresholdPercent is when to start warning (default: 80).
	WarningThresholdPercent int `json:"warning_threshold_percent,omitempty"`
}

// DefaultWarningThreshold is the default percentage at which to warn.
const DefaultWarningThreshold = 80

// UsageLimitStatus represents the current status relative to limits.
type UsageLimitStatus struct {
	// IsApproachingLimit indicates a limit is being approached.
	IsApproachingLimit bool `json:"is_approaching_limit"`

	// IsOverLimit indicates a limit has been exceeded.
	IsOverLimit bool `json:"is_over_limit"`

	// Warnings contains specific warning messages.
	Warnings []string `json:"warnings,omitempty"`

	// TokensUsedPercent is percentage of daily token limit used.
	TokensUsedPercent float64 `json:"tokens_used_percent,omitempty"`

	// CostUsedPercent is percentage of daily cost limit used.
	CostUsedPercent float64 `json:"cost_used_percent,omitempty"`

	// RequestsUsedPercent is percentage of hourly request limit used.
	RequestsUsedPercent float64 `json:"requests_used_percent,omitempty"`
}

// CheckLimits evaluates current usage against limits.
func CheckLimits(usage *UsageSummary, limits *UsageLimits) *UsageLimitStatus {
	status := &UsageLimitStatus{}

	if usage == nil || limits == nil {
		return status
	}

	threshold := limits.WarningThresholdPercent
	if threshold <= 0 {
		threshold = DefaultWarningThreshold
	}
	thresholdFloat := float64(threshold) / 100.0

	// Check token limit
	if limits.MaxTokensPerDay > 0 {
		status.TokensUsedPercent = float64(usage.TotalTokens) / float64(limits.MaxTokensPerDay) * 100
		if status.TokensUsedPercent >= 100 {
			status.IsOverLimit = true
			status.Warnings = append(status.Warnings, "Daily token limit exceeded")
		} else if status.TokensUsedPercent >= thresholdFloat*100 {
			status.IsApproachingLimit = true
			status.Warnings = append(status.Warnings, "Approaching daily token limit")
		}
	}

	// Check cost limit
	if limits.MaxCostPerDayCents > 0 {
		status.CostUsedPercent = float64(usage.TotalCostCents) / float64(limits.MaxCostPerDayCents) * 100
		if status.CostUsedPercent >= 100 {
			status.IsOverLimit = true
			status.Warnings = append(status.Warnings, "Daily cost limit exceeded")
		} else if status.CostUsedPercent >= thresholdFloat*100 {
			status.IsApproachingLimit = true
			status.Warnings = append(status.Warnings, "Approaching daily cost limit")
		}
	}

	// Check request limit
	if limits.MaxRequestsPerHour > 0 {
		status.RequestsUsedPercent = float64(usage.RequestCount) / float64(limits.MaxRequestsPerHour) * 100
		if status.RequestsUsedPercent >= 100 {
			status.IsOverLimit = true
			status.Warnings = append(status.Warnings, "Hourly request limit exceeded")
		} else if status.RequestsUsedPercent >= thresholdFloat*100 {
			status.IsApproachingLimit = true
			status.Warnings = append(status.Warnings, "Approaching hourly request limit")
		}
	}

	return status
}

// Validate checks if the account configuration is valid.
func (a *Account) Validate() error {
	validation := &ValidationErrors{}
	if a.Provider == "" {
		validation.Add("provider", ErrInvalidProvider)
	}
	if a.ProfileName == "" {
		validation.Add("profile_name", ErrInvalidProfileName)
	}
	return validation.Err()
}

// IsOnCooldown returns true if the account is currently on cooldown.
func (a *Account) IsOnCooldown() bool {
	if a.CooldownUntil == nil {
		return false
	}
	return time.Now().Before(*a.CooldownUntil)
}

// CooldownRemaining returns the remaining cooldown duration.
func (a *Account) CooldownRemaining() time.Duration {
	if !a.IsOnCooldown() {
		return 0
	}
	return time.Until(*a.CooldownUntil)
}

// IsAvailable returns true if the account can be used.
func (a *Account) IsAvailable() bool {
	return a.IsActive && !a.IsOnCooldown()
}
