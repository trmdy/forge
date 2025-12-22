// Package tui implements the Swarm terminal user interface.
package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/state"
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
	lastUpdated  time.Time
	stale        bool
	stateEngine  *state.Engine
	agentStates  map[string]models.AgentState
	stateChanges []StateChangeMsg
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
		lastUpdated:  now,
		agentStates:  make(map[string]models.AgentState),
		stateChanges: make([]StateChangeMsg, 0),
	}
}

func (m model) Init() tea.Cmd {
	return staleCheckCmd(m.lastUpdated)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "1":
			m.view = viewDashboard
		case "2":
			m.view = viewWorkspace
		case "3":
			m.view = viewAgent
		case "g":
			m.view = nextView(m.view)
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
		"",
	}

	lines = append(lines, m.viewLines()...)
	lines = append(lines, "", m.styles.Muted.Render(m.lastUpdatedLine()))
	lines = append(lines, "", m.styles.Muted.Render("Press q to quit."))

	if m.width > 0 && m.height > 0 {
		lines = append(lines, "")
		lines = append(lines, m.styles.Muted.Render(fmt.Sprintf("Viewport: %dx%d", m.width, m.height)))
	}

	lines = append(lines, "", m.styles.Muted.Render("Shortcuts: q quit | ? help | g goto | / search | 1/2/3 views"))

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

func (m model) viewLines() []string {
	switch m.view {
	case viewWorkspace:
		return []string{
			m.styles.Accent.Render("Workspace view"),
			m.styles.Text.Render("Workspace routing placeholder."),
		}
	case viewAgent:
		lines := []string{
			m.styles.Accent.Render("Agent view"),
		}
		if len(m.agentStates) == 0 {
			lines = append(lines, m.styles.Muted.Render("No agents tracked yet."))
		} else {
			lines = append(lines, m.styles.Text.Render(fmt.Sprintf("Tracking %d agent(s):", len(m.agentStates))))
			for id, state := range m.agentStates {
				stateStyle := m.stateStyle(state)
				lines = append(lines, fmt.Sprintf("  %s: %s", id[:8], stateStyle.Render(string(state))))
			}
		}
		return lines
	default:
		lines := []string{
			m.styles.Accent.Render("Dashboard view"),
		}
		// Show recent state changes
		if len(m.stateChanges) > 0 {
			lines = append(lines, "")
			lines = append(lines, m.styles.Text.Render("Recent state changes:"))
			for i := len(m.stateChanges) - 1; i >= 0 && i >= len(m.stateChanges)-5; i-- {
				change := m.stateChanges[i]
				line := fmt.Sprintf("  %s: %s → %s",
					change.AgentID[:8],
					change.PreviousState,
					change.CurrentState,
				)
				lines = append(lines, m.styles.Muted.Render(line))
			}
		} else {
			lines = append(lines, m.styles.Muted.Render("No state changes received yet."))
			if m.stateEngine == nil {
				lines = append(lines, m.styles.Warning.Render("(State engine not configured)"))
			}
		}
		return lines
	}
}

// stateStyle returns the appropriate style for an agent state.
func (m model) stateStyle(agentState models.AgentState) lipgloss.Style {
	switch agentState {
	case models.AgentStateWorking:
		return m.styles.Success
	case models.AgentStateIdle:
		return m.styles.Muted
	case models.AgentStateAwaitingApproval:
		return m.styles.Warning
	case models.AgentStateError, models.AgentStateRateLimited:
		return m.styles.Error
	case models.AgentStatePaused:
		return m.styles.Accent
	default:
		return m.styles.Text
	}
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
