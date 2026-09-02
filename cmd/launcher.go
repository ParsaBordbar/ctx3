package cmd

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// menuItem is one selectable command in the interactive launcher.
type menuItem struct {
	name, desc, group string
	groupHead         bool
}

// buildMenu flattens commandGroups into a selectable list.
func buildMenu() []menuItem {
	var items []menuItem
	for _, g := range commandGroups {
		for i, it := range g.items {
			items = append(items, menuItem{name: it[0], desc: it[1], group: g.title, groupHead: i == 0})
		}
	}
	return items
}

type launcherModel struct {
	items  []menuItem
	cursor int
	chosen string
	color  bool
}

func (m launcherModel) Init() tea.Cmd { return nil }

func (m launcherModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k", "shift+tab":
		m.cursor = (m.cursor - 1 + len(m.items)) % len(m.items)
	case "down", "j", "tab":
		m.cursor = (m.cursor + 1) % len(m.items)
	case "enter":
		m.chosen = m.items[m.cursor].name
		return m, tea.Quit
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m launcherModel) View() string {
	blue := lipgloss.NewStyle().Foreground(lipgloss.Color("#43B5E6")).Bold(true)
	head := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)
	if !m.color {
		blue = lipgloss.NewStyle().Bold(true)
		head = lipgloss.NewStyle()
		dim = lipgloss.NewStyle()
	}

	var b []byte
	b = append(b, banner(m.color)...)
	b = append(b, '\n')

	lastGroup := ""
	for i, it := range m.items {
		if it.group != lastGroup {
			b = append(b, "  "...)
			b = append(b, head.Render(it.group)...)
			b = append(b, '\n')
			lastGroup = it.group
		}
		cursor := "   "
		name := pad(it.name, 11)
		if i == m.cursor {
			cursor = " " + blue.Render("▸") + " "
			name = blue.Render(name)
		}
		b = append(b, "  "...)
		b = append(b, cursor...)
		b = append(b, name...)
		b = append(b, ' ')
		b = append(b, dim.Render(it.desc)...)
		b = append(b, '\n')
	}

	b = append(b, '\n')
	b = append(b, "  "...)
	b = append(b, dim.Render("↑/↓ move · Tab next · Enter run · q quit")...)
	b = append(b, '\n')
	return string(b)
}

// pad right-pads s to width with spaces (plain text, before any styling).
func pad(s string, width int) string {
	for len(s) < width {
		s += " "
	}
	return s
}

// runLauncher shows the interactive menu and returns the chosen command name, or "" if the user quit.
func runLauncher(color bool) string {
	m := launcherModel{items: buildMenu(), color: color}
	p := tea.NewProgram(m, tea.WithOutput(os.Stdout))
	final, err := p.Run()
	if err != nil {
		return ""
	}
	if lm, ok := final.(launcherModel); ok {
		return lm.chosen
	}
	return ""
}
