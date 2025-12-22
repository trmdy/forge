// Package components provides reusable TUI components.
package components

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/tui/styles"
)

// WorkspaceGrid holds state for the workspace grid display.
type WorkspaceGrid struct {
	Workspaces    []*models.Workspace
	SelectedIndex int
	ScrollOffset  int
	Filter        string
	Width         int
	Height        int
	Columns       int
}

// NewWorkspaceGrid creates a new workspace grid.
func NewWorkspaceGrid() *WorkspaceGrid {
	return &WorkspaceGrid{
		Workspaces:    make([]*models.Workspace, 0),
		SelectedIndex: 0,
		ScrollOffset:  0,
		Columns:       2, // Default to 2 columns
	}
}

// SetWorkspaces updates the workspace list.
func (g *WorkspaceGrid) SetWorkspaces(workspaces []*models.Workspace) {
	g.Workspaces = workspaces
	g.clampSelection()
}

// FilteredWorkspaces returns workspaces matching the current filter.
func (g *WorkspaceGrid) FilteredWorkspaces() []*models.Workspace {
	if g.Filter == "" {
		return g.Workspaces
	}

	filter := strings.ToLower(g.Filter)
	filtered := make([]*models.Workspace, 0)
	for _, ws := range g.Workspaces {
		name := strings.ToLower(ws.Name)
		path := strings.ToLower(ws.RepoPath)
		if strings.Contains(name, filter) || strings.Contains(path, filter) {
			filtered = append(filtered, ws)
		}
	}
	return filtered
}

// SetFilter sets the search filter and resets selection.
func (g *WorkspaceGrid) SetFilter(filter string) {
	g.Filter = filter
	g.SelectedIndex = 0
	g.ScrollOffset = 0
}

// MoveUp moves selection up.
func (g *WorkspaceGrid) MoveUp() {
	if g.SelectedIndex >= g.Columns {
		g.SelectedIndex -= g.Columns
		g.ensureVisible()
	}
}

// MoveDown moves selection down.
func (g *WorkspaceGrid) MoveDown() {
	filtered := g.FilteredWorkspaces()
	if g.SelectedIndex+g.Columns < len(filtered) {
		g.SelectedIndex += g.Columns
		g.ensureVisible()
	}
}

// MoveLeft moves selection left.
func (g *WorkspaceGrid) MoveLeft() {
	if g.SelectedIndex > 0 {
		g.SelectedIndex--
		g.ensureVisible()
	}
}

// MoveRight moves selection right.
func (g *WorkspaceGrid) MoveRight() {
	filtered := g.FilteredWorkspaces()
	if g.SelectedIndex < len(filtered)-1 {
		g.SelectedIndex++
		g.ensureVisible()
	}
}

// SelectedWorkspace returns the currently selected workspace.
func (g *WorkspaceGrid) SelectedWorkspace() *models.Workspace {
	filtered := g.FilteredWorkspaces()
	if g.SelectedIndex < 0 || g.SelectedIndex >= len(filtered) {
		return nil
	}
	return filtered[g.SelectedIndex]
}

// clampSelection ensures selection is within bounds.
func (g *WorkspaceGrid) clampSelection() {
	filtered := g.FilteredWorkspaces()
	if len(filtered) == 0 {
		g.SelectedIndex = 0
		return
	}
	if g.SelectedIndex >= len(filtered) {
		g.SelectedIndex = len(filtered) - 1
	}
	if g.SelectedIndex < 0 {
		g.SelectedIndex = 0
	}
}

// ensureVisible adjusts scroll offset to keep selection visible.
func (g *WorkspaceGrid) ensureVisible() {
	if g.Height <= 0 {
		return
	}

	// Calculate visible rows (assuming ~3 lines per card)
	cardHeight := 4
	visibleRows := g.Height / cardHeight
	if visibleRows < 1 {
		visibleRows = 1
	}

	selectedRow := g.SelectedIndex / g.Columns

	// Scroll up if needed
	if selectedRow < g.ScrollOffset {
		g.ScrollOffset = selectedRow
	}

	// Scroll down if needed
	if selectedRow >= g.ScrollOffset+visibleRows {
		g.ScrollOffset = selectedRow - visibleRows + 1
	}
}

// Render renders the workspace grid.
func (g *WorkspaceGrid) Render(styleSet styles.Styles) string {
	filtered := g.FilteredWorkspaces()
	if len(filtered) == 0 {
		if g.Filter != "" {
			return styleSet.Muted.Render(fmt.Sprintf("No workspaces matching '%s'", g.Filter))
		}
		return styleSet.Muted.Render("No workspaces. Use 'swarm ws create' to create one.")
	}

	// Calculate card width based on available width
	cardWidth := 35
	if g.Width > 0 && g.Columns > 0 {
		cardWidth = (g.Width - (g.Columns-1)*2) / g.Columns // 2 chars gap between columns
		if cardWidth < 25 {
			cardWidth = 25
		}
		if cardWidth > 50 {
			cardWidth = 50
		}
	}

	// Build rows
	var rows []string
	for i := 0; i < len(filtered); i += g.Columns {
		var cards []string
		for j := 0; j < g.Columns && i+j < len(filtered); j++ {
			idx := i + j
			ws := filtered[idx]
			isSelected := idx == g.SelectedIndex
			card := renderGridCard(styleSet, ws, isSelected, cardWidth)
			cards = append(cards, card)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, cards...)
		rows = append(rows, row)
	}

	// Apply scroll offset
	startRow := g.ScrollOffset
	if startRow < 0 {
		startRow = 0
	}
	if startRow >= len(rows) {
		startRow = 0
	}

	visibleRows := rows
	if startRow > 0 {
		visibleRows = rows[startRow:]
	}

	grid := lipgloss.JoinVertical(lipgloss.Left, visibleRows...)

	// Add filter status if filtering
	if g.Filter != "" {
		header := styleSet.Muted.Render(fmt.Sprintf("Filter: %s (%d/%d)", g.Filter, len(filtered), len(g.Workspaces)))
		grid = header + "\n" + grid
	}

	return grid
}

// renderGridCard renders a single workspace card for the grid.
func renderGridCard(styleSet styles.Styles, ws *models.Workspace, selected bool, width int) string {
	if width < 20 {
		width = 20
	}

	// Build card content
	name := truncate(ws.Name, width-4)
	if name == "" {
		name = truncate(filepath.Base(ws.RepoPath), width-4)
	}

	// Status badge
	statusBadge := renderWorkspaceStatus(styleSet, ws.Status)

	// Branch info
	branchLine := ""
	if ws.GitInfo != nil && ws.GitInfo.Branch != "" {
		branch := truncate(ws.GitInfo.Branch, width-8)
		branchLine = styleSet.Muted.Render("  " + branch)
		if ws.GitInfo.IsDirty {
			branchLine += styleSet.Warning.Render(" *")
		}
	}

	// Agent count
	agentLine := styleSet.Text.Render(fmt.Sprintf("  Agents: %d", ws.AgentCount))

	// Alert indicator
	alertLine := ""
	if len(ws.Alerts) > 0 {
		alertLine = styleSet.Warning.Render(fmt.Sprintf("  ! %d alert(s)", len(ws.Alerts)))
	}

	// Build card
	lines := []string{
		name + " " + statusBadge,
	}
	if branchLine != "" {
		lines = append(lines, branchLine)
	}
	lines = append(lines, agentLine)
	if alertLine != "" {
		lines = append(lines, alertLine)
	}

	content := strings.Join(lines, "\n")

	// Card style
	cardStyle := lipgloss.NewStyle().
		Width(width).
		Padding(0, 1).
		Margin(0, 1, 1, 0)

	if selected {
		cardStyle = cardStyle.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")) // Accent color for selection
	} else {
		cardStyle = cardStyle.
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240"))
	}

	return cardStyle.Render(content)
}

func renderWorkspaceStatus(styleSet styles.Styles, status models.WorkspaceStatus) string {
	switch status {
	case models.WorkspaceStatusActive:
		return styleSet.Success.Render("[OK]")
	case models.WorkspaceStatusInactive:
		return styleSet.Muted.Render("[--]")
	case models.WorkspaceStatusError:
		return styleSet.Error.Render("[ERR]")
	default:
		return styleSet.Muted.Render("[?]")
	}
}

func truncate(s string, maxLen int) string {
	if maxLen < 3 {
		maxLen = 3
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
