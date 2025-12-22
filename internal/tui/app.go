// Package tui implements the Swarm terminal user interface.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/state"
	"github.com/opencode-ai/swarm/internal/tui/components"
	"github.com/opencode-ai/swarm/internal/tui/styles"
)

const tuiSubscriberID = "tui-main"

// Config contains TUI configuration.
type Config struct {
	// StateEngine for subscribing to state changes.
	StateEngine *state.Engine
}

// Run launches the Swarm TUI program.
func Run() error {
	return RunWithConfig(Config{})
}

// RunWithConfig launches the Swarm TUI program with configuration.
func RunWithConfig(cfg Config) error {
	m := initialModel()
	m.stateEngine = cfg.StateEngine

	program := tea.NewProgram(m, tea.WithAltScreen())

	// If we have a state engine, set up subscription after program starts
	if cfg.StateEngine != nil {
		go func() {
			// Small delay to let the program initialize
			time.Sleep(50 * time.Millisecond)
			cmd := SubscribeToStateChanges(cfg.StateEngine, tuiSubscriberID)(program)
			if cmd != nil {
				program.Send(cmd())
			}
		}()
	}

	_, err := program.Run()

	// Clean up subscription on exit
	if cfg.StateEngine != nil {
		_ = cfg.StateEngine.Unsubscribe(tuiSubscriberID)
	}

	return err
}

type model struct {
	width        int
	height       int
	styles       styles.Styles
	view         viewID
	selectedView viewID
	showHelp     bool
	paletteOpen  bool
	paletteQuery string
	paletteIndex int
	lastUpdated  time.Time
	stale        bool
	stateEngine  *state.Engine
	agentStates  map[string]models.AgentState
	stateChanges []StateChangeMsg
	nodes        []nodeSummary
	nodesPreview bool
}

type nodeSummary struct {
	Name        string
	Status      models.NodeStatus
	AgentCount  int
	LoadPercent int
	Alerts      int
}

const (
	minWidth   = 60
	minHeight  = 15
	staleAfter = 30 * time.Second
)

func initialModel() model {
	now := time.Now()
	return model{
		styles:       styles.DefaultStyles(),
		view:         viewDashboard,
		selectedView: viewDashboard,
		lastUpdated:  now,
		agentStates:  make(map[string]models.AgentState),
		stateChanges: make([]StateChangeMsg, 0),
		nodes:        sampleNodes(),
		nodesPreview: true,
	}
}

func (m model) Init() tea.Cmd {
	return staleCheckCmd(m.lastUpdated)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.paletteOpen {
			return m.updatePalette(msg)
		}
		switch msg.String() {
		case "1":
			m.view = viewDashboard
			m.selectedView = m.view
		case "2":
			m.view = viewWorkspace
			m.selectedView = m.view
		case "3":
			m.view = viewAgent
			m.selectedView = m.view
		case "g":
			m.view = nextView(m.view)
			m.selectedView = m.view
		case "left", "up":
			m.selectedView = prevView(m.selectedView)
		case "right", "down":
			m.selectedView = nextView(m.selectedView)
		case "enter":
			m.view = m.selectedView
		case "ctrl+k", ":":
			m.openPalette()
		case "?":
			m.showHelp = !m.showHelp
		case "r":
			m.lastUpdated = time.Now()
			m.stale = false
			return m, staleCheckCmd(m.lastUpdated)
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case StateChangeMsg:
		// Update agent state tracking
		m.agentStates[msg.AgentID] = msg.CurrentState
		m.lastUpdated = msg.Timestamp
		m.stale = false

		// Keep recent state changes for display (max 10)
		m.stateChanges = append(m.stateChanges, msg)
		if len(m.stateChanges) > 10 {
			m.stateChanges = m.stateChanges[1:]
		}
		return m, staleCheckCmd(msg.Timestamp)
	case staleMsg:
		if msg.Since.Equal(m.lastUpdated) && !m.stale {
			m.stale = true
		}
	case SubscriptionErrorMsg:
		// Log subscription errors (for now just ignore)
		// In production, might show a status indicator
	}
	return m, nil
}

func (m model) View() string {
	if m.width > 0 && m.height > 0 {
		if m.width < minWidth || m.height < minHeight {
			return fmt.Sprintf("%s\n", joinLines(m.smallViewLines()))
		}
	}

	lines := []string{
		m.styles.Title.Render("Swarm TUI (preview)"),
		m.breadcrumbLine(),
		m.navLine(),
		"",
	}

	if m.paletteOpen {
		lines = append(lines, m.paletteLines()...)
		lines = append(lines, "")
	}

	if m.showHelp {
		lines = append(lines, m.helpLines()...)
		lines = append(lines, "")
	}

	lines = append(lines, m.viewLines()...)
	lines = append(lines, "", m.lastUpdatedView())
	lines = append(lines, "", m.styles.Muted.Render("Press q to quit."))

	if m.width > 0 && m.height > 0 {
		lines = append(lines, "")
		lines = append(lines, m.styles.Muted.Render(fmt.Sprintf("Viewport: %dx%d", m.width, m.height)))
	}

	lines = append(lines, "", m.styles.Muted.Render("Shortcuts: arrows select | enter open | q quit | ? help | g goto | / search | r refresh | 1/2/3 views"))

	return fmt.Sprintf("%s\n", joinLines(lines))
}

func (m model) smallViewLines() []string {
	message := fmt.Sprintf("Terminal too small (%dx%d).", m.width, m.height)
	hint := fmt.Sprintf("Resize to at least %dx%d.", minWidth, minHeight)

	return []string{
		m.styles.Warning.Render(message),
		m.styles.Muted.Render(hint),
		m.styles.Muted.Render("Press q to quit."),
	}
}

type viewID int

const (
	viewDashboard viewID = iota
	viewWorkspace
	viewAgent
)

func nextView(current viewID) viewID {
	switch current {
	case viewDashboard:
		return viewWorkspace
	case viewWorkspace:
		return viewAgent
	default:
		return viewDashboard
	}
}

func prevView(current viewID) viewID {
	switch current {
	case viewDashboard:
		return viewAgent
	case viewWorkspace:
		return viewDashboard
	default:
		return viewWorkspace
	}
}

func (m model) viewLines() []string {
	switch m.view {
	case viewWorkspace:
		lines := []string{
			m.styles.Accent.Render("Workspace view"),
			m.styles.Text.Render("Workspace cards (sample data)"),
			"",
		}
		for _, card := range sampleWorkspaceCards() {
			lines = append(lines, components.RenderWorkspaceCard(m.styles, card), "")
		}
		return lines
	case viewAgent:
		lines := []string{
			m.styles.Accent.Render("Agent view"),
		}
		if len(m.agentStates) == 0 {
			lines = append(lines, m.styles.Muted.Render("No agents tracked yet."))
		} else {
			lines = append(lines, m.styles.Text.Render(fmt.Sprintf("Tracking %d agent(s):", len(m.agentStates))))
			for id, state := range m.agentStates {
				badge := components.RenderAgentStateBadge(m.styles, state)
				lines = append(lines, fmt.Sprintf("  %s: %s", shortID(id), badge))
			}
		}
		return lines
	default:
		return []string{m.dashboardView()}
	}
}

func (m model) dashboardView() string {
	leftWidth, rightWidth, gap := m.dashboardWidths()
	nodesPanel := m.renderNodesPanel(leftWidth)
	activityPanel := m.renderActivityPanel(rightWidth)
	spacer := strings.Repeat(" ", gap)
	return lipgloss.JoinHorizontal(lipgloss.Top, nodesPanel, spacer, activityPanel)
}

func (m model) dashboardWidths() (leftWidth, rightWidth, gap int) {
	total := m.width
	if total == 0 {
		total = 96
	}

	gap = 1
	leftWidth = clampInt(total/3, 24, 36)
	rightWidth = total - leftWidth - gap
	if rightWidth < 30 {
		rightWidth = 30
		leftWidth = total - rightWidth - gap
	}
	if leftWidth < 24 {
		leftWidth = 24
	}
	return leftWidth, rightWidth, gap
}

func (m model) renderNodesPanel(width int) string {
	contentWidth := panelContentWidth(width)
	lines := []string{m.nodeHeaderRow(contentWidth)}

	if len(m.nodes) == 0 {
		lines = append(lines, m.styles.Muted.Render("No nodes registered yet."))
		lines = append(lines, m.styles.Muted.Render("Run `swarm node add`."))
	} else {
		for _, node := range m.nodes {
			lines = append(lines, m.nodeRow(contentWidth, node))
		}
	}

	title := "Nodes"
	if m.nodesPreview {
		title = "Nodes (preview)"
	}
	return m.renderPanel(title, width, lines)
}

func (m model) renderActivityPanel(width int) string {
	lines := make([]string, 0, 6)
	if len(m.stateChanges) > 0 {
		for i := len(m.stateChanges) - 1; i >= 0 && i >= len(m.stateChanges)-5; i-- {
			change := m.stateChanges[i]
			prevBadge := components.RenderAgentStateBadge(m.styles, change.PreviousState)
			currBadge := components.RenderAgentStateBadge(m.styles, change.CurrentState)
			line := fmt.Sprintf("%s: %s -> %s", shortID(change.AgentID), prevBadge, currBadge)
			lines = append(lines, line)
		}
	} else {
		lines = append(lines, m.styles.Muted.Render("No state changes received yet."))
		if m.stateEngine == nil {
			lines = append(lines, m.styles.Warning.Render("(State engine not configured)"))
		}
	}
	return m.renderPanel("Activity", width, lines)
}

func (m model) renderPanel(title string, width int, lines []string) string {
	content := append([]string{m.styles.Accent.Render(title)}, lines...)
	return m.styles.Panel.Copy().Width(width).Padding(0, 1).Render(joinLines(content))
}

func panelContentWidth(width int) int {
	contentWidth := width - 4
	if contentWidth < 10 {
		return 10
	}
	return contentWidth
}

type nodeColumnWidths struct {
	status int
	name   int
	agents int
	load   int
	alerts int
}

func nodeColumns(total int) nodeColumnWidths {
	gap := 1
	cols := nodeColumnWidths{
		status: 2,
		agents: 4,
		load:   4,
		alerts: 4,
	}
	fixed := cols.status + cols.agents + cols.load + cols.alerts + gap*4
	cols.name = total - fixed
	if cols.name < 1 {
		cols.name = 1
	}
	return cols
}

func (m model) nodeHeaderRow(width int) string {
	cols := nodeColumns(width)
	return m.formatNodeRow(
		cols,
		m.styles.Muted.Render("S"),
		m.styles.Muted.Render("Node"),
		m.styles.Muted.Render("Agt"),
		m.styles.Muted.Render("Load"),
		m.styles.Muted.Render("Alert"),
	)
}

func (m model) nodeRow(width int, node nodeSummary) string {
	cols := nodeColumns(width)
	status := renderNodeStatus(m.styles, node.Status)
	agents := fmt.Sprintf("%da", node.AgentCount)
	load := renderNodeLoad(m.styles, node.LoadPercent)
	alerts := renderNodeAlerts(m.styles, node.Alerts)
	return m.formatNodeRow(cols, status, node.Name, agents, load, alerts)
}

func (m model) formatNodeRow(cols nodeColumnWidths, status, name, agents, load, alerts string) string {
	sep := " "
	return padCell(cols.status, status, lipgloss.Left) +
		sep + padCell(cols.name, name, lipgloss.Left) +
		sep + padCell(cols.agents, agents, lipgloss.Right) +
		sep + padCell(cols.load, load, lipgloss.Right) +
		sep + padCell(cols.alerts, alerts, lipgloss.Right)
}

func padCell(width int, value string, align lipgloss.Position) string {
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Align(align).Render(value)
}

func renderNodeStatus(styleSet styles.Styles, status models.NodeStatus) string {
	switch status {
	case models.NodeStatusOnline:
		return styleSet.Success.Render("O")
	case models.NodeStatusOffline:
		return styleSet.Error.Render("X")
	default:
		return styleSet.Muted.Render("?")
	}
}

func renderNodeLoad(styleSet styles.Styles, percent int) string {
	if percent < 0 {
		return styleSet.Muted.Render("--")
	}
	return fmt.Sprintf("%d%%", percent)
}

func renderNodeAlerts(styleSet styles.Styles, count int) string {
	if count <= 0 {
		return styleSet.Muted.Render("-")
	}
	return styleSet.Warning.Render(fmt.Sprintf("!%d", count))
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := lines[0]
	for _, line := range lines[1:] {
		out += "\n" + line
	}
	return out
}

func (m model) breadcrumbLine() string {
	parts := []string{
		m.styles.Muted.Render("Dashboard"),
		m.styles.Muted.Render("Workspace"),
		m.styles.Muted.Render("Agent"),
	}

	switch m.view {
	case viewDashboard:
		parts[0] = m.styles.Accent.Render("Dashboard")
	case viewWorkspace:
		parts[1] = m.styles.Accent.Render("Workspace")
	case viewAgent:
		parts[2] = m.styles.Accent.Render("Agent")
	}

	return fmt.Sprintf("%s > %s > %s", parts[0], parts[1], parts[2])
}

func (m model) navLine() string {
	dash := m.navLabel(viewDashboard)
	ws := m.navLabel(viewWorkspace)
	agent := m.navLabel(viewAgent)
	return fmt.Sprintf("Navigate: %s  %s  %s", dash, ws, agent)
}

func (m model) navLabel(view viewID) string {
	label := viewLabel(view)
	if m.selectedView == view {
		return m.styles.Accent.Render("[" + label + "]")
	}
	if m.view == view {
		return m.styles.Text.Render(label)
	}
	return m.styles.Muted.Render(label)
}

func viewLabel(view viewID) string {
	switch view {
	case viewDashboard:
		return "Dashboard"
	case viewWorkspace:
		return "Workspace"
	case viewAgent:
		return "Agent"
	default:
		return "Unknown"
	}
}

func (m model) helpLines() []string {
	return []string{
		m.styles.Accent.Render("Help"),
		m.styles.Muted.Render("Arrows: select view"),
		m.styles.Muted.Render("Enter: open selected view"),
		m.styles.Muted.Render("g: cycle views"),
		m.styles.Muted.Render("q: quit"),
	}
}

type paletteAction struct {
	ID    string
	Label string
	Hint  string
}

func (m *model) openPalette() {
	m.paletteOpen = true
	m.paletteQuery = ""
	m.paletteIndex = 0
}

func (m *model) closePalette() {
	m.paletteOpen = false
	m.paletteQuery = ""
	m.paletteIndex = 0
}

func (m model) paletteLines() []string {
	actions := m.filteredPaletteActions()
	lines := []string{
		m.styles.Accent.Render("Command Palette"),
		m.styles.Muted.Render("Type to filter. Enter to run. Esc to close."),
		m.styles.Text.Render(fmt.Sprintf("> %s", m.paletteQuery)),
	}

	if len(actions) == 0 {
		lines = append(lines, m.styles.Muted.Render("No matches."))
		return lines
	}

	for i, action := range actions {
		prefix := "  "
		label := action.Label
		if action.Hint != "" {
			label = fmt.Sprintf("%s — %s", action.Label, action.Hint)
		}
		if i == m.paletteIndex {
			lines = append(lines, m.styles.Accent.Render(prefix+label))
		} else {
			lines = append(lines, m.styles.Muted.Render(prefix+label))
		}
	}

	return lines
}

func (m model) paletteActions() []paletteAction {
	return []paletteAction{
		{ID: "view.dashboard", Label: "Go to Dashboard"},
		{ID: "view.workspace", Label: "Go to Workspace"},
		{ID: "view.agent", Label: "Go to Agent"},
		{ID: "refresh", Label: "Refresh", Hint: "update timestamp"},
		{ID: "toggle.help", Label: "Toggle Help"},
		{ID: "quit", Label: "Quit"},
	}
}

func (m model) filteredPaletteActions() []paletteAction {
	query := strings.TrimSpace(strings.ToLower(m.paletteQuery))
	actions := m.paletteActions()
	if query == "" {
		return actions
	}

	tokens := strings.Fields(query)
	filtered := make([]paletteAction, 0, len(actions))
	for _, action := range actions {
		haystack := strings.ToLower(action.Label + " " + action.Hint)
		matched := true
		for _, token := range tokens {
			if !strings.Contains(haystack, token) {
				matched = false
				break
			}
		}
		if matched {
			filtered = append(filtered, action)
		}
	}
	return filtered
}

func (m model) paletteSelectionMax() int {
	actions := m.filteredPaletteActions()
	if len(actions) == 0 {
		return 0
	}
	return len(actions) - 1
}

func (m *model) clampPaletteIndex() {
	max := m.paletteSelectionMax()
	if m.paletteIndex < 0 {
		m.paletteIndex = 0
	}
	if m.paletteIndex > max {
		m.paletteIndex = max
	}
}

func (m model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyRunes:
		m.paletteQuery += string(msg.Runes)
		m.paletteIndex = 0
	case tea.KeyBackspace, tea.KeyDelete:
		m.paletteQuery = trimLastRune(m.paletteQuery)
		m.paletteIndex = 0
	}

	switch msg.String() {
	case "esc":
		m.closePalette()
	case "enter":
		actions := m.filteredPaletteActions()
		if len(actions) == 0 {
			m.closePalette()
			return m, nil
		}
		action := actions[m.paletteIndex]
		m.closePalette()
		return m, m.applyPaletteAction(action)
	case "up":
		m.paletteIndex--
	case "down":
		m.paletteIndex++
	case "ctrl+n":
		m.paletteIndex++
	case "ctrl+p":
		m.paletteIndex--
	}

	m.clampPaletteIndex()
	return m, nil
}

func (m *model) applyPaletteAction(action paletteAction) tea.Cmd {
	switch action.ID {
	case "view.dashboard":
		m.view = viewDashboard
		m.selectedView = viewDashboard
	case "view.workspace":
		m.view = viewWorkspace
		m.selectedView = viewWorkspace
	case "view.agent":
		m.view = viewAgent
		m.selectedView = viewAgent
	case "refresh":
		m.lastUpdated = time.Now()
		m.stale = false
		return staleCheckCmd(m.lastUpdated)
	case "toggle.help":
		m.showHelp = !m.showHelp
	case "quit":
		return tea.Quit
	}
	return nil
}

func trimLastRune(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	return string(runes[:len(runes)-1])
}

type staleMsg struct {
	Since time.Time
}

func staleCheckCmd(since time.Time) tea.Cmd {
	if since.IsZero() {
		return nil
	}
	return tea.Tick(staleAfter, func(time.Time) tea.Msg {
		return staleMsg{Since: since}
	})
}

func (m model) lastUpdatedLine() string {
	if m.lastUpdated.IsZero() {
		return "Last updated: --"
	}
	label := m.lastUpdated.Format("15:04:05")
	if m.stale {
		label += " (stale)"
	}
	return fmt.Sprintf("Last updated: %s", label)
}

func (m model) lastUpdatedView() string {
	line := m.lastUpdatedLine()
	if m.stale {
		return m.styles.Warning.Render(line)
	}
	return m.styles.Muted.Render(line)
}

func shortID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

func sampleNodes() []nodeSummary {
	return []nodeSummary{
		{
			Name:        "local",
			Status:      models.NodeStatusOnline,
			AgentCount:  3,
			LoadPercent: 45,
			Alerts:      0,
		},
		{
			Name:        "gpu-box",
			Status:      models.NodeStatusOnline,
			AgentCount:  2,
			LoadPercent: 80,
			Alerts:      1,
		},
		{
			Name:        "prod-02",
			Status:      models.NodeStatusOffline,
			AgentCount:  0,
			LoadPercent: 0,
			Alerts:      2,
		},
	}
}

func sampleWorkspaceCards() []components.WorkspaceCard {
	return []components.WorkspaceCard{
		{
			Repo:          "api-service",
			Node:          "local",
			Branch:        "main",
			Pulse:         "active",
			AgentsWorking: 2,
			AgentsIdle:    1,
			AgentsBlocked: 0,
		},
		{
			Repo:          "frontend-ui",
			Node:          "gpu-box",
			Branch:        "feature/redesign",
			Pulse:         "idle",
			AgentsWorking: 0,
			AgentsIdle:    2,
			AgentsBlocked: 1,
			Alerts:        []string{"Approval needed"},
		},
		{
			Repo:          "data-pipeline",
			Node:          "prod-02",
			Branch:        "release/1.4",
			Pulse:         "active",
			AgentsWorking: 3,
			AgentsIdle:    0,
			AgentsBlocked: 1,
			Alerts:        []string{"Rate limited"},
		},
	}
}
