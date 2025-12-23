package config

import (
	"strings"
	"testing"

	"github.com/opencode-ai/swarm/internal/models"
)

func TestApprovalPolicyForWorkspace(t *testing.T) {
	cfg := DefaultConfig()
	ws := &models.Workspace{ID: "ws-1", Name: "alpha", RepoPath: "/tmp/alpha"}

	policy := cfg.ApprovalPolicyForWorkspace(ws)
	if policy.Mode != ApprovalPolicyStrict {
		t.Fatalf("expected default policy strict, got %q", policy.Mode)
	}

	cfg.WorkspaceOverrides = []WorkspaceOverrideConfig{
		{
			Name:           "alpha",
			ApprovalPolicy: ApprovalPolicyPermissive,
		},
		{
			RepoPath:       "/tmp/*",
			ApprovalPolicy: ApprovalPolicyCustom,
			ApprovalRules: []ApprovalRule{
				{RequestType: "*", Action: ApprovalRuleActionApprove},
			},
		},
	}

	policy = cfg.ApprovalPolicyForWorkspace(ws)
	if policy.Mode != ApprovalPolicyPermissive {
		t.Fatalf("expected override policy permissive, got %q", policy.Mode)
	}
}

func TestApprovalPolicyRepoPathOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkspaceOverrides = []WorkspaceOverrideConfig{
		{
			RepoPath:       "/tmp/*",
			ApprovalPolicy: ApprovalPolicyCustom,
			ApprovalRules: []ApprovalRule{
				{RequestType: "*", Action: ApprovalRuleActionApprove},
			},
		},
	}

	ws := &models.Workspace{ID: "ws-1", Name: "alpha", RepoPath: "/tmp/alpha"}
	policy := cfg.ApprovalPolicyForWorkspace(ws)
	if policy.Mode != ApprovalPolicyCustom {
		t.Fatalf("expected custom policy, got %q", policy.Mode)
	}
	if len(policy.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(policy.Rules))
	}
}

func TestApprovalPolicyValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(cfg *Config)
		wantErr string
	}{
		{
			name: "invalid default policy",
			mutate: func(cfg *Config) {
				cfg.AgentDefaults.ApprovalPolicy = "bad"
			},
			wantErr: "agent_defaults.approval_policy",
		},
		{
			name: "custom default missing rules",
			mutate: func(cfg *Config) {
				cfg.AgentDefaults.ApprovalPolicy = ApprovalPolicyCustom
			},
			wantErr: "approval_rules",
		},
		{
			name: "override missing selector",
			mutate: func(cfg *Config) {
				cfg.WorkspaceOverrides = []WorkspaceOverrideConfig{{
					ApprovalPolicy: ApprovalPolicyStrict,
				}}
			},
			wantErr: "workspace_overrides[0] must include",
		},
		{
			name: "override invalid rule action",
			mutate: func(cfg *Config) {
				cfg.WorkspaceOverrides = []WorkspaceOverrideConfig{{
					Name:           "alpha",
					ApprovalPolicy: ApprovalPolicyCustom,
					ApprovalRules: []ApprovalRule{
						{RequestType: "*", Action: "maybe"},
					},
				}}
			},
			wantErr: "approval_rules[0].action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestApprovalPolicyRulesWithoutMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkspaceOverrides = []WorkspaceOverrideConfig{
		{
			Name: "alpha",
			ApprovalRules: []ApprovalRule{
				{RequestType: "*", Action: ApprovalRuleActionApprove},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
