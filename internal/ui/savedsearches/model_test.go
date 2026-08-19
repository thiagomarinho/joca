package savedsearches

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
)

func TestNewIncludesGlobalAndCurrentPipelineSearches(t *testing.T) {
	m := New("deploy", []config.SavedLogSearch{
		{Name: "global"},
		{Name: "deploy", Pipeline: "deploy"},
		{Name: "other", Pipeline: "other"},
	})
	if len(m.searches) != 2 || strings.Contains(m.View(), "other") {
		t.Fatalf("filtered searches = %#v", m.searches)
	}
}

func TestRunAndDeleteSelectedSearch(t *testing.T) {
	m := New("deploy", []config.SavedLogSearch{{Name: "first"}, {Name: "second", Pipeline: "deploy"}})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := cmd().(RunMsg).Search.Name; got != "second" {
		t.Fatalf("run search = %q, want second", got)
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(Model)
	if got := cmd().(DeleteMsg); got.Name != "second" || got.Pipeline != "deploy" {
		t.Fatalf("delete message = %#v", got)
	}
	if len(m.searches) != 1 {
		t.Fatalf("searches after delete = %#v", m.searches)
	}
}
