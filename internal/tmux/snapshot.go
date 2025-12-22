// Package tmux provides tmux helpers and utilities.
package tmux

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashSnapshot returns a stable hash for captured pane content.
func HashSnapshot(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
