package main

import (
	"fmt"

	list "github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// --- BubbleTea list model for interactive selection ---

type yesNoItem string

func (y yesNoItem) Title() string       { return string(y) }
func (y yesNoItem) Description() string { return "" }
func (y yesNoItem) FilterValue() string { return string(y) }

type bubbleteaModel struct {
	list     list.Model
	choice   string
	quitting bool
}

func (m bubbleteaModel) Init() tea.Cmd {
	return nil
}

func (m bubbleteaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if i, ok := m.list.SelectedItem().(yesNoItem); ok {
				m.choice = string(i)
			}
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m bubbleteaModel) View() string {
	if m.quitting && m.choice == "" {
		return "No selection made.\n"
	}
	if m.choice != "" {
		return fmt.Sprintf("Selected: %s\n", m.choice)
	}
	return m.list.View()
}

func yesNoPrompt(prompt string) (string, error) {
	items := []list.Item{yesNoItem("Yes"), yesNoItem("No")}
	const defaultWidth = 80
	l := list.New(items, list.NewDefaultDelegate(), defaultWidth, len(items)+2)
	l.Title = prompt
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	m := bubbleteaModel{list: l}
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}
	choice := finalModel.(bubbleteaModel).choice
	return choice, nil
}
