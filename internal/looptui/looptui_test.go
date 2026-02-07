package looptui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

func TestModeTransitions(t *testing.T) {
	m := newModel(nil, Config{RefreshInterval: time.Second, LogLines: 8})
	m.loops = []loopView{
		testLoopView("a", "a12345", "alpha", models.LoopStateRunning, "/tmp/a"),
	}
	m.applyFilters("", 0)

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if m.mode != modeFilter {
		t.Fatalf("expected filter mode, got %v", m.mode)
	}

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeMain {
		t.Fatalf("expected main mode after esc from filter, got %v", m.mode)
	}

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.mode != modeExpandedLogs {
		t.Fatalf("expected expanded logs mode, got %v", m.mode)
	}

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeMain {
		t.Fatalf("expected main mode after esc from expanded logs, got %v", m.mode)
	}

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	if m.mode != modeConfirm {
		t.Fatalf("expected confirm mode, got %v", m.mode)
	}

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeMain {
		t.Fatalf("expected main mode after esc from confirm, got %v", m.mode)
	}
}

func TestDeleteConfirmPromptMatchesPRD(t *testing.T) {
	running := newModel(nil, Config{RefreshInterval: time.Second, LogLines: 8})
	running.loops = []loopView{
		testLoopView("loop-running-id", "run123", "alpha", models.LoopStateRunning, "/tmp/a"),
	}
	running.applyFilters("", 0)

	running = updateModel(t, running, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	if running.mode != modeConfirm || running.confirm == nil {
		t.Fatalf("expected confirm mode for running delete")
	}
	expectedForce := "Loop is still running. Force delete record run123? [y/N]"
	if running.confirm.Prompt != expectedForce {
		t.Fatalf("unexpected running delete prompt: %q", running.confirm.Prompt)
	}

	stopped := newModel(nil, Config{RefreshInterval: time.Second, LogLines: 8})
	stopped.loops = []loopView{
		testLoopView("loop-stopped-id", "stop12", "beta", models.LoopStateStopped, "/tmp/b"),
	}
	stopped.applyFilters("", 0)

	stopped = updateModel(t, stopped, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	if stopped.mode != modeConfirm || stopped.confirm == nil {
		t.Fatalf("expected confirm mode for stopped delete")
	}
	expectedDelete := "Delete loop record stop12? [y/N]"
	if stopped.confirm.Prompt != expectedDelete {
		t.Fatalf("unexpected stopped delete prompt: %q", stopped.confirm.Prompt)
	}
}

func TestSelectionChoosesNearestRowWhenLoopDisappears(t *testing.T) {
	m := newModel(nil, Config{RefreshInterval: time.Second, LogLines: 8})
	m.loops = []loopView{
		testLoopView("id-a", "ida", "alpha", models.LoopStateRunning, "/tmp/a"),
		testLoopView("id-b", "idb", "beta", models.LoopStateRunning, "/tmp/b"),
		testLoopView("id-c", "idc", "gamma", models.LoopStateRunning, "/tmp/c"),
	}
	m.applyFilters("", 0)
	m.selectedIdx = 1
	m.selectedID = "id-b"

	updated := []loopView{
		testLoopView("id-a", "ida", "alpha", models.LoopStateRunning, "/tmp/a"),
		testLoopView("id-c", "idc", "gamma", models.LoopStateRunning, "/tmp/c"),
	}

	m = updateModel(t, m, refreshMsg{loops: updated, selectedID: "", selected: logTailView{}})
	if m.selectedID != "id-c" {
		t.Fatalf("expected nearest surviving row id-c, got %s", m.selectedID)
	}
	if m.selectedIdx != 1 {
		t.Fatalf("expected selection index 1, got %d", m.selectedIdx)
	}
}

func TestFilterModeRealtimeTextAndStatus(t *testing.T) {
	m := newModel(nil, Config{RefreshInterval: time.Second, LogLines: 8})
	m.loops = []loopView{
		testLoopView("id-a", "ida", "alpha", models.LoopStateRunning, "/repo/alpha"),
		testLoopView("id-b", "idb", "beta", models.LoopStateStopped, "/repo/beta"),
	}
	m.applyFilters("", 0)

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if m.mode != modeFilter {
		t.Fatalf("expected filter mode")
	}

	for _, r := range []rune{'b', 'e', 't', 'a'} {
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(m.filtered) != 1 || m.filtered[0].Loop.ID != "id-b" {
		t.Fatalf("expected realtime text filter to isolate id-b")
	}

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.filterFocus != filterFocusStatus {
		t.Fatalf("expected status focus after tab")
	}

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.filterState != "running" {
		t.Fatalf("expected running status, got %s", m.filterState)
	}
	if len(m.filtered) != 0 {
		t.Fatalf("expected no rows for beta + running filter")
	}

	for m.filterState != "stopped" {
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRight})
	}
	if len(m.filtered) != 1 || m.filtered[0].Loop.ID != "id-b" {
		t.Fatalf("expected stopped status filter to restore id-b")
	}
}

func TestWizardStepValidation(t *testing.T) {
	m := newModel(nil, Config{RefreshInterval: time.Second, LogLines: 8})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.mode != modeWizard {
		t.Fatalf("expected wizard mode")
	}
	m.wizard.Values.Count = "0"
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.wizard.Step != 1 {
		t.Fatalf("expected wizard to stay on step 1 for invalid count")
	}
	if !strings.Contains(m.wizard.Error, "count") {
		t.Fatalf("expected count validation error, got %q", m.wizard.Error)
	}
}

func TestCreateLoopsWizardPath(t *testing.T) {
	database, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer database.Close()
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	oldStart := startLoopProcessFn
	startLoopProcessFn = func(loopID, configFile string) error { return nil }
	defer func() { startLoopProcessFn = oldStart }()

	selectedID, message, err := createLoops(
		context.Background(),
		database,
		t.TempDir(),
		"",
		30*time.Second,
		"",
		"",
		wizardValues{Name: "wizard-loop", Count: "1", MaxRuntime: "1m", MaxIterations: "1"},
	)
	if err != nil {
		t.Fatalf("create loops: %v", err)
	}
	if message == "" {
		t.Fatalf("expected success message")
	}

	loopRepo := db.NewLoopRepository(database)
	loops, err := loopRepo.List(context.Background())
	if err != nil {
		t.Fatalf("list loops: %v", err)
	}
	if len(loops) != 1 {
		t.Fatalf("expected 1 loop, got %d", len(loops))
	}
	if loops[0].Name != "wizard-loop" {
		t.Fatalf("unexpected loop name %q", loops[0].Name)
	}
	if selectedID != loops[0].ID {
		t.Fatalf("selected id mismatch: got %s want %s", selectedID, loops[0].ID)
	}
}

func TestViewEmptyStateGuidesLoopCreation(t *testing.T) {
	m := newModel(nil, Config{RefreshInterval: time.Second, LogLines: 8})
	m.applyFilters("", 0)

	out := m.View()
	if !strings.Contains(out, "Start one: forge up --count 1") {
		t.Fatalf("expected empty-state startup guidance, got:\n%s", out)
	}
}

func TestViewRendersErrorStateWithoutCrashing(t *testing.T) {
	m := newModel(nil, Config{RefreshInterval: time.Second, LogLines: 8})
	m.err = errors.New("boom")

	out := m.View()
	if !strings.Contains(out, "Error: boom") {
		t.Fatalf("expected visible error rendering, got:\n%s", out)
	}
}

func testLoopView(id, shortID, name string, state models.LoopState, repo string) loopView {
	return loopView{Loop: &models.Loop{ID: id, ShortID: shortID, Name: name, State: state, RepoPath: repo, CreatedAt: time.Now().UTC()}}
}

func updateModel(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	next, _ := m.Update(msg)
	updated, ok := next.(model)
	if !ok {
		t.Fatalf("unexpected model type %T", next)
	}
	return updated
}
