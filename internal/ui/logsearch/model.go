// Package logsearch provides an interactive search across AWS CodeBuild logs.
package logsearch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/provider"
	"github.com/thiagomarinho/joca/internal/ui/logopen"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

type phase int

const (
	phaseInput phase = iota
	phaseLoading
	phaseSearching
	phaseResults
)

// BackMsg asks the root model to return to the pipeline detail screen.
type BackMsg struct{}

type runsLoadedMsg struct {
	runs []provider.Run
	err  error
}

type runSearchedMsg struct {
	match searchMatch
	err   error
}

type logsWrittenMsg struct {
	path   string
	editor bool
	err    error
}

type pagerClosedMsg struct{ err error }

type recentRunsFunc func(context.Context, int) ([]provider.Run, error)

type searchMatch struct {
	run     provider.Run
	sources []matchedSource
}

type matchedSource struct {
	name    string
	project string
	content string
}

// Model searches CodeBuild logs for recent executions of one pipeline.
type Model struct {
	pipeline   string
	loadRuns   recentRunsFunc
	logEditor  string
	phase      phase
	field      int
	query      string
	depth      string
	err        string
	runs       []provider.Run
	searched   int
	matches    []searchMatch
	errors     []string
	cursor     int
	openStatus string
}

// New creates a log search with a default depth of ten executions.
func New(pipeline, logEditor string, loadRuns recentRunsFunc) Model {
	return Model{pipeline: pipeline, logEditor: logEditor, loadRuns: loadRuns, depth: "10"}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.phase == phaseInput {
			return m.updateInput(msg)
		}
		if m.phase == phaseResults {
			switch msg.String() {
			case "esc", "q":
				return m, func() tea.Msg { return BackMsg{} }
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < m.matchingLogCount()-1 {
					m.cursor++
				}
			case "enter", "l", "e":
				if run, source, ok := m.selectedLog(); ok {
					m.openStatus = "Preparing selected log…"
					return m, writeSourceLogCmd(m.pipeline, run, source, msg.String() == "e")
				}
			}
		}

	case runsLoadedMsg:
		if msg.err != nil {
			m.phase = phaseInput
			m.err = fmt.Sprintf("loading executions: %v", msg.err)
			return m, nil
		}
		m.runs = msg.runs
		m.searched = 0
		m.matches = nil
		m.errors = nil
		if len(m.runs) == 0 {
			m.phase = phaseResults
			return m, nil
		}
		m.phase = phaseSearching
		return m, searchRunCmd(m.runs[0], m.query)

	case runSearchedMsg:
		if msg.err != nil {
			m.errors = append(m.errors, msg.err.Error())
		}
		if len(msg.match.sources) > 0 {
			m.matches = append(m.matches, msg.match)
		}
		m.searched++
		if m.searched < len(m.runs) {
			return m, searchRunCmd(m.runs[m.searched], m.query)
		}
		m.phase = phaseResults

	case logsWrittenMsg:
		if msg.err != nil {
			m.openStatus = fmt.Sprintf("Unable to open logs: %v", msg.err)
			return m, nil
		}
		var cmd *exec.Cmd
		var ok bool
		if msg.editor {
			cmd, ok = logopen.Editor(m.logEditor, msg.path)
		} else {
			cmd, ok = logopen.Pager(msg.path)
		}
		if !ok {
			if msg.editor {
				m.openStatus = "Editor command not found; logs saved to " + msg.path
			} else {
				m.openStatus = "Logs saved to " + msg.path
			}
			return m, nil
		}
		m.openStatus = fmt.Sprintf("Opening logs using %s…", filepath.Base(cmd.Args[0]))
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return pagerClosedMsg{err: err} })

	case pagerClosedMsg:
		if msg.err != nil {
			m.openStatus = fmt.Sprintf("Log viewer failed: %v", msg.err)
		} else {
			m.openStatus = "Returned from logs"
		}
	}
	return m, nil
}

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return BackMsg{} }
	case "tab", "up", "down":
		m.field = 1 - m.field
	case "backspace", "ctrl+h":
		if m.field == 0 && m.query != "" {
			r := []rune(m.query)
			m.query = string(r[:len(r)-1])
		} else if m.field == 1 && m.depth != "" {
			m.depth = m.depth[:len(m.depth)-1]
		}
	case "enter":
		depth, err := strconv.Atoi(m.depth)
		if strings.TrimSpace(m.query) == "" {
			m.err = "Enter a search expression"
			return m, nil
		}
		if err != nil || depth < 1 || depth > 100 {
			m.err = "Execution count must be between 1 and 100"
			return m, nil
		}
		m.err = ""
		m.phase = phaseLoading
		load := m.loadRuns
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			runs, err := load(ctx, depth)
			return runsLoadedMsg{runs: runs, err: err}
		}
	default:
		if msg.Type == tea.KeyRunes {
			if m.field == 0 {
				m.query += string(msg.Runes)
			} else {
				for _, r := range msg.Runes {
					if r >= '0' && r <= '9' {
						m.depth += string(r)
					}
				}
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder
	sb.WriteString("\n  ")
	sb.WriteString(styles.DetailTitle.Render("Search CodeBuild logs — " + m.pipeline))
	sb.WriteString("\n\n")

	switch m.phase {
	case phaseInput:
		query, depth := "  Expression: "+m.query, "  Executions: "+m.depth
		if m.field == 0 {
			query = styles.SelectedRow.Render(query)
		} else {
			depth = styles.SelectedRow.Render(depth)
		}
		sb.WriteString(query + "\n" + depth + "\n")
		if m.err != "" {
			sb.WriteString("\n  " + styles.StatusFailed.Render(m.err) + "\n")
		}
		sb.WriteString("\n  " + styles.Footer.Render("tab: change field  enter: search  esc: back"))
	case phaseLoading:
		fmt.Fprintf(&sb, "  Loading %s executions…", m.depth)
	case phaseSearching:
		fmt.Fprintf(&sb, "  Searching execution %d/%d…\n", m.searched+1, len(m.runs))
		fmt.Fprintf(&sb, "  Matching logs so far: %d", m.matchingLogCount())
	case phaseResults:
		fmt.Fprintf(&sb, "  Expression: %q  ·  searched: %d  ·  matching logs: %d\n\n", m.query, m.searched, m.matchingLogCount())
		if len(m.matches) == 0 {
			sb.WriteString("  No matching executions.\n")
		}
		selection := 0
		for _, match := range m.matches {
			details := string(match.run.Status)
			if match.run.Branch != "" {
				details += "  " + match.run.Branch
			}
			if !match.run.StartedAt.IsZero() {
				details += "  " + match.run.StartedAt.Format("2006-01-02 15:04")
			}
			line := fmt.Sprintf("  #%s  %s  %d matching log(s)", match.run.ID, details, len(match.sources))
			sb.WriteString(line + "\n")
			for _, source := range match.sources {
				project := source.project
				if project == "" {
					project = "unknown project"
				}
				sourceLine := fmt.Sprintf("      %s  ·  %s", project, source.name)
				if selection == m.cursor {
					sourceLine = styles.SelectedRow.Render(sourceLine)
				}
				sb.WriteString(sourceLine + "\n")
				selection++
			}
		}
		if m.err != "" {
			sb.WriteString("\n  " + styles.StatusFailed.Render(m.err) + "\n")
		}
		if len(m.errors) > 0 {
			fmt.Fprintf(&sb, "\n  %s\n", styles.StatusFailed.Render(fmt.Sprintf("%d execution(s) could not be searched", len(m.errors))))
		}
		if m.openStatus != "" {
			sb.WriteString("\n  " + styles.Footer.Render(m.openStatus) + "\n")
		}
		sb.WriteString("\n  " + styles.Footer.Render("↑↓: select  l: open logs in pager  e: open logs in editor  esc: pipeline detail"))
	}
	return sb.String()
}

func searchRunCmd(run provider.Run, query string) tea.Cmd {
	return func() tea.Msg {
		if run.LogSources == nil {
			return runSearchedMsg{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		sources, err := run.LogSources(ctx)
		if err != nil {
			return runSearchedMsg{err: fmt.Errorf("searching execution %s: %w", run.ID, err)}
		}
		match := searchMatch{run: run}
		for _, source := range sources {
			if source.Logs == nil {
				continue
			}
			content, err := source.Logs(ctx)
			if err != nil {
				return runSearchedMsg{err: fmt.Errorf("loading %s/%s for execution %s: %w", source.Stage, source.Action, run.ID, err)}
			}
			if strings.Contains(content, query) {
				match.sources = append(match.sources, matchedSource{
					name:    source.Stage + " / " + source.Action,
					project: source.Project,
					content: content,
				})
			}
		}
		return runSearchedMsg{match: match}
	}
}

func (m Model) matchingLogCount() int {
	total := 0
	for _, match := range m.matches {
		total += len(match.sources)
	}
	return total
}

func (m Model) selectedLog() (provider.Run, matchedSource, bool) {
	selection := 0
	for _, match := range m.matches {
		for _, source := range match.sources {
			if selection == m.cursor {
				return match.run, source, true
			}
			selection++
		}
	}
	return provider.Run{}, matchedSource{}, false
}

func writeSourceLogCmd(pipeline string, run provider.Run, source matchedSource, editor bool) tea.Cmd {
	return func() tea.Msg {
		dir, err := os.MkdirTemp("", "joca-log-search-*")
		if err != nil {
			return logsWrittenMsg{err: fmt.Errorf("creating temporary directory: %w", err)}
		}
		var sb strings.Builder
		project := source.project
		if project == "" {
			project = "unknown project"
		}
		fmt.Fprintf(&sb, "===== CodeBuild project: %s · %s =====\n%s\n", project, source.name, source.content)
		name := safeFilename(strings.Join([]string{pipeline, run.ID, project, source.name}, "-") + ".log")
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
			return logsWrittenMsg{err: fmt.Errorf("writing logs: %w", err)}
		}
		return logsWrittenMsg{path: path, editor: editor}
	}
}

func safeFilename(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.", r) {
			return r
		}
		return '-'
	}, name)
}
