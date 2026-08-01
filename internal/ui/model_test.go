package ui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/ui/list"
)

func TestTogglePauseWhilePausedPipelinesHiddenClampsCursor(t *testing.T) {
	appCfg := &config.AppConfig{
		Pipelines: []config.PipelineEntry{
			{Name: "first"},
			{Name: "second"},
		},
	}
	m := New(appCfg, config.Config{ConfigFile: filepath.Join(t.TempDir(), "config.yaml")})

	listView := m.stack[0].(list.Model)
	listView.HidePaused = true
	updated, _ := listView.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.stack[0] = updated

	m.Update(list.TogglePauseMsg{Index: 1})

	// View asks the list for its selected pipeline to render provider-specific
	// shortcuts. It must remain safe after the selected row disappears.
	m.View()

	listView = m.stack[0].(list.Model)
	selected, ok := listView.Selected()
	if !ok {
		t.Fatal("expected the remaining visible pipeline to be selected")
	}
	if selected.Entry.Name != "first" {
		t.Errorf("selected pipeline = %q, want %q", selected.Entry.Name, "first")
	}
}
