package main

import (
	"fmt"

	list "github.com/charmbracelet/bubbles/list" //see folder on github
	tea "github.com/charmbracelet/bubbletea"
)

// --- BubbleTea  interactive selection bread and butter sstructs ---

type item string

func (y item) Title() string       { return string(y) } // returns item title
func (y item) Description() string { return "" }        // returns item description
func (y item) FilterValue() string { return string(y) } // returns item filter value

type bubbleteaModel struct { //struct for bubbletea model
	list     list.Model //pull Model struct from list package (holds all aspects of our component)
	choice   string     //user's choice, type string
	quitting bool       //quitting status, type bool
}

func (m bubbleteaModel) Init() tea.Cmd { // Init inputs m (model of type defined above)
	return nil
}

func (m bubbleteaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if i, ok := m.list.SelectedItem().(item); ok {
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

// YES OR NO interactive prompt, you may copy this and change its characteristics to what you need

func yesNoPrompt(prompt string) (string, error) {
	items := []list.Item{item("Yes"), item("No")}
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
