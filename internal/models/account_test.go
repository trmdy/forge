package models

import (
	"testing"
	"time"
)

func TestAccount_IsOnCooldown(t *testing.T) {
	tests := []struct {
		name          string
		cooldownUntil *time.Time
		want          bool
	}{
		{
			name:          "no cooldown",
			cooldownUntil: nil,
			want:          false,
		},
		{
			name:          "cooldown expired",
			cooldownUntil: timePtr(time.Now().Add(-time.Hour)),
			want:          false,
		},
		{
			name:          "on cooldown",
			cooldownUntil: timePtr(time.Now().Add(time.Hour)),
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{CooldownUntil: tt.cooldownUntil}
			if got := a.IsOnCooldown(); got != tt.want {
				t.Errorf("Account.IsOnCooldown() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAccount_IsAvailable(t *testing.T) {
	tests := []struct {
		name          string
		isActive      bool
		cooldownUntil *time.Time
		want          bool
	}{
		{
			name:     "active no cooldown",
			isActive: true,
			want:     true,
		},
		{
			name:     "inactive",
			isActive: false,
			want:     false,
		},
		{
			name:          "active on cooldown",
			isActive:      true,
			cooldownUntil: timePtr(time.Now().Add(time.Hour)),
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{
				IsActive:      tt.isActive,
				CooldownUntil: tt.cooldownUntil,
			}
			if got := a.IsAvailable(); got != tt.want {
				t.Errorf("Account.IsAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckLimits_NoLimits(t *testing.T) {
	usage := &UsageSummary{TotalTokens: 1000}
	limits := &UsageLimits{}

	status := CheckLimits(usage, limits)

	if status.IsApproachingLimit {
		t.Error("Should not be approaching limit with no limits set")
	}
	if status.IsOverLimit {
		t.Error("Should not be over limit with no limits set")
	}
}

func TestCheckLimits_NilInputs(t *testing.T) {
	status := CheckLimits(nil, nil)
	if status.IsApproachingLimit || status.IsOverLimit {
		t.Error("Should not have warnings with nil inputs")
	}

	status = CheckLimits(&UsageSummary{}, nil)
	if status.IsApproachingLimit || status.IsOverLimit {
		t.Error("Should not have warnings with nil limits")
	}
}

func TestCheckLimits_TokenLimit(t *testing.T) {
	limits := &UsageLimits{
		MaxTokensPerDay:         1000,
		WarningThresholdPercent: 80,
	}

	tests := []struct {
		name              string
		tokens            int64
		wantApproaching   bool
		wantOver          bool
		wantWarningCount  int
		wantTokensPercent float64
	}{
		{
			name:              "under threshold",
			tokens:            500,
			wantApproaching:   false,
			wantOver:          false,
			wantWarningCount:  0,
			wantTokensPercent: 50,
		},
		{
			name:              "at threshold",
			tokens:            800,
			wantApproaching:   true,
			wantOver:          false,
			wantWarningCount:  1,
			wantTokensPercent: 80,
		},
		{
			name:              "over threshold",
			tokens:            900,
			wantApproaching:   true,
			wantOver:          false,
			wantWarningCount:  1,
			wantTokensPercent: 90,
		},
		{
			name:              "at limit",
			tokens:            1000,
			wantApproaching:   false,
			wantOver:          true,
			wantWarningCount:  1,
			wantTokensPercent: 100,
		},
		{
			name:              "over limit",
			tokens:            1500,
			wantApproaching:   false,
			wantOver:          true,
			wantWarningCount:  1,
			wantTokensPercent: 150,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := &UsageSummary{TotalTokens: tt.tokens}
			status := CheckLimits(usage, limits)

			if status.IsApproachingLimit != tt.wantApproaching {
				t.Errorf("IsApproachingLimit = %v, want %v", status.IsApproachingLimit, tt.wantApproaching)
			}
			if status.IsOverLimit != tt.wantOver {
				t.Errorf("IsOverLimit = %v, want %v", status.IsOverLimit, tt.wantOver)
			}
			if len(status.Warnings) != tt.wantWarningCount {
				t.Errorf("len(Warnings) = %d, want %d", len(status.Warnings), tt.wantWarningCount)
			}
			if status.TokensUsedPercent != tt.wantTokensPercent {
				t.Errorf("TokensUsedPercent = %f, want %f", status.TokensUsedPercent, tt.wantTokensPercent)
			}
		})
	}
}

func TestCheckLimits_CostLimit(t *testing.T) {
	limits := &UsageLimits{
		MaxCostPerDayCents:      1000, // $10
		WarningThresholdPercent: 80,
	}

	usage := &UsageSummary{TotalCostCents: 850}
	status := CheckLimits(usage, limits)

	if !status.IsApproachingLimit {
		t.Error("Should be approaching cost limit at 85%")
	}
	if status.CostUsedPercent != 85 {
		t.Errorf("CostUsedPercent = %f, want 85", status.CostUsedPercent)
	}
}

func TestCheckLimits_RequestLimit(t *testing.T) {
	limits := &UsageLimits{
		MaxRequestsPerHour:      100,
		WarningThresholdPercent: 80,
	}

	usage := &UsageSummary{RequestCount: 95}
	status := CheckLimits(usage, limits)

	if !status.IsApproachingLimit {
		t.Error("Should be approaching request limit at 95%")
	}
	if status.RequestsUsedPercent != 95 {
		t.Errorf("RequestsUsedPercent = %f, want 95", status.RequestsUsedPercent)
	}
}

func TestCheckLimits_DefaultThreshold(t *testing.T) {
	limits := &UsageLimits{
		MaxTokensPerDay:         1000,
		WarningThresholdPercent: 0, // Should use default
	}

	usage := &UsageSummary{TotalTokens: 800} // 80%
	status := CheckLimits(usage, limits)

	if !status.IsApproachingLimit {
		t.Error("Should be approaching limit at 80% with default threshold")
	}
}

func TestCheckLimits_MultipleWarnings(t *testing.T) {
	limits := &UsageLimits{
		MaxTokensPerDay:         1000,
		MaxCostPerDayCents:      500,
		WarningThresholdPercent: 80,
	}

	usage := &UsageSummary{
		TotalTokens:    900,
		TotalCostCents: 450,
	}
	status := CheckLimits(usage, limits)

	if len(status.Warnings) != 2 {
		t.Errorf("Expected 2 warnings, got %d", len(status.Warnings))
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
