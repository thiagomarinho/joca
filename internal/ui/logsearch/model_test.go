package logsearch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/provider"
)

func TestFindMatchesSupportsCaseInsensitiveContext(t *testing.T) {
	content := strings.Join([]string{
		"before first",
		"ERROR one",
		"after first",
		"unrelated",
		"error two",
		"after second",
	}, "\n")

	result := findMatches(content, searchOptions{query: "error", caseInsensitive: true, contextLines: 1})
	if result.count != 2 {
		t.Fatalf("count = %d, want 2", result.count)
	}
	joined := strings.Join(result.snippets, "\n")
	for _, want := range []string{"1  before first", "2  ERROR one", "5  error two", "6  after second"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("context missing %q:\n%s", want, joined)
		}
	}
}

func TestFindMatchesSupportsRegularExpressions(t *testing.T) {
	result := findMatches("status=500\nstatus=404\nstatus=503", searchOptions{
		query: "status=5[0-9]{2}", regex: true,
	})
	if result.count != 2 {
		t.Fatalf("count = %d, want 2", result.count)
	}
}

func TestSearchInputAcceptsSpacesAndRejectsInvalidRegex(t *testing.T) {
	m := New("pipeline", "", func(context.Context, int) ([]provider.Run, error) { return nil, nil })
	m.query = "fatal"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("error")})
	m = updated.(Model)
	if m.query != "fatal error" {
		t.Fatalf("query = %q, want expression with space", m.query)
	}

	m.query = "["
	m.matchMode = 2
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil || !strings.Contains(m.err, "Invalid regular expression") {
		t.Fatalf("invalid regex was accepted: cmd=%v err=%q", cmd != nil, m.err)
	}
}

func TestSearchUsesBoundedConcurrencyAndContinuesAfterResult(t *testing.T) {
	runs := make([]provider.Run, 6)
	for i := range runs {
		runs[i].ID = fmt.Sprintf("execution-%d", i)
	}
	m := Model{phase: phaseLoading, generation: 1, options: searchOptions{query: "needle"}}

	updated, cmd := m.Update(runsLoadedMsg{generation: 1, runs: runs})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected concurrent search commands")
	}
	if m.active != maxConcurrentSearches || m.nextRun != maxConcurrentSearches {
		t.Fatalf("active=%d next=%d, want %d", m.active, m.nextRun, maxConcurrentSearches)
	}

	updated, cmd = m.Update(runSearchedMsg{generation: 1, match: searchMatch{run: runs[0]}})
	m = updated.(Model)
	if cmd == nil || m.active != maxConcurrentSearches || m.nextRun != maxConcurrentSearches+1 {
		t.Fatalf("replacement search not started: active=%d next=%d cmd=%v", m.active, m.nextRun, cmd != nil)
	}
}

func TestEscapeCancelsSearchAndIgnoresLateResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := Model{phase: phaseSearching, generation: 4, searchCancel: cancel, active: 2}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.phase != phaseResults || m.generation != 5 || !m.cancelled {
		t.Fatalf("search not cancelled: %#v", m)
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("context error = %v, want canceled", ctx.Err())
	}

	updated, _ = m.Update(runSearchedMsg{generation: 4, match: searchMatch{sources: []matchedSource{{content: "late"}}}})
	m = updated.(Model)
	if len(m.matches) != 0 {
		t.Fatal("late result from cancelled generation was applied")
	}
}

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
	if cmd == nil || !strings.Contains(m.View(), "0/2 complete") {
		t.Fatal("expected concurrent execution search progress")
	}

	updated, _ = m.Update(searchRunCmd(context.Background(), m.generation, 0, runs[0], m.options)())
	m = updated.(Model)
	updated, cmd = m.Update(searchRunCmd(context.Background(), m.generation, 1, runs[1], m.options)())
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
	run := provider.Run{ID: "execution-1"}
	source := matchedSource{name: "Build / compile", project: "payments-build", content: "needle"}
	msg := writeSourceLogCmd("pipeline", run, source, false)().(logsWrittenMsg)
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

func TestSelectsAndOpensIndividualLogWithinExecution(t *testing.T) {
	m := Model{
		pipeline: "pipeline",
		phase:    phaseResults,
		matches: []searchMatch{{
			run: provider.Run{ID: "execution-1"},
			sources: []matchedSource{
				{name: "Build / first", project: "first-project", content: "first needle"},
				{name: "Build / second", project: "second-project", content: "second needle"},
			},
		}},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want second matching log", m.cursor)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected selected log write command")
	}
	msg := cmd().(logsWrittenMsg)
	content, err := os.ReadFile(msg.path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "second-project") || strings.Contains(string(content), "first needle") {
		t.Fatalf("opened wrong log:\n%s", content)
	}
	if view := m.View(); !strings.Contains(view, "second-project") {
		t.Fatalf("results missing selected project:\n%s", view)
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
