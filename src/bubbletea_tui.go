package main

import (
	"fmt"

	list "github.com/charmbracelet/bubbles/list" //their documentation is useful
	tea "github.com/charmbracelet/bubbletea"     // higher level documentation
)

// This file includes all the basic bubbletea TUI blueprints that you may use/apply
// elsewhere for the Aerolab CLI.

// Here, we go through the following:
// 	(1) The high level explanation of bubbletea TUI components' construction.
// 	(2) We list the basic components used to build a bubbletea element.
// 	(3) We provide a simple example of a bubbletea TUI in action.
// 	(4) We provide instructions on how to reuse the bubbletea TUI components in your own code.

//(1) Bubbletea high level framework

//  - "Model", e.g. bubbleteaModel, is a STRUCT that contains the application's main features.
//   For example, it includes the list TUI format from bubbletea, the user's choice/selection,
//   and a quitting/exit option.

// - "Update", e.g. func (m bubbleteaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {},
//   handles incoming events (called messages), such as key presses. It defines how the
//   model should change in response to user actions, and can also return commands
//   for asynchronous tasks or side effects.

// - "View", e.g. func (m bubbleteaModel) View() string, generates the terminal output
//   based on the current state of the model. This is what the user sees on the screen,
//   and it is re-rendered whenever the model updates.

//

// (2) --- BubbleTea  interactive selection bread and butter compoents, don't need to change these ---
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

// --- BubbleTea  interactive selection bread and butter components ---

// (3) YES OR NO interactive bubbletea prompt in action (please refer to cmdClusterCreate.go)

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

// (4) Instructions for reusability / wrapping bubbletea around your prompts
// 	 1. **For Yes/No style prompts:**
//      - Just call `yesNoPrompt("Your question here?")`.
//      - It will return the user's choice ("Yes" or "No") as a string.
//
//   2. **For other list-based prompts:**
//      - Copy the `yesNoPrompt` function and rename it (e.g., `multiChoicePrompt`).
//      - Replace the `items` slice with your own options, e.g.:
//            items := []list.Item{item("Option A"), item("Option B"), item("Option C")}
//      - Everything else (list setup, model, running the program) works the same.
//
//   3. **Customizing behavior:**
///      - The logic for quitting, selecting items, or rendering output
//        lives inside `bubbleteaModel.Update` and `bubbleteaModel.View`.
//      - If you need special behavior (like different keybindings),
//        adjust those methods directly, using Update
