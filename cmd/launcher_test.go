package cmd

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBuildMenu_CoversAllCommands(t *testing.T) {
	items := buildMenu()
	want := 0
	for _, g := range commandGroups {
		want += len(g.items)
	}
	if len(items) != want {
		t.Fatalf("menu has %d items, want %d", len(items), want)
	}
	if !items[0].groupHead {
		t.Fatalf("first item should start a group")
	}
}

func TestLauncher_NavigateAndChoose(t *testing.T) {
	m := launcherModel{items: buildMenu()}

	down, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = down.(launcherModel)
	if m.cursor != 1 {
		t.Fatalf("after down, cursor=%d want 1", m.cursor)
	}

	enter, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = enter.(launcherModel)
	if m.chosen != m.items[1].name {
		t.Fatalf("chosen=%q want %q", m.chosen, m.items[1].name)
	}
	if cmd == nil {
		t.Fatalf("enter should return a quit command")
	}
}

func TestLauncher_WrapsUpFromTop(t *testing.T) {
	m := launcherModel{items: buildMenu()}
	up, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = up.(launcherModel)
	if m.cursor != len(m.items)-1 {
		t.Fatalf("up from top should wrap to last; cursor=%d", m.cursor)
	}
}

func TestLauncher_ViewHasCommands(t *testing.T) {
	m := launcherModel{items: buildMenu()}
	out := m.View()
	for _, want := range []string{"context", "pack", "▸", "Enter run"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view missing %q:\n%s", want, out)
		}
	}
}
