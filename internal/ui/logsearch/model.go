// Package logsearch provides an interactive search across AWS CodeBuild logs.
package logsearch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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

const (
	maxConcurrentSearches = 4
	maxContextLines       = 20
	maxSnippetGroups      = 3
	maxSnippetLines       = 25
	inputFieldCount       = 4
)

// BackMsg asks the root model to return to the pipeline detail screen.
type BackMsg struct{}

type runsLoadedMsg struct {
	generation uint64
	runs       []provider.Run
	err        error
}

type runSearchedMsg struct {
	generation uint64
	match      searchMatch
	err        error
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
	order   int
}

type matchedSource struct {
	name       string
	project    string
	content    string
	matchCount int
	snippets   []string
}

type searchOptions struct {
	query           string
	regex           bool
	caseInsensitive bool
	contextLines    int
}

type matchResult struct {
	count    int
	snippets []string
}

// Model searches CodeBuild logs for recent executions of one pipeline.
type Model struct {
	pipeline     string
	loadRuns     recentRunsFunc
	logEditor    string
	phase        phase
	field        int
	query        string
	depth        string
	context      string
	matchMode    int
	options      searchOptions
	err          string
	runs         []provider.Run
	searched     int
	nextRun      int
	active       int
	generation   uint64
	searchCtx    context.Context
	searchCancel context.CancelFunc
	cancelled    bool
	matches      []searchMatch
	errors       []string
	cursor       int
	openStatus   string
}

// New creates a log search with a default depth of ten executions.
func New(pipeline, logEditor string, loadRuns recentRunsFunc) Model {
	return Model{pipeline: pipeline, logEditor: logEditor, loadRuns: loadRuns, depth: "10", context: "2"}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if (m.phase == phaseLoading || m.phase == phaseSearching) && msg.String() == "esc" {
			if m.searchCancel != nil {
				m.searchCancel()
			}
			m.generation++
			m.active = 0
			m.cancelled = true
			m.phase = phaseResults
			return m, nil
		}
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
		if msg.generation != m.generation {
			return m, nil
		}
		if msg.err != nil {
			m.phase = phaseInput
			m.searchCancel = nil
			m.err = fmt.Sprintf("loading executions: %v", msg.err)
			return m, nil
		}
		m.runs = msg.runs
		m.searched = 0
		m.nextRun = 0
		m.active = 0
		m.matches = nil
		m.errors = nil
		m.cancelled = false
		if len(m.runs) == 0 {
			m.phase = phaseResults
			m.searchCancel = nil
			return m, nil
		}
		m.phase = phaseSearching
		return m, m.startSearches()

	case runSearchedMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		if m.active > 0 {
			m.active--
		}
		if msg.err != nil {
			m.errors = append(m.errors, msg.err.Error())
		}
		if len(msg.match.sources) > 0 {
			m.matches = append(m.matches, msg.match)
			sort.SliceStable(m.matches, func(i, j int) bool {
				return m.matches[i].order < m.matches[j].order
			})
		}
		m.searched++
		if m.nextRun < len(m.runs) {
			return m, m.startSearches()
		}
		if m.active == 0 {
			m.phase = phaseResults
			m.searchCancel = nil
		}

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
	case "tab", "down":
		m.field = (m.field + 1) % inputFieldCount
	case "shift+tab", "up":
		m.field = (m.field - 1 + inputFieldCount) % inputFieldCount
	case "left":
		if m.field == 3 {
			m.matchMode = (m.matchMode - 1 + 4) % 4
		}
	case "right":
		if m.field == 3 {
			m.matchMode = (m.matchMode + 1) % 4
		}
	case " ":
		switch m.field {
		case 3:
			m.matchMode = (m.matchMode + 1) % 4
		case 0:
			m.query += " "
		}
	case "backspace", "ctrl+h":
		switch m.field {
		case 0:
			if m.query != "" {
				r := []rune(m.query)
				m.query = string(r[:len(r)-1])
			}
		case 1:
			if m.depth != "" {
				m.depth = m.depth[:len(m.depth)-1]
			}
		case 2:
			if m.context != "" {
				m.context = m.context[:len(m.context)-1]
			}
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
		contextLines, contextErr := strconv.Atoi(m.context)
		if contextErr != nil || contextLines < 0 || contextLines > maxContextLines {
			m.err = fmt.Sprintf("Context lines must be between 0 and %d", maxContextLines)
			return m, nil
		}
		options := searchOptions{
			query:           m.query,
			regex:           m.matchMode >= 2,
			caseInsensitive: m.matchMode%2 == 1,
			contextLines:    contextLines,
		}
		if options.regex {
			pattern := options.query
			if options.caseInsensitive {
				pattern = "(?i)" + pattern
			}
			if _, err := regexp.Compile(pattern); err != nil {
				m.err = fmt.Sprintf("Invalid regular expression: %v", err)
				return m, nil
			}
		}
		m.err = ""
		m.options = options
		m.phase = phaseLoading
		m.cancelled = false
		m.generation++
		generation := m.generation
		ctx, cancel := context.WithCancel(context.Background())
		m.searchCtx = ctx
		m.searchCancel = cancel
		load := m.loadRuns
		return m, func() tea.Msg {
			loadCtx, loadCancel := context.WithTimeout(ctx, 30*time.Second)
			defer loadCancel()
			runs, err := load(loadCtx, depth)
			return runsLoadedMsg{generation: generation, runs: runs, err: err}
		}
	default:
		if msg.Type == tea.KeyRunes {
			switch m.field {
			case 0:
				m.query += string(msg.Runes)
			case 1, 2:
				for _, r := range msg.Runes {
					if r >= '0' && r <= '9' {
						if m.field == 1 {
							m.depth += string(r)
						} else {
							m.context += string(r)
						}
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
		fields := []string{
			"  Expression: " + m.query,
			"  Executions: " + m.depth,
			"  Context lines: " + m.context,
			"  Match mode: " + m.matchModeLabel(),
		}
		for i, value := range fields {
			if i == m.field {
				value = styles.SelectedRow.Render(value)
			}
			sb.WriteString(value + "\n")
		}
		if m.err != "" {
			sb.WriteString("\n  " + styles.StatusFailed.Render(m.err) + "\n")
		}
		sb.WriteString("\n  " + styles.Footer.Render("tab/↑↓: field  ←→/space: match mode  enter: search  esc: back"))
	case phaseLoading:
		fmt.Fprintf(&sb, "  Loading %s executions…\n\n  esc: cancel", m.depth)
	case phaseSearching:
		fmt.Fprintf(&sb, "  Searching executions: %d/%d complete  ·  %d active\n", m.searched, len(m.runs), m.active)
		fmt.Fprintf(&sb, "  Matching logs so far: %d\n\n  esc: cancel", m.matchingLogCount())
	case phaseResults:
		fmt.Fprintf(&sb, "  Expression: %q  ·  searched: %d  ·  matching logs: %d\n", m.query, m.searched, m.matchingLogCount())
		fmt.Fprintf(&sb, "  Mode: %s  ·  context: %d line(s)\n\n", m.matchModeLabel(), m.options.contextLines)
		if m.cancelled {
			sb.WriteString("  Search cancelled; showing completed results.\n\n")
		}
		if len(m.matches) == 0 {
			sb.WriteString("  No matching logs.\n")
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
				selected := selection == m.cursor
				if selected {
					sourceLine = styles.SelectedRow.Render(sourceLine)
				}
				fmt.Fprintf(&sb, "%s  (%d match(es))\n", sourceLine, source.matchCount)
				if selected {
					for _, snippet := range source.snippets {
						for _, line := range strings.Split(snippet, "\n") {
							fmt.Fprintf(&sb, "        %s\n", styles.Footer.Render(line))
						}
					}
				}
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

func (m *Model) startSearches() tea.Cmd {
	var cmds []tea.Cmd
	for m.active < maxConcurrentSearches && m.nextRun < len(m.runs) {
		order := m.nextRun
		run := m.runs[order]
		m.nextRun++
		m.active++
		cmds = append(cmds, searchRunCmd(m.searchCtx, m.generation, order, run, m.options))
	}
	return tea.Batch(cmds...)
}

func searchRunCmd(parent context.Context, generation uint64, order int, run provider.Run, options searchOptions) tea.Cmd {
	return func() tea.Msg {
		if run.LogSources == nil {
			return runSearchedMsg{generation: generation, match: searchMatch{run: run, order: order}}
		}
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		sources, err := run.LogSources(ctx)
		if err != nil {
			return runSearchedMsg{generation: generation, err: fmt.Errorf("searching execution %s: %w", run.ID, err)}
		}
		match := searchMatch{run: run, order: order}
		for _, source := range sources {
			if source.Logs == nil {
				continue
			}
			content, err := source.Logs(ctx)
			if err != nil {
				return runSearchedMsg{generation: generation, err: fmt.Errorf("loading %s/%s for execution %s: %w", source.Stage, source.Action, run.ID, err)}
			}
			result := findMatches(content, options)
			if result.count > 0 {
				match.sources = append(match.sources, matchedSource{
					name:       source.Stage + " / " + source.Action,
					project:    source.Project,
					content:    content,
					matchCount: result.count,
					snippets:   result.snippets,
				})
			}
		}
		return runSearchedMsg{generation: generation, match: match}
	}
}

func findMatches(content string, options searchOptions) matchResult {
	lines := strings.Split(content, "\n")
	matchingLines := make([]int, 0)
	result := matchResult{}

	var expression *regexp.Regexp
	if options.regex {
		pattern := options.query
		if options.caseInsensitive {
			pattern = "(?i)" + pattern
		}
		expression, _ = regexp.Compile(pattern)
	}

	for i, line := range lines {
		count := 0
		if expression != nil {
			count = len(expression.FindAllStringIndex(line, -1))
		} else {
			haystack, needle := line, options.query
			if options.caseInsensitive {
				haystack = strings.ToLower(haystack)
				needle = strings.ToLower(needle)
			}
			count = strings.Count(haystack, needle)
		}
		if count > 0 {
			result.count += count
			matchingLines = append(matchingLines, i)
		}
	}

	for _, bounds := range contextRanges(matchingLines, len(lines), options.contextLines) {
		if len(result.snippets) >= maxSnippetGroups {
			break
		}
		var snippet strings.Builder
		lastLine := bounds[1]
		truncated := false
		if lastLine-bounds[0]+1 > maxSnippetLines {
			lastLine = bounds[0] + maxSnippetLines - 1
			truncated = true
		}
		for line := bounds[0]; line <= lastLine; line++ {
			if snippet.Len() > 0 {
				snippet.WriteByte('\n')
			}
			fmt.Fprintf(&snippet, "%d  %s", line+1, lines[line])
		}
		if truncated {
			snippet.WriteString("\n…")
		}
		result.snippets = append(result.snippets, snippet.String())
	}
	return result
}

func contextRanges(matches []int, lineCount, contextLines int) [][2]int {
	var ranges [][2]int
	for _, line := range matches {
		start := line - contextLines
		if start < 0 {
			start = 0
		}
		end := line + contextLines
		if end >= lineCount {
			end = lineCount - 1
		}
		if len(ranges) > 0 && start <= ranges[len(ranges)-1][1]+1 {
			if end > ranges[len(ranges)-1][1] {
				ranges[len(ranges)-1][1] = end
			}
			continue
		}
		ranges = append(ranges, [2]int{start, end})
	}
	return ranges
}

func (m Model) matchModeLabel() string {
	return []string{
		"literal · case-sensitive",
		"literal · case-insensitive",
		"regular expression · case-sensitive",
		"regular expression · case-insensitive",
	}[m.matchMode%4]
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
