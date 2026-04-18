package tui

import (
	"fmt"
	"strings"

	"mux/internal/grid"
	"mux/internal/tmux"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

type focus int

const (
	focusCompose focus = iota
	focusPicker
	focusGrid
	focusGridInput
)

type App struct {
	compose  textarea.Model
	filter   textinput.Model
	sessions []tmux.Session
	filtered []tmux.Session
	cursor   int
	focus    focus
	status   string
	width    int
	height   int

	// grid overlay
	gridLayouts []grid.MonitorLayout
	gridCursor  int
	gridErr     string
	gridInput   textinput.Model // URL input for grid cell assignment
	gridTarget  gridCell        // which cell we're assigning to
}

type gridCell struct {
	monitor string
	cell    string
}

// Messages

type sessionsMsg []tmux.Session
type sessionsErrMsg struct{ err error }
type sendDoneMsg struct{ target string }
type sendErrMsg struct{ err error }
type gridMsg []grid.MonitorLayout
type gridErrMsg struct{ err error }
type gridSetDoneMsg struct{ cell, url string }
type gridSetErrMsg struct{ err error }

func NewApp() App {
	ta := textarea.New()
	ta.Placeholder = "Type your message..."
	ta.Focus()
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	fi := textinput.New()
	fi.Placeholder = "filter sessions..."

	gi := textinput.New()
	gi.Placeholder = "enter URL..."
	gi.Width = 60

	return App{
		compose:   ta,
		filter:    fi,
		gridInput: gi,
		focus:     focusCompose,
	}
}

func (a App) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, fetchSessions)
}

func fetchSessions() tea.Msg {
	sessions, err := tmux.ListAll()
	if err != nil {
		return sessionsErrMsg{err}
	}
	return sessionsMsg(sessions)
}

func fetchGrid() tea.Msg {
	c := grid.NewClient("")
	layouts, err := c.GetLayout()
	if err != nil {
		return gridErrMsg{err}
	}
	return gridMsg(layouts)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			if a.focus == focusGridInput {
				a.focus = focusGrid
				a.gridInput.Blur()
				a.gridInput.SetValue("")
				return a, nil
			}
			if a.focus == focusGrid {
				a.focus = focusCompose
				a.compose.Focus()
				return a, nil
			}
			return a, tea.Quit

		case "tab":
			if a.focus == focusGrid {
				return a, nil
			}
			if a.focus == focusCompose {
				a.focus = focusPicker
				a.compose.Blur()
				a.filter.Focus()
			} else {
				a.focus = focusCompose
				a.filter.Blur()
				a.compose.Focus()
			}
			return a, nil

		case "ctrl+g":
			if a.focus == focusGrid {
				a.focus = focusCompose
				a.compose.Focus()
				return a, nil
			}
			a.focus = focusGrid
			a.compose.Blur()
			a.filter.Blur()
			return a, fetchGrid

		case "ctrl+r":
			return a, fetchSessions

		case "enter":
			if a.focus == focusPicker && len(a.filtered) > 0 {
				return a, a.sendToSelected()
			}
			if a.focus == focusGrid {
				target := a.gridCellAt(a.gridCursor)
				if target != nil {
					a.gridTarget = *target
					a.gridInput.SetValue(a.gridPaneURL(a.gridCursor))
					a.gridInput.Focus()
					a.focus = focusGridInput
				}
				return a, nil
			}
			if a.focus == focusGridInput {
				url := strings.TrimSpace(a.gridInput.Value())
				if url != "" {
					return a, a.setGridCell(a.gridTarget.monitor, a.gridTarget.cell, url)
				}
				return a, nil
			}

		case "up":
			if a.focus == focusPicker && a.cursor > 0 {
				a.cursor--
			} else if a.focus == focusGrid && a.gridCursor > 0 {
				a.gridCursor--
			}
			return a, nil

		case "down":
			if a.focus == focusPicker && a.cursor < len(a.filtered)-1 {
				a.cursor++
			} else if a.focus == focusGrid && a.gridCursor < a.gridPaneCount()-1 {
				a.gridCursor++
			}
			return a, nil
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.compose.SetWidth(msg.Width - 4)

	case sessionsMsg:
		a.sessions = msg
		a.applyFilter()
		a.status = fmt.Sprintf("%d sessions loaded", len(msg))

	case sessionsErrMsg:
		a.status = fmt.Sprintf("Error: %v", msg.err)

	case sendDoneMsg:
		a.status = fmt.Sprintf("Sent to %s", msg.target)
		a.compose.Reset()
		a.focus = focusCompose
		a.compose.Focus()
		a.filter.Blur()
		return a, fetchSessions

	case sendErrMsg:
		a.status = fmt.Sprintf("Send failed: %v", msg.err)

	case gridMsg:
		a.gridLayouts = msg
		a.gridErr = ""
		a.gridCursor = 0

	case gridErrMsg:
		a.gridErr = msg.err.Error()

	case gridSetDoneMsg:
		a.status = fmt.Sprintf("Set %s → %s", msg.cell, msg.url)
		a.gridInput.SetValue("")
		a.gridInput.Blur()
		a.focus = focusGrid
		return a, fetchGrid

	case gridSetErrMsg:
		a.status = fmt.Sprintf("Grid set failed: %v", msg.err)
		a.focus = focusGrid
		a.gridInput.Blur()
	}

	// Update focused component
	if a.focus == focusCompose {
		var cmd tea.Cmd
		a.compose, cmd = a.compose.Update(msg)
		cmds = append(cmds, cmd)
	} else if a.focus == focusPicker {
		oldVal := a.filter.Value()
		var cmd tea.Cmd
		a.filter, cmd = a.filter.Update(msg)
		cmds = append(cmds, cmd)
		if a.filter.Value() != oldVal {
			a.applyFilter()
		}
	} else if a.focus == focusGridInput {
		var cmd tea.Cmd
		a.gridInput, cmd = a.gridInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

func (a *App) applyFilter() {
	q := a.filter.Value()
	if q == "" {
		a.filtered = a.sessions
	} else {
		names := make([]string, len(a.sessions))
		for i, s := range a.sessions {
			names[i] = s.Name + " " + s.Host + " " + s.Group
		}
		matches := fuzzy.Find(q, names)
		a.filtered = make([]tmux.Session, len(matches))
		for i, m := range matches {
			a.filtered[i] = a.sessions[m.Index]
		}
	}
	if a.cursor >= len(a.filtered) {
		a.cursor = max(0, len(a.filtered)-1)
	}
}

func (a *App) sendToSelected() tea.Cmd {
	if a.cursor >= len(a.filtered) {
		return nil
	}
	target := a.filtered[a.cursor]
	text := strings.TrimSpace(a.compose.Value())
	if text == "" {
		a.status = "Nothing to send (compose is empty)"
		return nil
	}
	return func() tea.Msg {
		if err := tmux.Send(target, text); err != nil {
			return sendErrMsg{err}
		}
		return sendDoneMsg{fmt.Sprintf("%s (%s)", target.Name, target.Host)}
	}
}

func (a *App) gridPaneCount() int {
	n := 0
	for _, l := range a.gridLayouts {
		n += len(l.Panes)
	}
	return n
}

func (a *App) gridCellAt(idx int) *gridCell {
	n := 0
	for _, l := range a.gridLayouts {
		for _, p := range l.Panes {
			if n == idx {
				return &gridCell{monitor: l.Monitor.String(), cell: p.ID}
			}
			n++
		}
	}
	return nil
}

func (a *App) gridPaneURL(idx int) string {
	n := 0
	for _, l := range a.gridLayouts {
		for _, p := range l.Panes {
			if n == idx {
				return p.URL
			}
			n++
		}
	}
	return ""
}

func (a *App) setGridCell(monitor, cell, url string) tea.Cmd {
	return func() tea.Msg {
		c := grid.NewClient("")
		if err := c.SetCell(monitor, cell, url); err != nil {
			return gridSetErrMsg{err}
		}
		return gridSetDoneMsg{cell: monitor + "/" + cell, url: url}
	}
}

func (a App) View() string {
	if a.focus == focusGrid || a.focus == focusGridInput {
		return a.viewGrid()
	}
	return a.viewMain()
}

func (a App) viewMain() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	activeStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("99"))
	inactiveStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))

	// Compose pane
	composeBox := inactiveStyle
	if a.focus == focusCompose {
		composeBox = activeStyle
	}
	compose := composeBox.Width(a.width - 4).Render(
		titleStyle.Render("compose") + "\n" + a.compose.View(),
	)

	// Sessions pane
	pickerBox := inactiveStyle
	if a.focus == focusPicker {
		pickerBox = activeStyle
	}

	maxVisible := a.height - 14
	if maxVisible < 3 {
		maxVisible = 3
	}

	var sessionLines []string
	for i, s := range a.filtered {
		if i >= maxVisible {
			sessionLines = append(sessionLines, dimStyle.Render(fmt.Sprintf("  ... +%d more", len(a.filtered)-maxVisible)))
			break
		}
		prefix := "  "
		style := lipgloss.NewStyle()
		if i == a.cursor && a.focus == focusPicker {
			prefix = "▸ "
			style = style.Bold(true).Foreground(lipgloss.Color("99"))
		}
		group := s.Group
		if group == "" {
			group = "-"
		}
		line := fmt.Sprintf("%s%-25s %-10s group:%s", prefix, s.Name, s.Host, group)
		sessionLines = append(sessionLines, style.Render(line))
	}
	if len(a.filtered) == 0 {
		sessionLines = append(sessionLines, dimStyle.Render("  (no sessions)"))
	}

	filterLine := "filter: " + a.filter.View()
	sessionsContent := titleStyle.Render("sessions") + "\n" +
		strings.Join(sessionLines, "\n") + "\n" +
		filterLine

	picker := pickerBox.Width(a.width - 4).Render(sessionsContent)

	// Status + keys
	statusLine := dimStyle.Render(a.status)
	keys := dimStyle.Render("[Tab] switch  [Enter] send  [Ctrl+G] grid  [Ctrl+R] refresh  [Esc] quit")

	return compose + "\n" + picker + "\n" + statusLine + "\n" + keys
}

func (a App) viewGrid() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	activeStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("212"))

	if a.gridErr != "" {
		return activeStyle.Width(a.width - 4).Render(
			titleStyle.Render("grid server") + "\n\n" +
				"Error: " + a.gridErr + "\n\n" +
				dimStyle.Render("[Esc] back  [Ctrl+G] back"),
		)
	}

	var lines []string
	idx := 0
	for _, l := range a.gridLayouts {
		label := l.Label
		if label == "" {
			label = "Monitor " + l.Monitor.String()
		}
		lines = append(lines, titleStyle.Render(fmt.Sprintf("%s [%s] (%s)", label, l.Monitor, l.Mode)))
		for _, p := range l.Panes {
			prefix := "  "
			style := lipgloss.NewStyle()
			if idx == a.gridCursor {
				prefix = "▸ "
				style = style.Bold(true).Foreground(lipgloss.Color("212"))
			}
			url := p.URL
			if len(url) > 55 {
				url = url[:52] + "..."
			}
			lines = append(lines, style.Render(fmt.Sprintf("%s%-4s %s", prefix, p.ID, url)))
			idx++
		}
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")

	if a.focus == focusGridInput {
		inputLabel := fmt.Sprintf("URL for %s/%s:", a.gridTarget.monitor, a.gridTarget.cell)
		content += "\n" + titleStyle.Render(inputLabel) + "\n" + a.gridInput.View()
		keys := dimStyle.Render("[Enter] set  [Esc] cancel")
		return activeStyle.Width(a.width - 4).Render(
			titleStyle.Render("grid server layout") + "\n\n" + content,
		) + "\n" + keys
	}

	keys := dimStyle.Render("[Enter] set URL  [Esc] back  [↑/↓] navigate")

	return activeStyle.Width(a.width - 4).Render(
		titleStyle.Render("grid server layout") + "\n\n" + content,
	) + "\n" + keys
}
