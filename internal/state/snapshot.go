// Package state provides agent state management and change notifications.
package state

import (
	"context"
	"fmt"
	"time"

	"github.com/opencode-ai/swarm/internal/tmux"
)

// Snapshot contains captured pane content and metadata.
type Snapshot struct {
	Content    string
	Hash       string
	CapturedAt time.Time
}

// SnapshotSource captures pane content.
type SnapshotSource interface {
	CapturePane(ctx context.Context, target string, history bool) (string, error)
}

// CaptureSnapshot captures a pane snapshot and computes its hash.
func CaptureSnapshot(ctx context.Context, source SnapshotSource, target string, history bool) (*Snapshot, error) {
	if source == nil {
		return nil, fmt.Errorf("snapshot source is required")
	}
	if target == "" {
		return nil, fmt.Errorf("target is required")
	}

	content, err := source.CapturePane(ctx, target, history)
	if err != nil {
		return nil, err
	}

	return &Snapshot{
		Content:    content,
		Hash:       tmux.HashSnapshot(content),
		CapturedAt: time.Now().UTC(),
	}, nil
}
