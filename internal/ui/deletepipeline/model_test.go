package deletepipeline

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
)

func TestNewCountsAndWarnsAboutAssociatedAutomations(t *testing.T) {
	entry := config.PipelineEntry{Name: "deploy", Provider: config.ProviderAWS}
	rules := []config.AutomationRule{
		{Name: "to deploy", TriggerPipeline: "deploy"},
		{Name: "from deploy", WatchPipeline: "deploy"},
		{Name: "unrelated", WatchPipeline: "build", TriggerPipeline: "test"},
	}
	m := New("", entry, rules)

	if m.automationsDeleted != 2 {
		t.Errorf("associated automations = %d, want 2", m.automationsDeleted)
	}
	if view := m.View(); !strings.Contains(view, "2 associated automation rule(s)") {
		t.Errorf("expected automation warning in view:\n%s", view)
	}
}

func TestConfirmDeletesPipeline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	entry := config.PipelineEntry{Name: "deploy", Provider: config.ProviderAWS}
	if err := config.Save(path, &config.AppConfig{Pipelines: []config.PipelineEntry{entry}}); err != nil {
		t.Fatal(err)
	}
	m := New(path, entry, nil)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected delete command")
	}
	msg, ok := cmd().(DeletedMsg)
	if !ok {
		t.Fatalf("expected DeletedMsg, got %T", cmd())
	}
	if msg.Name != entry.Name {
		t.Errorf("deleted name = %q, want %q", msg.Name, entry.Name)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Pipelines) != 0 {
		t.Errorf("pipelines remain after delete: %#v", cfg.Pipelines)
	}
}

func TestCancelDoesNotDeletePipeline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	entry := config.PipelineEntry{Name: "deploy", Provider: config.ProviderAWS}
	if err := config.Save(path, &config.AppConfig{Pipelines: []config.PipelineEntry{entry}}); err != nil {
		t.Fatal(err)
	}
	m := New(path, entry, nil)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := cmd().(CancelledMsg); !ok {
		t.Fatalf("expected CancelledMsg, got %T", cmd())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Pipelines) != 1 {
		t.Fatal("pipeline was deleted after cancellation")
	}
}
