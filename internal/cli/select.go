package cli

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type selectModel struct {
	title    string
	choices  []string
	cursor   int
	selected bool
	canceled bool
}

func newSelectModel(title string, choices []string) selectModel {
	return selectModel{title: title, choices: choices}
}

func (m selectModel) Init() tea.Cmd {
	return nil
}

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			m.selected = true
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.canceled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m selectModel) View() string {
	if len(m.choices) == 0 {
		return "No options available.\n"
	}

	var b strings.Builder
	if strings.TrimSpace(m.title) != "" {
		b.WriteString(m.title)
		b.WriteString("\n\n")
	}

	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		fmt.Fprintf(&b, "%s %s\n", cursor, choice)
	}

	b.WriteString("\nUse up/down to move, enter to select, q to cancel.\n")
	return b.String()
}

func selectChoice(title string, choices []string) (string, bool, error) {
	if len(choices) == 0 {
		return "", false, nil
	}

	program := tea.NewProgram(newSelectModel(title, choices))
	model, err := program.Run()
	if err != nil {
		return "", false, err
	}

	selection, ok := model.(selectModel)
	if !ok {
		return "", false, fmt.Errorf("unexpected selection model")
	}
	if selection.canceled {
		return "", false, nil
	}
	if selection.cursor < 0 || selection.cursor >= len(selection.choices) {
		return "", false, fmt.Errorf("invalid selection index")
	}
	return selection.choices[selection.cursor], true, nil
}
