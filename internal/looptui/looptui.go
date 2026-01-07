package looptui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/loop"
	"github.com/tOgg1/forge/internal/models"
)

const (
	defaultRefreshInterval = 2 * time.Second
	defaultLogLines        = 12
	minCardWidth           = 32
	minCardHeight          = 10
)

type Config struct {
	DataDir         string
	RefreshInterval time.Duration
	LogLines        int
}

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

type loopView struct {
	Loop        *models.Loop
	QueueCount  int
	ProfileName string
	PoolName    string
	LogTail     string
}

type model struct {
	db              *db.DB
	dataDir         string
	refreshInterval time.Duration
	logLines        int
	loops           []loopView
	err             error
	page            int
	width           int
	height          int
	quitting        bool
}

type refreshMsg struct {
	loops []loopView
	err   error
}

type tickMsg struct{}

func newModel(database *db.DB, cfg Config) model {
	return model{
		db:              database,
		dataDir:         cfg.DataDir,
		refreshInterval: cfg.RefreshInterval,
		logLines:        cfg.LogLines,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(), m.tickCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.logLines = computeLogLines(m.height)
		return m, m.fetchCmd()
	case tickMsg:
		return m, tea.Batch(m.fetchCmd(), m.tickCmd())
	case refreshMsg:
		m.loops = msg.loops
		m.err = msg.err
		maxPage := maxPageIndex(len(m.loops))
		if m.page > maxPage {
			m.page = maxPage
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "tab", "right":
			if m.page < maxPageIndex(len(m.loops)) {
				m.page++
			}
		case "shift+tab", "left":
			if m.page > 0 {
				m.page--
			}
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	pageCount := maxPageIndex(len(m.loops)) + 1
	header := fmt.Sprintf("Forge loops | page %d/%d | tab/shift+tab to switch | q to quit", m.page+1, pageCount)
	if m.err != nil {
		header = header + "\n" + fmt.Sprintf("Error: %v", m.err)
	}

	if len(m.loops) == 0 {
		return header + "\n\nNo active loops."
	}

	grid := renderGrid(m.loops, m.page, m.width, m.height)
	return header + "\n\n" + grid
}

func (m model) fetchCmd() tea.Cmd {
	logLines := m.logLines
	if logLines <= 0 {
		logLines = defaultLogLines
	}
	dataDir := m.dataDir

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		views, err := loadLoopViews(ctx, m.db, dataDir, logLines)
		return refreshMsg{loops: views, err: err}
	}
}

func (m model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func loadLoopViews(ctx context.Context, database *db.DB, dataDir string, logLines int) ([]loopView, error) {
	loopRepo := db.NewLoopRepository(database)
	queueRepo := db.NewLoopQueueRepository(database)
	profileRepo := db.NewProfileRepository(database)
	poolRepo := db.NewPoolRepository(database)

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
		if loopEntry.State == models.LoopStateStopped {
			continue
		}

		queueItems, _ := queueRepo.List(ctx, loopEntry.ID)
		pending := 0
		for _, item := range queueItems {
			if item.Status == models.LoopQueueStatusPending {
				pending++
			}
		}

		logPath := loopEntry.LogPath
		if logPath == "" {
			logPath = loop.LogPath(dataDir, loopEntry.Name, loopEntry.ID)
		}
		logTail, _ := tailFile(logPath, logLines)

		views = append(views, loopView{
			Loop:        loopEntry,
			QueueCount:  pending,
			ProfileName: profileNames[loopEntry.ProfileID],
			PoolName:    poolNames[loopEntry.PoolID],
			LogTail:     logTail,
		})
	}

	sort.Slice(views, func(i, j int) bool {
		return views[i].Loop.CreatedAt.Before(views[j].Loop.CreatedAt)
	})

	return views, nil
}

func renderGrid(loops []loopView, page, width, height int) string {
	width = maxInt(width, minCardWidth*2+2)
	height = maxInt(height, minCardHeight*2+3)

	gap := 2
	headerLines := 3
	gridHeight := height - headerLines
	if gridHeight < minCardHeight*2 {
		gridHeight = minCardHeight * 2
	}
	cellHeight := maxInt(minCardHeight, (gridHeight-gap)/2)
	cellWidth := maxInt(minCardWidth, (width-gap)/2)

	start := page * 4
	end := minInt(len(loops), start+4)
	visible := loops[start:end]

	cards := make([]string, 4)
	for i := 0; i < 4; i++ {
		if i < len(visible) {
			cards[i] = renderLoopCard(visible[i], cellWidth, cellHeight)
		} else {
			cards[i] = renderEmptyCard(cellWidth, cellHeight)
		}
	}

	row1 := cards[0] + strings.Repeat(" ", gap) + cards[1]
	row2 := cards[2] + strings.Repeat(" ", gap) + cards[3]
	return row1 + "\n" + strings.Repeat("\n", gap-1) + row2
}

func renderLoopCard(view loopView, width, height int) string {
	loopEntry := view.Loop
	if loopEntry == nil {
		return renderEmptyCard(width, height)
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(width).
		Height(height)

	profile := view.ProfileName
	if profile == "" && loopEntry.ProfileID != "" {
		profile = loopEntry.ProfileID
	}
	pool := view.PoolName
	if pool == "" && loopEntry.PoolID != "" {
		pool = loopEntry.PoolID
	}

	lastRun := "-"
	if loopEntry.LastRunAt != nil {
		lastRun = loopEntry.LastRunAt.UTC().Format(time.RFC3339)
	}
	lastExit := "-"
	if loopEntry.LastExitCode != nil {
		lastExit = fmt.Sprintf("%d", *loopEntry.LastExitCode)
	}

	lines := []string{
		fmt.Sprintf("%s [%s]", loopEntry.Name, loopEntry.State),
		fmt.Sprintf("profile: %s", profile),
		fmt.Sprintf("pool: %s", pool),
		fmt.Sprintf("queue: %d", view.QueueCount),
		fmt.Sprintf("last: %s (exit %s)", lastRun, lastExit),
		"logs:",
	}

	logLines := trimLines(view.LogTail, maxInt(0, height-len(lines)-2))
	lines = append(lines, logLines...)

	contentWidth := maxInt(1, width-2)
	for i, line := range lines {
		lines[i] = truncateLine(line, contentWidth)
	}

	content := strings.Join(lines, "\n")
	return style.Render(content)
}

func renderEmptyCard(width, height int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(width).
		Height(height)
	return style.Render("(empty)")
}

func trimLines(text string, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return nil
	}
	return lines
}

func truncateLine(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(text) <= width {
		return text
	}
	if width <= 3 {
		return text[:width]
	}
	return text[:width-3] + "..."
}

func maxPageIndex(count int) int {
	if count <= 0 {
		return 0
	}
	pages := (count + 3) / 4
	if pages == 0 {
		return 0
	}
	return pages - 1
}

func computeLogLines(height int) int {
	if height <= 0 {
		return defaultLogLines
	}
	lines := height/2 - 6
	if lines < 6 {
		lines = 6
	}
	return lines
}

func tailFile(path string, maxLines int) (string, error) {
	if maxLines <= 0 {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n"), nil
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
