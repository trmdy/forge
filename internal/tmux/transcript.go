// Package tmux provides tmux helpers and utilities.
package tmux

import (
	"strings"
)

// TranscriptBuffer accumulates pane snapshots with a bounded line count.
type TranscriptBuffer struct {
	lines    []string
	maxLines int
}

// NewTranscriptBuffer creates a new buffer with a max line count.
func NewTranscriptBuffer(maxLines int) *TranscriptBuffer {
	if maxLines <= 0 {
		maxLines = 1000
	}
	return &TranscriptBuffer{maxLines: maxLines}
}

// Append adds new content to the buffer, trimming to the max line count.
func (b *TranscriptBuffer) Append(content string) {
	if content == "" {
		return
	}

	newLines := strings.Split(content, "\n")
	b.lines = append(b.lines, newLines...)

	if len(b.lines) > b.maxLines {
		b.lines = b.lines[len(b.lines)-b.maxLines:]
	}
}

// Lines returns a copy of the buffered lines.
func (b *TranscriptBuffer) Lines() []string {
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

// String returns the buffer as a single string.
func (b *TranscriptBuffer) String() string {
	return strings.Join(b.lines, "\n")
}

// Reset clears the buffer.
func (b *TranscriptBuffer) Reset() {
	b.lines = nil
}
