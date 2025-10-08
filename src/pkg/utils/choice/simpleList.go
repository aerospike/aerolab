package choice

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const listHeight = 14

var (
	titleStyle        = lipgloss.NewStyle().MarginLeft(2)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
)

type Item string

func (i Item) FilterValue() string { return "" }

// StringSliceToItems converts a slice of strings into a slice of Items for use with the choice system.
// This is a utility function that transforms regular string slices into the format required
// by the interactive choice interface.
//
// Parameters:
//   - slice: A slice of strings to convert to choice items
//
// Returns:
//   - Items: A slice of Item objects that can be used with Choice() and ChoiceWithHeight()
//
// Usage:
//
//	options := []string{"option1", "option2", "option3"}
//	items := choice.StringSliceToItems(options)
//	selected, quit, err := choice.Choice("Select an option:", items)
func StringSliceToItems(slice []string) Items {
	items := make(Items, len(slice))
	for i, s := range slice {
		items[i] = Item(s)
	}
	return items
}

type itemDelegate struct {
	lenght int
}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(Item)
	if !ok {
		return
	}

	// get number of digits in d.lenght
	digits := len(fmt.Sprintf("%d", d.lenght))
	str := fmt.Sprintf("%*d. %s", digits, index+1, i)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

type model struct {
	list     list.Model
	choice   string
	quitting bool
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		return m, nil

	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "enter":
			i, ok := m.list.SelectedItem().(Item)
			if ok {
				m.choice = string(i)
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *model) View() string {
	if m.choice != "" {
		return ""
	}
	if m.quitting {
		return ""
	}
	return "\n" + m.list.View()
}

// ChoiceWithHeight presents an interactive choice selection interface with a custom height.
// This function displays a terminal-based list interface where users can navigate with arrow keys
// and select an item with Enter. The interface supports pagination for long lists.
//
// Parameters:
//   - title: The title to display above the choice list
//   - items: The list of items to choose from (use StringSliceToItems to convert from strings)
//   - height: The height of the choice interface in terminal lines
//
// Returns:
//   - choice: The selected item as a string, empty if user quit without selecting
//   - quitting: true if the user quit without making a selection (Ctrl+C or 'q')
//   - err: nil on success, or an error if the interface failed to initialize
//
// Usage:
//
//	items := choice.StringSliceToItems([]string{"option1", "option2", "option3"})
//	selected, quit, err := choice.ChoiceWithHeight("Select an option:", items, 10)
//	if err != nil || quit {
//	    return
//	}
//	fmt.Printf("Selected: %s\n", selected)
func ChoiceWithHeight(title string, items Items, height int) (choice string, quitting bool, err error) {
	const defaultWidth = 20

	l := list.New(items, itemDelegate{lenght: len(items)}, defaultWidth, height)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = titleStyle
	l.Styles.PaginationStyle = paginationStyle
	l.Styles.HelpStyle = helpStyle

	m := model{list: l}

	if _, err := tea.NewProgram(&m).Run(); err != nil {
		return "", false, err
	}

	return m.choice, m.quitting, nil
}

// Choice presents an interactive choice selection interface with the default height.
// This is a convenience function that calls ChoiceWithHeight with a predefined height of 14 lines.
// It provides the same functionality as ChoiceWithHeight but with a standard interface size.
//
// Parameters:
//   - title: The title to display above the choice list
//   - items: The list of items to choose from (use StringSliceToItems to convert from strings)
//
// Returns:
//   - choice: The selected item as a string, empty if user quit without selecting
//   - quitting: true if the user quit without making a selection (Ctrl+C or 'q')
//   - err: nil on success, or an error if the interface failed to initialize
//
// Usage:
//
//	items := choice.StringSliceToItems([]string{"option1", "option2", "option3"})
//	selected, quit, err := choice.Choice("Select an option:", items)
//	if err != nil || quit {
//	    return
//	}
//	fmt.Printf("Selected: %s\n", selected)
func Choice(title string, items Items) (choice string, quitting bool, err error) {
	return ChoiceWithHeight(title, items, listHeight)
}

type Items []list.Item
