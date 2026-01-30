package progress

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170"))

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	progressBarFull = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42"))

	progressBarEmpty = lipgloss.NewStyle().
				Foreground(lipgloss.Color("238"))

	completeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	speedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// tickMsg is sent periodically to refresh the display
type tickMsg time.Time

// DoneMsg signals that all transfers are complete
type DoneMsg struct{}

// ErrorMsg signals an error occurred
type ErrorMsg struct {
	Err error
}

// TUIModel is the bubbletea model for progress display
type TUIModel struct {
	tracker   *Tracker
	title     string
	showTotal bool // true for upload (aggregate), false for download (per-file only)
	width     int
	quitting  bool
	done      bool
	err       error
	startTime time.Time
}

// NewTUIModel creates a new TUI model for progress display
func NewTUIModel(tracker *Tracker, title string, showTotal bool) TUIModel {
	return TUIModel{
		tracker:   tracker,
		title:     title,
		showTotal: showTotal,
		width:     80,
		startTime: time.Now(),
	}
}

// Init implements tea.Model
func (m TUIModel) Init() tea.Cmd {
	return tickCmd()
}

// Update implements tea.Model
func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			m.tracker.Cancel()
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		if m.width < 40 {
			m.width = 40
		}
		return m, nil

	case tickMsg:
		if m.tracker.IsComplete() {
			m.done = true
			return m, tea.Quit
		}
		return m, tickCmd()

	case DoneMsg:
		m.done = true
		return m, tea.Quit

	case ErrorMsg:
		m.err = msg.Err
		return m, tea.Quit
	}

	return m, nil
}

// View implements tea.Model
func (m TUIModel) View() string {
	if m.quitting {
		return errorStyle.Render("Cancelled.") + "\n"
	}

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render(m.title) + "\n\n")

	// Aggregate progress (for uploads)
	if m.showTotal {
		total, done := m.tracker.GetTotalProgress()
		if total > 0 {
			pct := float64(done) / float64(total) * 100

			elapsed := time.Since(m.startTime).Seconds()
			speed := float64(0)
			if elapsed > 0.1 {
				speed = float64(done) / elapsed
			}

			// Pre-format the stats to calculate exact length
			pctStr := fmt.Sprintf(" %5.1f%%", pct)
			sizeStr := fmt.Sprintf(" (%s / %s)", humanBytes(done), humanBytes(total))
			speedStr := fmt.Sprintf(" %s/s", humanBytes(int64(speed)))

			// Calculate bar width: total width - "Total: " (7) - stats length
			statsLen := len(pctStr) + len(sizeStr) + len(speedStr)
			barWidth := m.width - 7 - statsLen
			if barWidth < 10 {
				barWidth = 10
			}
			bar := renderProgressBar(pct, barWidth)

			b.WriteString(headerStyle.Render("Total: "))
			b.WriteString(bar)
			b.WriteString(pctStr)
			b.WriteString(sizeStr)
			b.WriteString(speedStyle.Render(speedStr))
			b.WriteString("\n\n")
		}
	}

	// Per-file progress
	files := m.tracker.GetFileProgress()

	// Sort by node number, then by filename
	sort.Slice(files, func(i, j int) bool {
		if files[i].NodeNo != files[j].NodeNo {
			return files[i].NodeNo < files[j].NodeNo
		}
		return files[i].FileName < files[j].FileName
	})

	// Max bar width for per-file (will be adjusted dynamically)
	maxBarWidth := 30

	for _, fp := range files {
		b.WriteString(renderFileProgress(fp, maxBarWidth, m.width))
		b.WriteString("\n")
	}

	// Help text
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Press 'q' to cancel"))

	return b.String()
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func renderProgressBar(pct float64, width int) string {
	if width <= 0 {
		return ""
	}

	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled

	return progressBarFull.Render(strings.Repeat("█", filled)) +
		progressBarEmpty.Render(strings.Repeat("░", empty))
}

func renderFileProgress(fp *FileProgress, barWidth int, totalWidth int) string {
	// Truncate filename if too long
	fileName := fp.FileName
	maxNameLen := 20
	if len(fileName) > maxNameLen {
		fileName = "..." + fileName[len(fileName)-maxNameLen+3:]
	}

	var status string
	if fp.Error != nil {
		status = errorStyle.Render("✗ ERROR")
	} else if fp.Complete {
		status = completeStyle.Render("✓ Done")
	} else {
		pct := fp.Percent()
		speed := fp.Speed()

		// Pre-format stats to calculate exact length
		pctStr := fmt.Sprintf(" %5.1f%%", pct)
		speedStr := fmt.Sprintf(" %s/s", humanBytes(int64(speed)))

		// Calculate bar width dynamically
		// "  Node X: " = 10 chars (assuming single digit node), filename = maxNameLen, " " = 1
		prefix := fmt.Sprintf("  Node %d: ", fp.NodeNo)
		prefixLen := len(prefix) + maxNameLen + 1
		statsLen := len(pctStr) + len(speedStr)
		dynamicBarWidth := totalWidth - prefixLen - statsLen
		if dynamicBarWidth < 10 {
			dynamicBarWidth = 10
		}
		if dynamicBarWidth > barWidth {
			dynamicBarWidth = barWidth
		}

		bar := renderProgressBar(pct, dynamicBarWidth)
		status = fmt.Sprintf("%s%s%s", bar, pctStr, speedStyle.Render(speedStr))
	}

	return fmt.Sprintf("  Node %d: %-*s %s",
		fp.NodeNo,
		maxNameLen,
		fileName,
		status)
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// RunWithProgress runs file transfers with a TUI progress display.
// It takes a tracker, title, showTotal flag, and a function that performs the transfers.
// The transferFunc should call tracker.SetComplete() when done.
func RunWithProgress(tracker *Tracker, title string, showTotal bool, transferFunc func()) error {
	model := NewTUIModel(tracker, title, showTotal)
	p := tea.NewProgram(model)

	// Run transfers in background
	go func() {
		transferFunc()
		tracker.SetComplete()
		p.Send(DoneMsg{})
	}()

	// Run TUI (blocks until done)
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	// Check if cancelled
	m := finalModel.(TUIModel)
	if m.quitting {
		return fmt.Errorf("operation cancelled by user")
	}

	// Check for transfer errors
	if tracker.HasErrors() {
		errs := tracker.GetErrors()
		if len(errs) == 1 {
			return errs[0]
		}
		return fmt.Errorf("multiple errors occurred during transfer: %v", errs)
	}

	return nil
}
