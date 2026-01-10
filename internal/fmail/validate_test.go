package fmail

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeTopic(t *testing.T) {
	normalized, err := NormalizeTopic("Build-Status")
	require.NoError(t, err)
	require.Equal(t, "build-status", normalized)
}

func TestValidateTopic(t *testing.T) {
	valid := []string{"task", "build-status", "a1", "status123"}
	for _, name := range valid {
		require.NoError(t, ValidateTopic(name))
	}

	invalid := []string{"Task", "task_ok", "task status", "@task", "", "TASK"}
	for _, name := range invalid {
		require.Error(t, ValidateTopic(name))
	}
}

func TestNormalizeAgentName(t *testing.T) {
	normalized, err := NormalizeAgentName("Reviewer-1")
	require.NoError(t, err)
	require.Equal(t, "reviewer-1", normalized)
}

func TestValidateAgentName(t *testing.T) {
	valid := []string{"architect", "coder-1", "reviewer"}
	for _, name := range valid {
		require.NoError(t, ValidateAgentName(name))
	}

	invalid := []string{"Reviewer", "agent_1", "agent 1", "", "@agent"}
	for _, name := range invalid {
		require.Error(t, ValidateAgentName(name))
	}
}

func TestNormalizeTarget(t *testing.T) {
	target, isDM, err := NormalizeTarget("@Reviewer")
	require.NoError(t, err)
	require.True(t, isDM)
	require.Equal(t, "@reviewer", target)

	target, isDM, err = NormalizeTarget("Task")
	require.NoError(t, err)
	require.False(t, isDM)
	require.Equal(t, "task", target)
}
