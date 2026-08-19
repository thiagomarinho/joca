package detail

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/provider"
	"github.com/thiagomarinho/joca/internal/ui/list"
)

func TestCodeBuildLogSearchOpensForAWSPipeline(t *testing.T) {
	item := list.PipelineItem{Entry: config.PipelineEntry{Name: "pipeline", Provider: config.ProviderAWS}}
	m := New(item)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if cmd == nil {
		t.Fatal("expected log search command")
	}
	msg, ok := cmd().(OpenLogSearchMsg)
	if !ok || msg.Item.Entry.Name != "pipeline" {
		t.Fatalf("message = %#v, want OpenLogSearchMsg for pipeline", msg)
	}
	if !strings.Contains(m.View(), "/: search CodeBuild logs") {
		t.Fatal("expected CodeBuild log search shortcut in detail footer")
	}
}

func TestSavedLogSearchesOpenForAWSPipeline(t *testing.T) {
	item := list.PipelineItem{Entry: config.PipelineEntry{Name: "pipeline", Provider: config.ProviderAWS}}
	m := New(item)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	if cmd == nil {
		t.Fatal("expected saved-search command")
	}
	if _, ok := cmd().(OpenSavedLogSearchesMsg); !ok {
		t.Fatalf("message = %T, want OpenSavedLogSearchesMsg", cmd())
	}
	if !strings.Contains(m.View(), "S: saved searches") {
		t.Fatal("expected saved-search shortcut in detail footer")
	}
}

func TestLoadLogsShowsCodeBuildActionPicker(t *testing.T) {
	run := provider.Run{
		ID: "execution-1",
		LogSources: func(context.Context) ([]provider.LogSource, error) {
			return []provider.LogSource{
				{Stage: "Build", Action: "Compile", Project: "app-build", Status: provider.StatusSuccess},
				{Stage: "Test", Action: "Integration", Project: "app-test", Status: provider.StatusFailed},
			}, nil
		},
	}
	m := New(list.PipelineItem{Current: run})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd == nil {
		t.Fatal("expected log discovery command")
	}
	updated, _ = updated.(Model).Update(cmd())
	view := updated.(Model).View()
	if !strings.Contains(view, "Build / Compile") || !strings.Contains(view, "Test / Integration") {
		t.Fatalf("action picker missing CodeBuild actions:\n%s", view)
	}
}

func TestDownloadLogCmdWritesPrivateTemporaryFile(t *testing.T) {
	source := provider.LogSource{
		Stage:  "Build stage",
		Action: "Compile/app",
		Logs: func(context.Context) (string, error) {
			return "build output\n", nil
		},
	}

	msg := downloadLogCmd(source)().(logDownloadedMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(msg.path)) })
	if filepath.Base(msg.path) != "Build-stage-Compile-app.log" {
		t.Errorf("temporary filename = %q", filepath.Base(msg.path))
	}
	info, err := os.Stat(msg.path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("log permissions = %o, want 600", info.Mode().Perm())
	}
	content, err := os.ReadFile(msg.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "build output\n" {
		t.Errorf("log content = %q", content)
	}
}

func TestSelectedLogRunFallsBackToLatestHistoryWhenCurrentHasNoExecution(t *testing.T) {
	historyRun := provider.Run{ID: "execution-1", LogSources: func(context.Context) ([]provider.LogSource, error) { return nil, nil }}
	m := New(list.PipelineItem{Current: provider.Run{}, History: []provider.Run{historyRun}})

	got, ok := m.selectedLogRun()
	if !ok || got.ID != historyRun.ID {
		t.Fatalf("selectedLogRun() = %#v, %v", got, ok)
	}
}
