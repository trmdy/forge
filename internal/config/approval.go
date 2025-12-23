package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/opencode-ai/swarm/internal/models"
)

const (
	ApprovalPolicyStrict     = "strict"
	ApprovalPolicyPermissive = "permissive"
	ApprovalPolicyCustom     = "custom"

	ApprovalRuleActionApprove = "approve"
	ApprovalRuleActionDeny    = "deny"
	ApprovalRuleActionPrompt  = "prompt"
)

// ResolvedApprovalPolicy captures the effective policy and rules to apply.
type ResolvedApprovalPolicy struct {
	Mode  string
	Rules []ApprovalRule
}

// ApprovalPolicyForWorkspace resolves the effective approval policy for a workspace.
func (c *Config) ApprovalPolicyForWorkspace(ws *models.Workspace) ResolvedApprovalPolicy {
	base := ResolvedApprovalPolicy{
		Mode:  normalizeApprovalPolicy(c.AgentDefaults.ApprovalPolicy),
		Rules: c.AgentDefaults.ApprovalRules,
	}
	if base.Mode == "" {
		base.Mode = ApprovalPolicyStrict
	}

	if ws == nil {
		return base
	}

	for _, override := range c.WorkspaceOverrides {
		if !override.matchesWorkspace(ws) {
			continue
		}

		mode := normalizeApprovalPolicy(override.ApprovalPolicy)
		if mode == "" && len(override.ApprovalRules) > 0 {
			mode = ApprovalPolicyCustom
		}
		if mode == "" {
			return base
		}
		return ResolvedApprovalPolicy{
			Mode:  mode,
			Rules: override.ApprovalRules,
		}
	}

	return base
}

func (w WorkspaceOverrideConfig) matchesWorkspace(ws *models.Workspace) bool {
	if ws == nil {
		return false
	}

	if strings.TrimSpace(w.WorkspaceID) != "" && strings.EqualFold(strings.TrimSpace(w.WorkspaceID), ws.ID) {
		return true
	}

	if strings.TrimSpace(w.Name) != "" && strings.EqualFold(strings.TrimSpace(w.Name), ws.Name) {
		return true
	}

	pattern := strings.TrimSpace(w.RepoPath)
	if pattern == "" || strings.TrimSpace(ws.RepoPath) == "" {
		return false
	}

	pattern = filepath.Clean(pattern)
	target := filepath.Clean(ws.RepoPath)

	if containsGlob(pattern) {
		if ok, err := filepath.Match(pattern, target); err == nil && ok {
			return true
		}
		return false
	}

	return strings.EqualFold(pattern, target)
}

func containsGlob(value string) bool {
	return strings.ContainsAny(value, "*?[]")
}

func normalizeApprovalPolicy(policy string) string {
	return strings.ToLower(strings.TrimSpace(policy))
}

func validateApprovalPolicy(path string, policy string, rules []ApprovalRule) error {
	normalized := normalizeApprovalPolicy(policy)
	if normalized == "" {
		if len(rules) == 0 {
			return nil
		}
		normalized = ApprovalPolicyCustom
	}

	switch normalized {
	case ApprovalPolicyStrict, ApprovalPolicyPermissive, ApprovalPolicyCustom:
	default:
		return fmt.Errorf("%s.approval_policy must be strict, permissive, or custom", path)
	}

	if normalized == ApprovalPolicyCustom && len(rules) == 0 {
		return fmt.Errorf("%s.approval_policy is custom but approval_rules is empty", path)
	}

	for i, rule := range rules {
		if strings.TrimSpace(rule.RequestType) == "" {
			return fmt.Errorf("%s.approval_rules[%d].request_type is required", path, i)
		}
		action := strings.ToLower(strings.TrimSpace(rule.Action))
		switch action {
		case ApprovalRuleActionApprove, ApprovalRuleActionDeny, ApprovalRuleActionPrompt:
		default:
			return fmt.Errorf("%s.approval_rules[%d].action must be approve, deny, or prompt", path, i)
		}
	}

	return nil
}
