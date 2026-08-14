package logsearch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/provider"
)

func TestSearchReportsProgressAndCollectsMatchingExecutions(t *testing.T) {
	runs := []provider.Run{
		{ID: "execution-2", LogSources: sourcesWithLogs("compile", "all good")},
		{ID: "execution-1", LogSources: sourcesWithLogs("test", "fatal: needle found")},
	}
	m := New("pipeline", "", func(context.Context, int) ([]provider.Run, error) { return runs, nil })
	m.query = "needle"
	m.depth = "2"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil || !strings.Contains(m.View(), "Loading 2 executions") {
		t.Fatal("expected execution loading status")
	}

	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if cmd == nil || !strings.Contains(m.View(), "Searching execution 1/2") {
		t.Fatal("expected per-execution search progress")
	}

	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected search to be complete")
	}

	if len(m.matches) != 1 || m.matches[0].run.ID != "execution-1" {
		t.Fatalf("matches = %#v, want execution-1", m.matches)
	}
	view := m.View()
	if !strings.Contains(view, "needle") || !strings.Contains(view, "#execution-1") || !strings.Contains(view, "project-test") {
		t.Fatalf("results missing query, execution, or CodeBuild project:\n%s", view)
	}
}

func TestEscapeFromResultsReturnsToPipelineDetail(t *testing.T) {
	m := Model{phase: phaseResults}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected back command")
	}
	if _, ok := cmd().(BackMsg); !ok {
		t.Fatalf("message = %T, want BackMsg", cmd())
	}
}

func TestClosingExecutionLogsReturnsToResults(t *testing.T) {
	m := Model{
		pipeline: "pipeline",
		phase:    phaseResults,
		query:    "needle",
		searched: 2,
		matches: []searchMatch{{
			run:     provider.Run{ID: "execution-1"},
			sources: []matchedSource{{name: "Build / compile", content: "needle"}},
		}},
	}

	updated, cmd := m.Update(pagerClosedMsg{})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command after pager closes")
	}
	if m.phase != phaseResults || !strings.Contains(m.View(), "#execution-1") {
		t.Fatal("expected to remain on the search results after closing logs")
	}
}

func TestOpenedLogsIdentifyCodeBuildProject(t *testing.T) {
	match := searchMatch{
		run: provider.Run{ID: "execution-1"},
		sources: []matchedSource{{
			name: "Build / compile", project: "payments-build", content: "needle",
		}},
	}
	msg := writeMatchLogsCmd("pipeline", match, false)().(logsWrittenMsg)
	if msg.err != nil {
		t.Fatalf("writeMatchLogsCmd() error = %v", msg.err)
	}
	content, err := os.ReadFile(msg.path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "CodeBuild project: payments-build") {
		t.Fatalf("opened logs do not identify project:\n%s", content)
	}
}

func TestOpeningLogsShowsResolvedEditor(t *testing.T) {
	dir := t.TempDir()
	editor := filepath.Join(dir, "custom-code")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := Model{phase: phaseResults, logEditor: editor}

	updated, cmd := m.Update(logsWrittenMsg{path: "/tmp/log.txt", editor: true})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected editor launch command")
	}
	if m.openStatus != "Opening logs using custom-code…" {
		t.Fatalf("status = %q", m.openStatus)
	}
}

func sourcesWithLogs(action, content string) func(context.Context) ([]provider.LogSource, error) {
	return func(context.Context) ([]provider.LogSource, error) {
		return []provider.LogSource{{
			Stage: "Build", Action: action, Project: "project-" + action,
			Logs: func(context.Context) (string, error) { return content, nil },
		}}, nil
	}
}
