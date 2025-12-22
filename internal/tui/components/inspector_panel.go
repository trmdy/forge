// Package components provides reusable TUI components.
package components

import (
	"strings"

	"github.com/opencode-ai/swarm/internal/tui/styles"
)

// RenderInspectorPanel renders a titled inspector panel.
func RenderInspectorPanel(styleSet styles.Styles, title string, lines []string, width int) string {
	if width < 20 {
		width = 20
	}
	content := append([]string{styleSet.Accent.Render(title)}, lines...)
	return styleSet.Panel.Copy().Width(width).Padding(0, 1).Render(strings.Join(content, "\n"))
}
