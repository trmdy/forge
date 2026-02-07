package looptui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/loop"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/names"
)

const (
	defaultRefreshInterval = 2 * time.Second
	defaultLogLines        = 12
	defaultStatusTTL       = 5 * time.Second

	minWindowWidth  = 80
	minWindowHeight = 22
)

const (
	colorRunning      = "#22C55E"
	colorWaiting      = "#F59E0B"
	colorStopped      = "#6B7280"
	colorError        = "#EF4444"
	colorPaused       = "#3B82F6"
	colorLeftBorder   = "#334155"
	colorRightBorder  = "#0EA5E9"
	colorSelectedBG   = "#1E293B"
	colorSelectedFG   = "#E2E8F0"
	colorFocusOutline = "#38BDF8"
	colorWarnBorder   = "#F97316"
	colorDanger       = "#EF4444"
)

var filterStatusOptions = []string{"all", "running", "sleeping", "waiting", "stopped", "error"}

// Config controls loop TUI behavior.
type Config struct {
	DataDir          string
	RefreshInterval  time.Duration
	LogLines         int
	DefaultInterval  time.Duration
	DefaultPrompt    string
	DefaultPromptMsg string
	ConfigFile       string
}

// Run starts the loop TUI.
func Run(database *db.DB, cfg Config) error {
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = defaultRefreshInterval
	}
	if cfg.LogLines <= 0 {
		cfg.LogLines = defaultLogLines
	}

	model := newModel(database, cfg)
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

type uiMode int

const (
	modeMain uiMode = iota
	modeFilter
	modeExpandedLogs
	modeConfirm
	modeWizard
)

type statusKind int

const (
	statusInfo statusKind = iota
	statusOK
	statusErr
)

type filterFocus int

const (
	filterFocusText filterFocus = iota
	filterFocusStatus
)

type actionType int

const (
	actionNone actionType = iota
	actionStop
	actionKill
	actionDelete
	actionResume
	actionCreate
)

type loopView struct {
	Loop        *models.Loop
	Runs        int
	QueueDepth  int
	ProfileName string
	PoolName    string
}

type logTailView struct {
	Lines   []string
	Message string
}

type confirmState struct {
	Action actionType
	LoopID string
	Prompt string
}

type wizardValues struct {
	Name          string
	NamePrefix    string
	Count         string
	Pool          string
	Profile       string
	Prompt        string
	PromptMsg     string
	Interval      string
	MaxRuntime    string
	MaxIterations string
	Tags          string
}

type wizardState struct {
	Step   int
	Field  int
	Values wizardValues
	Error  string
}

type model struct {
	db               *db.DB
	dataDir          string
	refreshInterval  time.Duration
	logLines         int
	defaultInterval  time.Duration
	defaultPrompt    string
	defaultPromptMsg string
	configFile       string

	width  int
	height int

	loops       []loopView
	filtered    []loopView
	selectedID  string
	selectedIdx int
	selectedLog logTailView

	mode        uiMode
	filterText  string
	filterState string
	filterFocus filterFocus
	confirm     *confirmState
	wizard      wizardState

	err           error
	statusText    string
	statusKind    statusKind
	statusExpires time.Time
	actionBusy    bool
	quitting      bool
}

type refreshMsg struct {
	loops      []loopView
	selectedID string
	selected   logTailView
	err        error
}

type tickMsg struct{}

type actionRequest struct {
	Kind        actionType
	LoopID      string
	ForceDelete bool
	Wizard      wizardValues
}

type actionResultMsg struct {
	Kind           actionType
	LoopID         string
	SelectedLoopID string
	Message        string
	Err            error
}

var startLoopProcessFn = startLoopProcess

func newModel(database *db.DB, cfg Config) model {
	m := model{
		db:               database,
		dataDir:          cfg.DataDir,
		refreshInterval:  cfg.RefreshInterval,
		logLines:         cfg.LogLines,
		defaultInterval:  cfg.DefaultInterval,
		defaultPrompt:    cfg.DefaultPrompt,
		defaultPromptMsg: cfg.DefaultPromptMsg,
		configFile:       cfg.ConfigFile,
		mode:             modeMain,
		filterState:      "all",
		filterFocus:      filterFocusText,
	}
	m.wizard = newWizardState(cfg.DefaultInterval, cfg.DefaultPrompt, cfg.DefaultPromptMsg)
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(), m.tickCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, m.fetchCmd()
	case tickMsg:
		if !m.statusExpires.IsZero() && time.Now().After(m.statusExpires) {
			m.statusText = ""
		}
		return m, tea.Batch(m.fetchCmd(), m.tickCmd())
	case refreshMsg:
		m.err = msg.err
		if msg.err == nil {
			m.loops = msg.loops
			oldSelectedID := m.selectedID
			oldSelectedIdx := m.selectedIdx
			m.applyFilters(oldSelectedID, oldSelectedIdx)
			if m.selectedID == msg.selectedID {
				m.selectedLog = msg.selected
			} else if m.selectedID != "" {
				return m, m.fetchCmd()
			} else {
				m.selectedLog = logTailView{}
			}
		}
		return m, nil
	case actionResultMsg:
		m.actionBusy = false
		if msg.Err != nil {
			m.setStatus(statusErr, msg.Err.Error())
			if msg.Kind == actionCreate {
				m.mode = modeWizard
				m.wizard.Error = msg.Err.Error()
			}
			return m, nil
		}

		if msg.Kind == actionCreate {
			m.mode = modeMain
			m.wizard.Error = ""
			if msg.SelectedLoopID != "" {
				m.selectedID = msg.SelectedLoopID
			}
		}

		if msg.Message != "" {
			m.setStatus(statusOK, msg.Message)
		}
		return m, m.fetchCmd()
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

		switch m.mode {
		case modeFilter:
			return m.updateFilterMode(msg)
		case modeExpandedLogs:
			return m.updateExpandedLogsMode(msg)
		case modeConfirm:
			return m.updateConfirmMode(msg)
		case modeWizard:
			return m.updateWizardMode(msg)
		default:
			return m.updateMainMode(msg)
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	width := m.effectiveWidth()
	height := m.effectiveHeight()

	header := m.renderHeader()
	leftWidth, rightWidth := paneWidths(width)
	paneHeight := maxInt(10, height-8)

	leftPane := m.renderLeftPane(leftWidth, paneHeight)
	rightPane := m.renderRightPane(rightWidth, paneHeight)
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	parts := []string{header, body}
	if m.mode == modeFilter {
		parts = append(parts, m.renderFilterBar(width))
	}
	if m.mode == modeConfirm && m.confirm != nil {
		parts = append(parts, m.renderConfirmDialog(width))
	}
	if m.mode == modeWizard {
		parts = append(parts, m.renderWizard(width))
	}
	if m.statusText != "" {
		parts = append(parts, m.renderStatusLine(width))
	}

	if m.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorError)).Bold(true)
		parts = append(parts, errStyle.Render("Error: "+m.err.Error()))
	}

	return strings.Join(parts, "\n")
}

func (m model) updateMainMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "/":
		m.mode = modeFilter
		m.filterFocus = filterFocusText
		return m, nil
	case "j", "down":
		m.moveSelection(1)
		return m, m.fetchCmd()
	case "k", "up":
		m.moveSelection(-1)
		return m, m.fetchCmd()
	case "l":
		if _, ok := m.selectedView(); !ok {
			m.setStatus(statusInfo, "No loop selected")
			return m, nil
		}
		m.mode = modeExpandedLogs
		return m, m.fetchCmd()
	case "n":
		m.mode = modeWizard
		m.wizard = newWizardState(m.defaultInterval, m.defaultPrompt, m.defaultPromptMsg)
		return m, nil
	case "r":
		view, ok := m.selectedView()
		if !ok {
			m.setStatus(statusInfo, "No loop selected")
			return m, nil
		}
		return m.runAction(actionRequest{Kind: actionResume, LoopID: view.Loop.ID})
	case "S":
		return m.enterConfirm(actionStop)
	case "K":
		return m.enterConfirm(actionKill)
	case "D":
		return m.enterConfirm(actionDelete)
	default:
		return m, nil
	}
}

func (m model) updateFilterMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.mode = modeMain
		m.filterFocus = filterFocusText
		return m, nil
	case "tab":
		if m.filterFocus == filterFocusText {
			m.filterFocus = filterFocusStatus
		} else {
			m.filterFocus = filterFocusText
		}
		return m, nil
	}

	if m.filterFocus == filterFocusStatus {
		switch msg.String() {
		case "left", "up", "k":
			m.cycleFilterStatus(-1)
			return m, nil
		case "right", "down", "j", "enter":
			m.cycleFilterStatus(1)
			return m, nil
		default:
			return m, nil
		}
	}

	switch msg.String() {
	case "backspace", "ctrl+h", "delete":
		if m.filterText != "" {
			m.filterText = removeLastRune(m.filterText)
			oldID, oldIdx := m.selectedID, m.selectedIdx
			m.applyFilters(oldID, oldIdx)
			return m, m.fetchCmd()
		}
		return m, nil
	case "space":
		m.filterText += " "
		oldID, oldIdx := m.selectedID, m.selectedIdx
		m.applyFilters(oldID, oldIdx)
		return m, m.fetchCmd()
	default:
		if len(msg.Runes) > 0 {
			m.filterText += string(msg.Runes)
			oldID, oldIdx := m.selectedID, m.selectedIdx
			m.applyFilters(oldID, oldIdx)
			return m, m.fetchCmd()
		}
		return m, nil
	}
}

func (m model) updateExpandedLogsMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.mode = modeMain
		return m, m.fetchCmd()
	case "j", "down":
		m.moveSelection(1)
		return m, m.fetchCmd()
	case "k", "up":
		m.moveSelection(-1)
		return m, m.fetchCmd()
	case "/":
		m.mode = modeFilter
		m.filterFocus = filterFocusText
		return m, nil
	case "S":
		m.mode = modeMain
		return m.enterConfirm(actionStop)
	case "K":
		m.mode = modeMain
		return m.enterConfirm(actionKill)
	case "D":
		m.mode = modeMain
		return m.enterConfirm(actionDelete)
	case "r":
		view, ok := m.selectedView()
		if !ok {
			m.setStatus(statusInfo, "No loop selected")
			return m, nil
		}
		m.mode = modeMain
		return m.runAction(actionRequest{Kind: actionResume, LoopID: view.Loop.ID})
	default:
		return m, nil
	}
}

func (m model) updateConfirmMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirm == nil {
		m.mode = modeMain
		return m, nil
	}

	switch msg.String() {
	case "q", "esc", "n", "N", "enter":
		m.mode = modeMain
		m.confirm = nil
		m.setStatus(statusInfo, "Action cancelled")
		return m, nil
	case "y", "Y":
		confirm := m.confirm
		m.mode = modeMain
		m.confirm = nil
		req := actionRequest{Kind: confirm.Action, LoopID: confirm.LoopID, ForceDelete: confirm.Action == actionDelete && strings.Contains(confirm.Prompt, "Force delete")}
		return m.runAction(req)
	default:
		return m, nil
	}
}

func (m model) updateWizardMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.mode = modeMain
		m.wizard.Error = ""
		return m, nil
	case "tab", "down", "j":
		m.wizardNextField()
		return m, nil
	case "shift+tab", "up", "k":
		m.wizardPrevField()
		return m, nil
	case "enter":
		if m.wizard.Step < 4 {
			if err := validateWizardStep(m.wizard.Step, m.wizard.Values, m.defaultInterval); err != nil {
				m.wizard.Error = err.Error()
				return m, nil
			}
			m.wizard.Step++
			m.wizard.Field = 0
			m.wizard.Error = ""
			return m, nil
		}
		return m.runAction(actionRequest{Kind: actionCreate, Wizard: m.wizard.Values})
	case "b", "left":
		if m.wizard.Step > 1 {
			m.wizard.Step--
			m.wizard.Field = 0
			m.wizard.Error = ""
		}
		return m, nil
	case "backspace", "ctrl+h", "delete":
		if m.wizard.Step > 3 {
			return m, nil
		}
		key := wizardFieldKey(m.wizard.Step, m.wizard.Field)
		if key == "" {
			return m, nil
		}
		value := wizardGet(&m.wizard.Values, key)
		wizardSet(&m.wizard.Values, key, removeLastRune(value))
		return m, nil
	case "space":
		if m.wizard.Step > 3 {
			return m, nil
		}
		key := wizardFieldKey(m.wizard.Step, m.wizard.Field)
		if key == "" {
			return m, nil
		}
		wizardSet(&m.wizard.Values, key, wizardGet(&m.wizard.Values, key)+" ")
		return m, nil
	default:
		if m.wizard.Step > 3 || len(msg.Runes) == 0 {
			return m, nil
		}
		key := wizardFieldKey(m.wizard.Step, m.wizard.Field)
		if key == "" {
			return m, nil
		}
		wizardSet(&m.wizard.Values, key, wizardGet(&m.wizard.Values, key)+string(msg.Runes))
		return m, nil
	}
}

func (m model) runAction(req actionRequest) (tea.Model, tea.Cmd) {
	if m.actionBusy {
		m.setStatus(statusInfo, "Another action is still running")
		return m, nil
	}

	m.actionBusy = true
	switch req.Kind {
	case actionCreate:
		m.setStatus(statusInfo, "Creating loop(s)...")
	case actionResume:
		m.setStatus(statusInfo, "Resuming loop...")
	case actionStop:
		m.setStatus(statusInfo, "Requesting graceful stop...")
	case actionKill:
		m.setStatus(statusInfo, "Killing loop...")
	case actionDelete:
		m.setStatus(statusInfo, "Deleting loop record...")
	default:
		m.setStatus(statusInfo, "Running action...")
	}
	return m, m.actionCmd(req)
}

func (m model) actionCmd(req actionRequest) tea.Cmd {
	database := m.db
	dataDir := m.dataDir
	configFile := m.configFile
	defaultInterval := m.defaultInterval
	defaultPrompt := m.defaultPrompt
	defaultPromptMsg := m.defaultPromptMsg

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result := actionResultMsg{Kind: req.Kind, LoopID: req.LoopID}
		var err error
		switch req.Kind {
		case actionResume:
			result.Message, err = resumeLoop(ctx, database, configFile, req.LoopID)
		case actionStop:
			result.Message, err = stopLoop(ctx, database, req.LoopID)
		case actionKill:
			result.Message, err = killLoop(ctx, database, req.LoopID)
		case actionDelete:
			result.Message, err = deleteLoop(ctx, database, req.LoopID, req.ForceDelete)
		case actionCreate:
			result.SelectedLoopID, result.Message, err = createLoops(ctx, database, dataDir, configFile, defaultInterval, defaultPrompt, defaultPromptMsg, req.Wizard)
		default:
			err = errors.New("unsupported action")
		}
		if err != nil {
			result.Err = err
		}
		return result
	}
}

func (m model) enterConfirm(action actionType) (tea.Model, tea.Cmd) {
	view, ok := m.selectedView()
	if !ok {
		m.setStatus(statusInfo, "No loop selected")
		return m, nil
	}

	loopID := loopDisplayID(view.Loop)
	confirm := &confirmState{Action: action, LoopID: view.Loop.ID}
	switch action {
	case actionStop:
		confirm.Prompt = fmt.Sprintf("Stop loop %s after current iteration? [y/N]", loopID)
	case actionKill:
		confirm.Prompt = fmt.Sprintf("Kill loop %s immediately? [y/N]", loopID)
	case actionDelete:
		if view.Loop.State == models.LoopStateStopped {
			confirm.Prompt = fmt.Sprintf("Delete loop record %s? [y/N]", loopID)
		} else {
			confirm.Prompt = fmt.Sprintf("Loop is still running. Force delete record %s? [y/N]", loopID)
		}
	default:
		m.setStatus(statusErr, "Unsupported destructive action")
		return m, nil
	}

	m.confirm = confirm
	m.mode = modeConfirm
	return m, nil
}

func (m *model) moveSelection(delta int) {
	if len(m.filtered) == 0 {
		m.selectedIdx = 0
		m.selectedID = ""
		return
	}
	m.selectedIdx += delta
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
	if m.selectedIdx >= len(m.filtered) {
		m.selectedIdx = len(m.filtered) - 1
	}
	m.selectedID = m.filtered[m.selectedIdx].Loop.ID
}

func (m *model) cycleFilterStatus(delta int) {
	idx := 0
	for i, candidate := range filterStatusOptions {
		if candidate == m.filterState {
			idx = i
			break
		}
	}
	idx += delta
	if idx < 0 {
		idx = len(filterStatusOptions) - 1
	}
	if idx >= len(filterStatusOptions) {
		idx = 0
	}
	m.filterState = filterStatusOptions[idx]
	oldID, oldIdx := m.selectedID, m.selectedIdx
	m.applyFilters(oldID, oldIdx)
}

func (m *model) applyFilters(previousID string, previousIdx int) {
	filtered := make([]loopView, 0, len(m.loops))
	query := strings.ToLower(strings.TrimSpace(m.filterText))
	state := strings.ToLower(strings.TrimSpace(m.filterState))

	for _, view := range m.loops {
		if view.Loop == nil {
			continue
		}
		loopState := strings.ToLower(string(view.Loop.State))
		if state != "" && state != "all" && loopState != state {
			continue
		}
		if query != "" {
			idCandidate := strings.ToLower(loopDisplayID(view.Loop))
			fullID := strings.ToLower(view.Loop.ID)
			name := strings.ToLower(view.Loop.Name)
			repoPath := strings.ToLower(view.Loop.RepoPath)
			if !strings.Contains(idCandidate, query) && !strings.Contains(fullID, query) && !strings.Contains(name, query) && !strings.Contains(repoPath, query) {
				continue
			}
		}
		filtered = append(filtered, view)
	}

	m.filtered = filtered
	if len(filtered) == 0 {
		m.selectedIdx = 0
		m.selectedID = ""
		return
	}

	if previousID != "" {
		for i := range filtered {
			if filtered[i].Loop != nil && filtered[i].Loop.ID == previousID {
				m.selectedIdx = i
				m.selectedID = previousID
				return
			}
		}
	}

	if previousIdx < 0 {
		previousIdx = 0
	}
	if previousIdx >= len(filtered) {
		previousIdx = len(filtered) - 1
	}
	m.selectedIdx = previousIdx
	m.selectedID = filtered[previousIdx].Loop.ID
}

func (m model) selectedView() (loopView, bool) {
	if len(m.filtered) == 0 {
		return loopView{}, false
	}
	idx := m.selectedIdx
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.filtered) {
		idx = len(m.filtered) - 1
	}
	return m.filtered[idx], true
}

func (m *model) setStatus(kind statusKind, text string) {
	m.statusKind = kind
	m.statusText = strings.TrimSpace(text)
	m.statusExpires = time.Now().Add(defaultStatusTTL)
}

func (m model) fetchCmd() tea.Cmd {
	database := m.db
	dataDir := m.dataDir
	selectedID := m.selectedID
	logLines := m.desiredLogLines()

	if selectedID == "" && len(m.filtered) > 0 && m.selectedIdx >= 0 && m.selectedIdx < len(m.filtered) {
		selectedID = m.filtered[m.selectedIdx].Loop.ID
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		views, err := loadLoopViews(ctx, database)
		if err != nil {
			return refreshMsg{err: err}
		}

		logLoopID, tail := loadSelectedLogTail(views, selectedID, dataDir, logLines)
		return refreshMsg{
			loops:      views,
			selectedID: logLoopID,
			selected:   tail,
		}
	}
}

func (m model) desiredLogLines() int {
	lines := m.logLines
	if lines <= 0 {
		lines = defaultLogLines
	}
	if m.mode == modeExpandedLogs {
		if m.height > 0 {
			return maxInt(lines, m.height-10)
		}
		return maxInt(lines, 24)
	}
	return lines
}

func (m model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m model) effectiveWidth() int {
	if m.width <= 0 {
		return 120
	}
	return maxInt(m.width, minWindowWidth)
}

func (m model) effectiveHeight() int {
	if m.height <= 0 {
		return 34
	}
	return maxInt(m.height, minWindowHeight)
}

func (m model) renderHeader() string {
	modeName := "Main"
	switch m.mode {
	case modeFilter:
		modeName = "Filter"
	case modeExpandedLogs:
		modeName = "Expanded Logs"
	case modeConfirm:
		modeName = "Confirm"
	case modeWizard:
		modeName = "New Loop Wizard"
	}

	header := fmt.Sprintf("Forge loops | mode: %s | / filter | n new | S/K/D destructive | r resume | l logs | q quit", modeName)
	if m.actionBusy {
		header += " | action: running"
	}
	return header
}

func paneWidths(width int) (int, int) {
	left := int(float64(width) * 0.44)
	if left < 34 {
		left = 34
	}
	right := width - left - 1
	if right < 34 {
		right = 34
		left = width - right - 1
		if left < 34 {
			left = 34
		}
	}
	return left, right
}

func (m model) renderLeftPane(width, height int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorLeftBorder)).
		Padding(0, 1).
		Width(width).
		Height(height)

	contentWidth := maxInt(1, width-2)
	rows := make([]string, 0, height)
	rows = append(rows, lipgloss.NewStyle().Bold(true).Render(truncateLine("STATUS    ID        RUNS  DIR", contentWidth)))

	if len(m.filtered) == 0 {
		empty := []string{
			"No loops matched.",
			"Start one: forge up --count 1",
			"Press / to clear filter.",
		}
		for _, line := range empty {
			rows = append(rows, truncateLine(line, contentWidth))
		}
		return style.Render(strings.Join(rows, "\n"))
	}

	available := maxInt(1, height-4)
	start := 0
	if len(m.filtered) > available {
		start = m.selectedIdx - available/2
		if start < 0 {
			start = 0
		}
		if start > len(m.filtered)-available {
			start = len(m.filtered) - available
		}
	}
	end := minInt(len(m.filtered), start+available)

	for i := start; i < end; i++ {
		view := m.filtered[i]
		line := renderListRow(view, contentWidth-2)
		marker := "  "
		if i == m.selectedIdx {
			marker = lipgloss.NewStyle().Foreground(lipgloss.Color(colorFocusOutline)).Bold(true).Render("> ")
			line = lipgloss.NewStyle().
				Background(lipgloss.Color(colorSelectedBG)).
				Foreground(lipgloss.Color(colorSelectedFG)).
				Render(truncateLine(line, contentWidth-2))
		} else {
			line = truncateLine(line, contentWidth-2)
		}
		rows = append(rows, marker+line)
	}

	if start > 0 {
		rows = append(rows, truncateLine("...", contentWidth))
	}
	if end < len(m.filtered) {
		rows = append(rows, truncateLine(fmt.Sprintf("... %d more", len(m.filtered)-end), contentWidth))
	}

	return style.Render(strings.Join(rows, "\n"))
}

func renderListRow(view loopView, width int) string {
	if view.Loop == nil {
		return ""
	}
	status := strings.ToUpper(string(view.Loop.State))
	status = truncateLine(status, 8)
	statusStyled := statusStyle(view.Loop.State).Render(padRight(status, 8))
	id := truncateLine(loopDisplayID(view.Loop), 9)
	runs := fmt.Sprintf("%d", view.Runs)
	dir := filepath.Base(view.Loop.RepoPath)

	base := fmt.Sprintf("%s  %-9s %4s  %s", statusStyled, id, runs, dir)
	return truncateLine(base, width)
}

func (m model) renderRightPane(width, height int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorRightBorder)).
		Padding(0, 1).
		Width(width).
		Height(height)

	view, ok := m.selectedView()
	if !ok || view.Loop == nil {
		content := []string{
			"No loop selected.",
			"Use j/k or arrow keys to choose a loop.",
			"Start one: forge up --count 1",
		}
		return style.Render(strings.Join(content, "\n"))
	}

	if m.mode == modeExpandedLogs {
		return style.Render(m.renderExpandedLogs(view, width-2, height-2))
	}

	lines := make([]string, 0, 16)
	loopEntry := view.Loop
	lines = append(lines, fmt.Sprintf("ID: %s", loopDisplayID(loopEntry)))
	lines = append(lines, fmt.Sprintf("Name: %s", loopEntry.Name))
	lines = append(lines, fmt.Sprintf("Status: %s", strings.ToUpper(string(loopEntry.State))))
	lines = append(lines, fmt.Sprintf("Runs: %d", view.Runs))
	lines = append(lines, fmt.Sprintf("Dir: %s", loopEntry.RepoPath))
	lines = append(lines, fmt.Sprintf("Pool: %s", displayName(view.PoolName, loopEntry.PoolID)))
	lines = append(lines, fmt.Sprintf("Profile: %s", displayName(view.ProfileName, loopEntry.ProfileID)))
	lines = append(lines, fmt.Sprintf("Last Run: %s", formatTime(loopEntry.LastRunAt)))
	lines = append(lines, fmt.Sprintf("Queue Depth: %d", view.QueueDepth))
	lines = append(lines, fmt.Sprintf("Interval: %s", formatDurationSeconds(loopEntry.IntervalSeconds)))
	lines = append(lines, fmt.Sprintf("Max Runtime: %s", formatDurationSeconds(loopEntry.MaxRuntimeSeconds)))
	lines = append(lines, fmt.Sprintf("Max Iterations: %s", formatIterations(loopEntry.MaxIterations)))
	if strings.TrimSpace(loopEntry.LastError) != "" {
		lines = append(lines, fmt.Sprintf("Last Error: %s", loopEntry.LastError))
	}

	contentWidth := maxInt(1, width-2)
	for i := range lines {
		lines[i] = truncateLine(lines[i], contentWidth)
	}

	content := make([]string, 0, len(lines)+6)
	content = append(content, lines...)

	availableForLogs := height - len(lines) - 6
	if availableForLogs >= 4 {
		content = append(content, "", "Logs:")
		logLines := m.selectedLog.Lines
		if len(logLines) == 0 {
			if m.selectedLog.Message != "" {
				content = append(content, truncateLine(m.selectedLog.Message, contentWidth))
			} else {
				content = append(content, "Log is empty.")
			}
		} else {
			if len(logLines) > availableForLogs {
				logLines = logLines[len(logLines)-availableForLogs:]
			}
			for _, line := range logLines {
				content = append(content, truncateLine(line, contentWidth))
			}
		}
	}

	return style.Render(strings.Join(content, "\n"))
}

func (m model) renderExpandedLogs(view loopView, width, height int) string {
	content := []string{fmt.Sprintf("Expanded logs for %s", loopDisplayID(view.Loop)), "Press q/esc to return."}
	content = append(content, "")

	logLines := m.selectedLog.Lines
	if len(logLines) == 0 {
		if m.selectedLog.Message != "" {
			content = append(content, m.selectedLog.Message)
		} else {
			content = append(content, "Log is empty.")
		}
	} else {
		available := maxInt(1, height-len(content)-1)
		if len(logLines) > available {
			logLines = logLines[len(logLines)-available:]
		}
		for _, line := range logLines {
			content = append(content, truncateLine(line, maxInt(1, width-2)))
		}
	}

	for i := range content {
		content[i] = truncateLine(content[i], maxInt(1, width-2))
	}

	return strings.Join(content, "\n")
}

func (m model) renderFilterBar(width int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorLeftBorder)).
		Padding(0, 1).
		Width(maxInt(40, width))

	textStyle := lipgloss.NewStyle()
	statusStyle := lipgloss.NewStyle()
	if m.filterFocus == filterFocusText {
		textStyle = textStyle.Foreground(lipgloss.Color(colorFocusOutline)).Bold(true)
	} else {
		statusStyle = statusStyle.Foreground(lipgloss.Color(colorFocusOutline)).Bold(true)
	}

	textField := textStyle.Render(fmt.Sprintf("text=%q", m.filterText))
	statusParts := make([]string, 0, len(filterStatusOptions))
	for _, option := range filterStatusOptions {
		partStyle := lipgloss.NewStyle()
		if option == m.filterState {
			partStyle = partStyle.
				Foreground(lipgloss.Color(colorSelectedFG)).
				Background(lipgloss.Color(colorSelectedBG))
		}
		part := partStyle.Render(option)
		statusParts = append(statusParts, part)
	}
	statusField := statusStyle.Render("status=" + strings.Join(statusParts, " "))

	line := fmt.Sprintf("Filter mode | %s | %s | tab switches focus | esc exits", textField, statusField)
	return box.Render(truncateLine(line, maxInt(1, width-6)))
}

func (m model) renderConfirmDialog(width int) string {
	if m.confirm == nil {
		return ""
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorWarnBorder)).
		Padding(0, 1).
		Width(maxInt(40, width))

	title := lipgloss.NewStyle().Foreground(lipgloss.Color(colorDanger)).Bold(true).Render("Confirm destructive action")
	text := []string{
		title,
		m.confirm.Prompt,
		"Press y to confirm. Press n, Enter, q, or Esc to cancel.",
	}
	return box.Render(strings.Join(text, "\n"))
}

func (m model) renderWizard(width int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorRightBorder)).
		Padding(0, 1).
		Width(maxInt(40, width))

	stepLabels := []string{"1) Identity+Count", "2) Pool/Profile", "3) Prompt+Runtime", "4) Review+Submit"}
	for i := range stepLabels {
		if i+1 == m.wizard.Step {
			stepLabels[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(colorFocusOutline)).Bold(true).Render(stepLabels[i])
		}
	}

	content := []string{
		"New loop wizard",
		strings.Join(stepLabels, "  "),
		"",
	}

	switch m.wizard.Step {
	case 1:
		content = append(content,
			renderWizardField("name", m.wizard.Values.Name, m.wizard.Field == 0),
			renderWizardField("name-prefix", m.wizard.Values.NamePrefix, m.wizard.Field == 1),
			renderWizardField("count", m.wizard.Values.Count, m.wizard.Field == 2),
		)
	case 2:
		content = append(content,
			renderWizardField("pool", m.wizard.Values.Pool, m.wizard.Field == 0),
			renderWizardField("profile", m.wizard.Values.Profile, m.wizard.Field == 1),
		)
	case 3:
		content = append(content,
			renderWizardField("prompt", m.wizard.Values.Prompt, m.wizard.Field == 0),
			renderWizardField("prompt-msg", m.wizard.Values.PromptMsg, m.wizard.Field == 1),
			renderWizardField("interval", m.wizard.Values.Interval, m.wizard.Field == 2),
			renderWizardField("max-runtime", m.wizard.Values.MaxRuntime, m.wizard.Field == 3),
			renderWizardField("max-iterations", m.wizard.Values.MaxIterations, m.wizard.Field == 4),
			renderWizardField("tags", m.wizard.Values.Tags, m.wizard.Field == 5),
		)
	case 4:
		content = append(content,
			"Review:",
			fmt.Sprintf("  name=%q", m.wizard.Values.Name),
			fmt.Sprintf("  name-prefix=%q", m.wizard.Values.NamePrefix),
			fmt.Sprintf("  count=%q", m.wizard.Values.Count),
			fmt.Sprintf("  pool=%q", m.wizard.Values.Pool),
			fmt.Sprintf("  profile=%q", m.wizard.Values.Profile),
			fmt.Sprintf("  prompt=%q", m.wizard.Values.Prompt),
			fmt.Sprintf("  prompt-msg=%q", m.wizard.Values.PromptMsg),
			fmt.Sprintf("  interval=%q", m.wizard.Values.Interval),
			fmt.Sprintf("  max-runtime=%q", m.wizard.Values.MaxRuntime),
			fmt.Sprintf("  max-iterations=%q", m.wizard.Values.MaxIterations),
			fmt.Sprintf("  tags=%q", m.wizard.Values.Tags),
		)
	}

	content = append(content, "")
	content = append(content, "tab/down/up navigate fields, enter next/submit, b back, esc cancel")
	if m.wizard.Error != "" {
		content = append(content, lipgloss.NewStyle().Foreground(lipgloss.Color(colorError)).Render("Error: "+m.wizard.Error))
	}

	for i := range content {
		content[i] = truncateLine(content[i], maxInt(1, width-6))
	}

	return box.Render(strings.Join(content, "\n"))
}

func renderWizardField(label, value string, focused bool) string {
	display := value
	if strings.TrimSpace(display) == "" {
		display = "<empty>"
	}
	if focused {
		display = lipgloss.NewStyle().Foreground(lipgloss.Color(colorFocusOutline)).Render(display + "_")
	}
	return fmt.Sprintf("%s: %s", label, display)
}

func (m model) renderStatusLine(width int) string {
	style := lipgloss.NewStyle()
	switch m.statusKind {
	case statusOK:
		style = style.Foreground(lipgloss.Color(colorRunning)).Bold(true)
	case statusErr:
		style = style.Foreground(lipgloss.Color(colorError)).Bold(true)
	default:
		style = style.Foreground(lipgloss.Color(colorFocusOutline))
	}
	return style.Render(truncateLine(m.statusText, maxInt(1, width-1)))
}

func statusStyle(state models.LoopState) lipgloss.Style {
	switch state {
	case models.LoopStateRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorRunning)).Bold(true)
	case models.LoopStateWaiting, models.LoopStateSleeping:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorWaiting)).Bold(true)
	case models.LoopStateStopped:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorStopped)).Bold(true)
	case models.LoopStateError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorError)).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorPaused))
	}
}

func loadLoopViews(ctx context.Context, database *db.DB) ([]loopView, error) {
	if database == nil {
		return nil, errors.New("database is nil")
	}

	loopRepo := db.NewLoopRepository(database)
	queueRepo := db.NewLoopQueueRepository(database)
	profileRepo := db.NewProfileRepository(database)
	poolRepo := db.NewPoolRepository(database)
	runRepo := db.NewLoopRunRepository(database)

	loops, err := loopRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	profiles, _ := profileRepo.List(ctx)
	pools, _ := poolRepo.List(ctx)
	profileNames := make(map[string]string)
	poolNames := make(map[string]string)
	for _, profile := range profiles {
		profileNames[profile.ID] = profile.Name
	}
	for _, pool := range pools {
		poolNames[pool.ID] = pool.Name
	}

	views := make([]loopView, 0, len(loops))
	for _, loopEntry := range loops {
		if loopEntry == nil {
			continue
		}

		runs, _ := runRepo.CountByLoop(ctx, loopEntry.ID)
		queueItems, _ := queueRepo.List(ctx, loopEntry.ID)
		queueDepth := 0
		for _, item := range queueItems {
			if item.Status == models.LoopQueueStatusPending || item.Status == models.LoopQueueStatusDispatched {
				queueDepth++
			}
		}

		views = append(views, loopView{
			Loop:        loopEntry,
			Runs:        runs,
			QueueDepth:  queueDepth,
			ProfileName: profileNames[loopEntry.ProfileID],
			PoolName:    poolNames[loopEntry.PoolID],
		})
	}

	sort.Slice(views, func(i, j int) bool {
		left := views[i].Loop
		right := views[j].Loop
		if left == nil || right == nil {
			return i < j
		}
		return left.CreatedAt.Before(right.CreatedAt)
	})

	return views, nil
}

func loadSelectedLogTail(views []loopView, selectedID, dataDir string, maxLines int) (string, logTailView) {
	if maxLines <= 0 {
		maxLines = defaultLogLines
	}

	var selected *models.Loop
	if selectedID != "" {
		for _, view := range views {
			if view.Loop != nil && view.Loop.ID == selectedID {
				selected = view.Loop
				break
			}
		}
	}
	if selected == nil && len(views) > 0 {
		selected = views[0].Loop
	}
	if selected == nil {
		return "", logTailView{}
	}

	path := selected.LogPath
	if path == "" {
		path = loop.LogPath(dataDir, selected.Name, selected.ID)
	}

	tail, err := tailFile(path, maxLines)
	if err != nil {
		if os.IsNotExist(err) {
			return selected.ID, logTailView{Message: "Log file not found."}
		}
		return selected.ID, logTailView{Message: "Failed to read log: " + err.Error()}
	}
	if len(tail) == 0 {
		return selected.ID, logTailView{Message: "Log is empty."}
	}
	return selected.ID, logTailView{Lines: tail}
}

func tailFile(path string, maxLines int) ([]string, error) {
	if maxLines <= 0 {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimRight(string(data), "\n")
	if strings.TrimSpace(trimmed) == "" {
		return nil, nil
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines, nil
}

func stopLoop(ctx context.Context, database *db.DB, loopID string) (string, error) {
	loopRepo := db.NewLoopRepository(database)
	queueRepo := db.NewLoopQueueRepository(database)
	loopEntry, err := loopRepo.Get(ctx, loopID)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(models.StopPayload{Reason: "operator"})
	if err != nil {
		return "", err
	}
	item := &models.LoopQueueItem{Type: models.LoopQueueItemStopGraceful, Payload: payload}
	if err := queueRepo.Enqueue(ctx, loopEntry.ID, item); err != nil {
		return "", err
	}

	return fmt.Sprintf("Stop requested for loop %s", loopDisplayID(loopEntry)), nil
}

func killLoop(ctx context.Context, database *db.DB, loopID string) (string, error) {
	loopRepo := db.NewLoopRepository(database)
	queueRepo := db.NewLoopQueueRepository(database)
	loopEntry, err := loopRepo.Get(ctx, loopID)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(models.KillPayload{Reason: "operator"})
	if err != nil {
		return "", err
	}
	item := &models.LoopQueueItem{Type: models.LoopQueueItemKillNow, Payload: payload}
	if err := queueRepo.Enqueue(ctx, loopEntry.ID, item); err != nil {
		return "", err
	}

	_ = killLoopProcess(loopEntry)
	loopEntry.State = models.LoopStateStopped
	_ = loopRepo.Update(ctx, loopEntry)
	return fmt.Sprintf("Killed loop %s", loopDisplayID(loopEntry)), nil
}

func resumeLoop(ctx context.Context, database *db.DB, configFile, loopID string) (string, error) {
	loopRepo := db.NewLoopRepository(database)
	loopEntry, err := loopRepo.Get(ctx, loopID)
	if err != nil {
		return "", err
	}

	switch loopEntry.State {
	case models.LoopStateStopped, models.LoopStateError:
	default:
		return "", fmt.Errorf("loop %q is %s; only stopped or errored loops can be resumed", loopEntry.Name, loopEntry.State)
	}

	if err := startLoopProcessFn(loopEntry.ID, configFile); err != nil {
		return "", err
	}
	return fmt.Sprintf("Loop %q resumed (%s)", loopEntry.Name, loopDisplayID(loopEntry)), nil
}

func deleteLoop(ctx context.Context, database *db.DB, loopID string, force bool) (string, error) {
	loopRepo := db.NewLoopRepository(database)
	loopEntry, err := loopRepo.Get(ctx, loopID)
	if err != nil {
		return "", err
	}
	if loopEntry.State != models.LoopStateStopped && !force {
		return "", fmt.Errorf("loop %q is %s; force delete required", loopEntry.Name, loopEntry.State)
	}
	if err := loopRepo.Delete(ctx, loopEntry.ID); err != nil {
		return "", err
	}
	return fmt.Sprintf("Loop record %s deleted", loopDisplayID(loopEntry)), nil
}

func createLoops(ctx context.Context, database *db.DB, dataDir, configFile string, defaultInterval time.Duration, defaultPrompt, defaultPromptMsg string, values wizardValues) (string, string, error) {
	spec, err := buildWizardSpec(values, defaultInterval, defaultPrompt, defaultPromptMsg)
	if err != nil {
		return "", "", err
	}

	repoPath, err := resolveRepoPath("")
	if err != nil {
		return "", "", err
	}

	loopRepo := db.NewLoopRepository(database)
	poolRepo := db.NewPoolRepository(database)
	profileRepo := db.NewProfileRepository(database)

	poolID := ""
	if spec.PoolRef != "" {
		pool, err := resolvePoolByRef(ctx, poolRepo, spec.PoolRef)
		if err != nil {
			return "", "", err
		}
		poolID = pool.ID
	}

	profileID := ""
	if spec.ProfileRef != "" {
		profile, err := resolveProfileByRef(ctx, profileRepo, spec.ProfileRef)
		if err != nil {
			return "", "", err
		}
		profileID = profile.ID
	}

	promptPath := ""
	if spec.PromptRef != "" {
		resolved, err := resolvePromptPath(repoPath, spec.PromptRef)
		if err != nil {
			return "", "", err
		}
		promptPath = resolved
	}

	existing, err := loopRepo.List(ctx)
	if err != nil {
		return "", "", err
	}
	existingNames := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		existingNames[item.Name] = struct{}{}
	}

	createdIDs := make([]string, 0, spec.Count)
	for i := 0; i < spec.Count; i++ {
		name := spec.Name
		if name == "" {
			if spec.NamePrefix != "" {
				name = fmt.Sprintf("%s-%d", spec.NamePrefix, i+1)
			} else {
				name = generateLoopName(existingNames)
			}
		}
		if _, exists := existingNames[name]; exists {
			return "", "", fmt.Errorf("loop name %q already exists", name)
		}
		existingNames[name] = struct{}{}

		entry := &models.Loop{
			Name:              name,
			RepoPath:          repoPath,
			BasePromptPath:    promptPath,
			BasePromptMsg:     spec.PromptMsg,
			IntervalSeconds:   int(spec.Interval.Round(time.Second).Seconds()),
			MaxIterations:     spec.MaxIterations,
			MaxRuntimeSeconds: int(spec.MaxRuntime.Round(time.Second).Seconds()),
			PoolID:            poolID,
			ProfileID:         profileID,
			Tags:              spec.Tags,
			State:             models.LoopStateStopped,
		}

		if err := loopRepo.Create(ctx, entry); err != nil {
			return "", "", err
		}
		entry.LogPath = loop.LogPath(dataDir, entry.Name, entry.ID)
		entry.LedgerPath = loop.LedgerPath(repoPath, entry.Name, entry.ID)
		if err := loopRepo.Update(ctx, entry); err != nil {
			return "", "", err
		}
		if err := startLoopProcessFn(entry.ID, configFile); err != nil {
			return "", "", err
		}
		createdIDs = append(createdIDs, entry.ID)
	}

	selectedID := ""
	if len(createdIDs) > 0 {
		selectedID = createdIDs[len(createdIDs)-1]
	}
	return selectedID, fmt.Sprintf("Created %d loop(s)", len(createdIDs)), nil
}

type wizardSpec struct {
	Name          string
	NamePrefix    string
	Count         int
	PoolRef       string
	ProfileRef    string
	PromptRef     string
	PromptMsg     string
	Interval      time.Duration
	MaxRuntime    time.Duration
	MaxIterations int
	Tags          []string
}

func buildWizardSpec(values wizardValues, defaultInterval time.Duration, defaultPrompt, defaultPromptMsg string) (wizardSpec, error) {
	count, err := parsePositiveInt(defaultString(values.Count, "1"), "count")
	if err != nil {
		return wizardSpec{}, err
	}

	name := strings.TrimSpace(values.Name)
	namePrefix := strings.TrimSpace(values.NamePrefix)
	if name != "" && count > 1 {
		return wizardSpec{}, fmt.Errorf("name requires count=1")
	}

	poolRef := strings.TrimSpace(values.Pool)
	profileRef := strings.TrimSpace(values.Profile)
	if poolRef != "" && profileRef != "" {
		return wizardSpec{}, fmt.Errorf("use either pool or profile, not both")
	}

	interval, err := parseDurationInput(values.Interval, defaultInterval)
	if err != nil {
		return wizardSpec{}, err
	}
	if interval < 0 {
		return wizardSpec{}, fmt.Errorf("interval must be >= 0")
	}

	maxRuntime, err := parseDurationInput(values.MaxRuntime, 0)
	if err != nil {
		return wizardSpec{}, err
	}
	if maxRuntime < 0 {
		return wizardSpec{}, fmt.Errorf("max runtime must be >= 0")
	}
	if maxRuntime == 0 {
		return wizardSpec{}, fmt.Errorf("max-runtime must be > 0")
	}

	if strings.TrimSpace(values.MaxIterations) == "" {
		return wizardSpec{}, fmt.Errorf("max-iterations must be > 0")
	}
	maxIterations, err := strconv.Atoi(strings.TrimSpace(values.MaxIterations))
	if err != nil {
		return wizardSpec{}, fmt.Errorf("invalid max-iterations %q", values.MaxIterations)
	}
	if maxIterations <= 0 {
		return wizardSpec{}, fmt.Errorf("max-iterations must be > 0")
	}

	promptRef := strings.TrimSpace(values.Prompt)
	if promptRef == "" {
		promptRef = strings.TrimSpace(defaultPrompt)
	}

	promptMsg := strings.TrimSpace(values.PromptMsg)
	if promptMsg == "" {
		promptMsg = strings.TrimSpace(defaultPromptMsg)
	}

	return wizardSpec{
		Name:          name,
		NamePrefix:    namePrefix,
		Count:         count,
		PoolRef:       poolRef,
		ProfileRef:    profileRef,
		PromptRef:     promptRef,
		PromptMsg:     promptMsg,
		Interval:      interval,
		MaxRuntime:    maxRuntime,
		MaxIterations: maxIterations,
		Tags:          parseTags(values.Tags),
	}, nil
}

func validateWizardStep(step int, values wizardValues, defaultInterval time.Duration) error {
	switch step {
	case 1:
		_, err := parsePositiveInt(defaultString(values.Count, "1"), "count")
		if err != nil {
			return err
		}
		if strings.TrimSpace(values.Name) != "" && strings.TrimSpace(values.Count) != "" {
			count, _ := strconv.Atoi(strings.TrimSpace(values.Count))
			if count > 1 {
				return fmt.Errorf("name requires count=1")
			}
		}
	case 2:
		if strings.TrimSpace(values.Pool) != "" && strings.TrimSpace(values.Profile) != "" {
			return fmt.Errorf("use either pool or profile, not both")
		}
	case 3:
		if _, err := parseDurationInput(values.Interval, defaultInterval); err != nil {
			return err
		}
		maxRuntime, err := parseDurationInput(values.MaxRuntime, 0)
		if err != nil {
			return err
		}
		if maxRuntime == 0 {
			return fmt.Errorf("max-runtime must be > 0")
		}
		if strings.TrimSpace(values.MaxIterations) == "" {
			return fmt.Errorf("max-iterations must be > 0")
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(values.MaxIterations))
		if err != nil {
			return fmt.Errorf("invalid max-iterations %q", values.MaxIterations)
		}
		if parsed <= 0 {
			return fmt.Errorf("max-iterations must be > 0")
		}
	}
	return nil
}

func newWizardState(defaultInterval time.Duration, defaultPrompt, defaultPromptMsg string) wizardState {
	interval := ""
	if defaultInterval > 0 {
		interval = defaultInterval.String()
	}
	return wizardState{
		Step:  1,
		Field: 0,
		Values: wizardValues{
			Count:      "1",
			Prompt:     strings.TrimSpace(defaultPrompt),
			PromptMsg:  strings.TrimSpace(defaultPromptMsg),
			Interval:   interval,
			MaxRuntime: "",
		},
	}
}

func (m *model) wizardNextField() {
	count := wizardFieldCount(m.wizard.Step)
	if count <= 0 {
		return
	}
	m.wizard.Field = (m.wizard.Field + 1) % count
}

func (m *model) wizardPrevField() {
	count := wizardFieldCount(m.wizard.Step)
	if count <= 0 {
		return
	}
	m.wizard.Field--
	if m.wizard.Field < 0 {
		m.wizard.Field = count - 1
	}
}

func wizardFieldCount(step int) int {
	switch step {
	case 1:
		return 3
	case 2:
		return 2
	case 3:
		return 6
	default:
		return 0
	}
}

func wizardFieldKey(step, field int) string {
	switch step {
	case 1:
		switch field {
		case 0:
			return "name"
		case 1:
			return "name_prefix"
		case 2:
			return "count"
		}
	case 2:
		switch field {
		case 0:
			return "pool"
		case 1:
			return "profile"
		}
	case 3:
		switch field {
		case 0:
			return "prompt"
		case 1:
			return "prompt_msg"
		case 2:
			return "interval"
		case 3:
			return "max_runtime"
		case 4:
			return "max_iterations"
		case 5:
			return "tags"
		}
	}
	return ""
}

func wizardGet(values *wizardValues, key string) string {
	switch key {
	case "name":
		return values.Name
	case "name_prefix":
		return values.NamePrefix
	case "count":
		return values.Count
	case "pool":
		return values.Pool
	case "profile":
		return values.Profile
	case "prompt":
		return values.Prompt
	case "prompt_msg":
		return values.PromptMsg
	case "interval":
		return values.Interval
	case "max_runtime":
		return values.MaxRuntime
	case "max_iterations":
		return values.MaxIterations
	case "tags":
		return values.Tags
	default:
		return ""
	}
}

func wizardSet(values *wizardValues, key, value string) {
	switch key {
	case "name":
		values.Name = value
	case "name_prefix":
		values.NamePrefix = value
	case "count":
		values.Count = value
	case "pool":
		values.Pool = value
	case "profile":
		values.Profile = value
	case "prompt":
		values.Prompt = value
	case "prompt_msg":
		values.PromptMsg = value
	case "interval":
		values.Interval = value
	case "max_runtime":
		values.MaxRuntime = value
	case "max_iterations":
		values.MaxIterations = value
	case "tags":
		values.Tags = value
	}
}

func parsePositiveInt(value, field string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q", field, value)
	}
	if parsed < 1 {
		return 0, fmt.Errorf("%s must be at least 1", field)
	}
	return parsed, nil
}

func parseDurationInput(value string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", value)
	}
	return parsed, nil
}

func resolveRepoPath(path string) (string, error) {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
		return filepath.Abs(cwd)
	}
	return filepath.Abs(path)
}

func resolvePromptPath(repoPath, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("prompt is required")
	}

	candidate := value
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(repoPath, candidate)
	}
	if exists(candidate) {
		return candidate, nil
	}

	if !strings.HasSuffix(value, ".md") {
		candidate = filepath.Join(repoPath, ".forge", "prompts", value+".md")
	} else {
		candidate = filepath.Join(repoPath, ".forge", "prompts", value)
	}
	if exists(candidate) {
		return candidate, nil
	}

	return "", fmt.Errorf("prompt not found: %s", value)
}

func parseTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	seen := make(map[string]struct{})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func resolvePoolByRef(ctx context.Context, repo *db.PoolRepository, ref string) (*models.Pool, error) {
	pool, err := repo.GetByName(ctx, ref)
	if err == nil {
		return pool, nil
	}
	return repo.Get(ctx, ref)
}

func resolveProfileByRef(ctx context.Context, repo *db.ProfileRepository, ref string) (*models.Profile, error) {
	profile, err := repo.GetByName(ctx, ref)
	if err == nil {
		return profile, nil
	}
	return repo.Get(ctx, ref)
}

func startLoopProcess(loopID, configFile string) error {
	args := []string{"loop", "run", loopID}
	if strings.TrimSpace(configFile) != "" {
		args = append([]string{"--config", configFile}, args...)
	}

	cmd := exec.Command(os.Args[0], args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start loop process: %w", err)
	}
	return nil
}

func killLoopProcess(loopEntry *models.Loop) error {
	pid, ok := loopPID(loopEntry)
	if !ok {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGKILL); err != nil {
		_ = process.Kill()
	}
	return nil
}

func loopPID(loopEntry *models.Loop) (int, bool) {
	if loopEntry == nil || loopEntry.Metadata == nil {
		return 0, false
	}
	value, ok := loopEntry.Metadata["pid"]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case string:
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func generateLoopName(existing map[string]struct{}) string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	maxAttempts := names.LoopNameCountTwoPart() * 2

	for i := 0; i < maxAttempts; i++ {
		candidate := strings.TrimSpace(names.RandomLoopNameTwoPart(rng))
		if candidate == "" {
			continue
		}
		if _, ok := existing[candidate]; ok {
			continue
		}
		return candidate
	}

	maxAttempts = names.LoopNameCountThreePart() * 2
	for i := 0; i < maxAttempts; i++ {
		candidate := strings.TrimSpace(names.RandomLoopNameThreePart(rng))
		if candidate == "" {
			continue
		}
		if _, ok := existing[candidate]; ok {
			continue
		}
		return candidate
	}

	fallback := "loop-" + time.Now().Format("150405")
	if _, ok := existing[fallback]; !ok {
		return fallback
	}

	counter := 1
	for {
		candidate := fmt.Sprintf("%s-%d", fallback, counter)
		if _, ok := existing[candidate]; !ok {
			return candidate
		}
		counter++
	}
}

func loopDisplayID(loopEntry *models.Loop) string {
	if loopEntry == nil {
		return ""
	}
	if strings.TrimSpace(loopEntry.ShortID) != "" {
		return loopEntry.ShortID
	}
	if len(loopEntry.ID) <= 8 {
		return loopEntry.ID
	}
	return loopEntry.ID[:8]
}

func displayName(name, fallback string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "-"
}

func formatDurationSeconds(seconds int) string {
	if seconds <= 0 {
		return "-"
	}
	return (time.Duration(seconds) * time.Second).String()
}

func formatIterations(max int) string {
	if max <= 0 {
		return "unlimited"
	}
	return strconv.Itoa(max)
}

func formatTime(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}

func truncateLine(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	if width <= 3 {
		return text[:width]
	}
	plain := []rune(stripANSI(text))
	if len(plain) <= width {
		return string(plain)
	}
	return string(plain[:width-3]) + "..."
}

func padRight(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(text) >= width {
		return text[:width]
	}
	return text + strings.Repeat(" ", width-len(text))
}

func removeLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func stripANSI(in string) string {
	builder := strings.Builder{}
	builder.Grow(len(in))
	inside := false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if inside {
			if c >= '@' && c <= '~' {
				inside = false
			}
			continue
		}
		if c == 0x1b {
			inside = true
			continue
		}
		builder.WriteByte(c)
	}
	return builder.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
