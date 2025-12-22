// Package workspace provides helpers for workspace lifecycle management.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/opencode-ai/swarm/internal/db"
	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/node"
)

// SessionRecoveryReport summarizes orphaned session recovery results.
type SessionRecoveryReport struct {
	Imported []*models.Workspace `json:"imported"`
	Skipped  []string            `json:"skipped"`
	Failures map[string]string   `json:"failures"`
}

// RecoverOrphanedSessions imports tmux sessions without workspace records.
func (s *Service) RecoverOrphanedSessions(ctx context.Context, nodeID, prefix string) (*SessionRecoveryReport, error) {
	report := &SessionRecoveryReport{
		Failures: make(map[string]string),
	}

	if s == nil || s.repo == nil || s.nodeService == nil {
		return report, fmt.Errorf("workspace service is missing dependencies")
	}

	nodeObj, err := s.resolveRecoveryNode(ctx, nodeID)
	if err != nil {
		return report, err
	}
	if nodeObj == nil || !nodeObj.IsLocal {
		return report, fmt.Errorf("remote node tmux inspection not yet implemented")
	}

	client := s.tmuxClient()
	sessions, err := client.ListSessions(ctx)
	if err != nil {
		return report, err
	}

	prefixFilter, err := normalizeSessionPrefix(prefix)
	if err != nil {
		return report, err
	}

	for _, session := range sessions {
		name := strings.TrimSpace(session.Name)
		if name == "" {
			continue
		}
		if prefixFilter != "" && !strings.HasPrefix(strings.ToLower(name), prefixFilter) {
			report.Skipped = append(report.Skipped, name)
			continue
		}

		if _, err := s.repo.GetByTmuxSession(ctx, nodeObj.ID, name); err == nil {
			continue
		} else if !errors.Is(err, db.ErrWorkspaceNotFound) {
			return report, err
		}

		ws, err := s.ImportWorkspace(ctx, ImportWorkspaceInput{
			NodeID:      nodeObj.ID,
			TmuxSession: name,
		})
		if err != nil {
			if errors.Is(err, ErrWorkspaceAlreadyExists) {
				report.Skipped = append(report.Skipped, name)
				continue
			}
			report.Failures[name] = err.Error()
			continue
		}

		report.Imported = append(report.Imported, ws)
	}

	return report, nil
}

func normalizeSessionPrefix(prefix string) (string, error) {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return "", nil
	}

	normalized := sanitizeTmuxName(trimmed)
	if normalized == "" {
		return "", fmt.Errorf("tmux prefix has no valid characters: %q", prefix)
	}

	return normalized + "-", nil
}

func (s *Service) resolveRecoveryNode(ctx context.Context, nodeID string) (*models.Node, error) {
	if strings.TrimSpace(nodeID) == "" {
		nodes, err := s.nodeService.ListNodes(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list nodes: %w", err)
		}
		for _, n := range nodes {
			if n != nil && n.IsLocal {
				return n, nil
			}
		}
		return nil, ErrNodeNotFound
	}

	nodeObj, err := s.nodeService.GetNode(ctx, nodeID)
	if err != nil {
		if errors.Is(err, node.ErrNodeNotFound) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	return nodeObj, nil
}
