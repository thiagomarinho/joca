package detail

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/provider"
	"github.com/thiagomarinho/joca/internal/ui/list"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

type logLoadedMsg struct{ content string }

type logSourcesLoadedMsg struct {
	sources []provider.LogSource
	err     error
}

type logDownloadedMsg struct {
	path    string
	content string
	err     error
}

type pagerClosedMsg struct {
	path string
	err  error
}

// Model shows the detail view for a selected pipeline.
type Model struct {
	item       list.PipelineItem
	cursor     int // -1 = no run selected (opens pipeline page); 0..n = history index
	logOffset  int
	logs       string
	loading    bool
	logSources []provider.LogSource
	logCursor  int
	logMessage string
	height     int
}

func New(item list.PipelineItem) Model {
	return Model{item: item, cursor: -1}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if len(m.logSources) > 1 {
			switch msg.String() {
			case "esc", "q":
				m.logSources = nil
				m.logCursor = 0
				return m, nil
			case "up", "k":
				if m.logCursor > 0 {
					m.logCursor--
				}
				return m, nil
			case "down", "j":
				if m.logCursor < len(m.logSources)-1 {
					m.logCursor++
				}
				return m, nil
			case "enter", "l":
				m.loading = true
				return m, downloadLogCmd(m.logSources[m.logCursor])
			}
		}
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return backMsg{} }
		case "o":
			var url string
			if m.cursor >= 0 && m.cursor < len(m.item.History) {
				url = m.item.History[m.cursor].URL
			} else {
				url = m.item.URL
			}
			return m, func() tea.Msg { return list.OpenBrowserMsg{URL: url} }
		case "l":
			if run, ok := m.selectedLogRun(); ok && run.LogSources != nil && !m.loading {
				m.loading = true
				m.logMessage = "Discovering CodeBuild actions…"
				loadSources := run.LogSources
				return m, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					sources, err := loadSources(ctx)
					return logSourcesLoadedMsg{sources: sources, err: err}
				}
			}
			if run, ok := m.selectedLogRun(); ok && run.Logs != nil && m.logs == "" && !m.loading {
				m.loading = true
				m.logMessage = "Loading logs…"
				logFn := run.Logs
				return m, func() tea.Msg {
					content, err := logFn(context.Background())
					if err != nil {
						return logLoadedMsg{content: fmt.Sprintf("error loading logs: %v", err)}
					}
					return logLoadedMsg{content: content}
				}
			}
		case "up", "k":
			if m.logs != "" {
				if m.logOffset > 0 {
					m.logOffset--
				}
			} else if m.cursor > -1 {
				m.cursor--
			}
		case "down", "j":
			if m.logs != "" {
				m.logOffset++
			} else if m.cursor < len(m.item.History)-1 {
				m.cursor++
			}
		}

	case logLoadedMsg:
		m.loading = false
		m.logs = msg.content

	case logSourcesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.logMessage = fmt.Sprintf("Unable to discover logs: %v", msg.err)
			return m, nil
		}
		if len(msg.sources) == 0 {
			m.logMessage = "No CodeBuild actions with CloudWatch logs found for this execution"
			return m, nil
		}
		m.logSources = msg.sources
		m.logCursor = 0
		if len(msg.sources) == 1 {
			m.loading = true
			return m, downloadLogCmd(msg.sources[0])
		}
		m.logMessage = ""

	case logDownloadedMsg:
		m.loading = false
		if msg.err != nil {
			m.logMessage = fmt.Sprintf("Unable to download logs: %v", msg.err)
			return m, nil
		}
		m.logSources = nil
		if cmd, ok := pagerCommand(msg.path); ok {
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return pagerClosedMsg{path: msg.path, err: err}
			})
		}
		m.logs = msg.content
		m.logMessage = fmt.Sprintf("Logs downloaded to %s", msg.path)

	case pagerClosedMsg:
		if msg.err != nil {
			m.logMessage = fmt.Sprintf("Pager failed: %v (logs: %s)", msg.err, msg.path)
		} else {
			m.logMessage = fmt.Sprintf("Logs downloaded to %s", msg.path)
		}

	case tea.WindowSizeMsg:
		m.height = msg.Height
	}
	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder
	item := m.item

	sb.WriteString("\n  ")
	sb.WriteString(styles.DetailTitle.Render(item.Entry.Name))
	sb.WriteString("\n\n")

	write := func(label, value string) {
		sb.WriteString("  ")
		sb.WriteString(styles.DetailLabel.Render(label))
		sb.WriteString(styles.DetailValue.Render(value))
		sb.WriteByte('\n')
	}

	write("Provider", string(item.Entry.Provider))
	write("Status", string(item.Current.Status))
	if !item.Current.StartedAt.IsZero() {
		write("Last run", item.Current.StartedAt.Format(time.RFC1123))
	}
	if item.Current.Branch != "" {
		write("Branch", item.Current.Branch)
	}
	write("URL", item.Current.URL)

	sb.WriteString("\n  ")
	sb.WriteString(styles.Header.Render("Recent runs"))
	sb.WriteByte('\n')

	if len(item.History) == 0 {
		sb.WriteString("  No history available.\n")
	} else {
		for i, r := range item.History {
			if i >= 8 {
				break
			}
			dot := renderRunDot(r.Status)
			age := ""
			if !r.StartedAt.IsZero() {
				age = "  " + humanAge(r.StartedAt)
			}
			statusText := renderDetailStatus(r)
			line := fmt.Sprintf("  %s  #%s  %-8s  %s%s",
				dot, r.ID, r.Branch, statusText, age)
			switch {
			case i == m.cursor:
				sb.WriteString(styles.SelectedRow.Render(line))
			case r.Status == provider.StatusApproval:
				sb.WriteString(styles.ApprovalRow.Render(line))
			default:
				sb.WriteString(line)
			}
			sb.WriteByte('\n')
		}
	}

	switch {
	case len(m.logSources) > 1:
		sb.WriteString("\n  ")
		sb.WriteString(styles.Header.Render("CodeBuild actions"))
		sb.WriteByte('\n')
		for i, source := range m.logSources {
			line := fmt.Sprintf("  %s / %s  (%s · %s)", source.Stage, source.Action, source.Project, source.Status)
			if i == m.logCursor {
				line = styles.SelectedRow.Render(line)
			}
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	case m.logs != "":
		sb.WriteString("\n")
		sb.WriteString(styles.LogContainer.Render(m.logs))
	case m.loading:
		sb.WriteString("\n  ")
		sb.WriteString(m.logMessage)
		sb.WriteByte('\n')
	case m.hasLogs():
		sb.WriteString("\n  ")
		sb.WriteString(styles.Footer.Render("Press l to download logs"))
		sb.WriteByte('\n')
	}
	if m.logMessage != "" && !m.loading {
		sb.WriteString("\n  ")
		sb.WriteString(styles.Footer.Render(m.logMessage))
		sb.WriteByte('\n')
	}

	sb.WriteString("\n  ")
	sb.WriteString(styles.Footer.Render("↑↓: select run  o: open in browser  esc: back"))
	return sb.String()
}

func (m Model) selectedLogRun() (provider.Run, bool) {
	if m.cursor >= 0 && m.cursor < len(m.item.History) {
		run := m.item.History[m.cursor]
		return run, run.Logs != nil || run.LogSources != nil
	}
	if m.item.Current.Logs != nil || m.item.Current.LogSources != nil {
		return m.item.Current, true
	}
	if len(m.item.History) > 0 {
		run := m.item.History[0]
		return run, run.Logs != nil || run.LogSources != nil
	}
	return provider.Run{}, false
}

func (m Model) hasLogs() bool {
	_, ok := m.selectedLogRun()
	return ok
}

func downloadLogCmd(source provider.LogSource) tea.Cmd {
	return func() tea.Msg {
		if source.Logs == nil {
			return logDownloadedMsg{err: fmt.Errorf("log source is not available")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		content, err := source.Logs(ctx)
		if err != nil {
			return logDownloadedMsg{err: err}
		}
		dir, err := os.MkdirTemp("", "joca-logs-*")
		if err != nil {
			return logDownloadedMsg{err: fmt.Errorf("creating temporary directory: %w", err)}
		}
		name := safeLogFilename(source.Stage + "-" + source.Action + ".log")
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return logDownloadedMsg{err: fmt.Errorf("writing temporary log file: %w", err)}
		}
		return logDownloadedMsg{path: path, content: content}
	}
}

func safeLogFilename(name string) string {
	var sb strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			sb.WriteRune(r)
		default:
			sb.WriteByte('-')
		}
	}
	return sb.String()
}

func pagerCommand(path string) (*exec.Cmd, bool) {
	if configured := strings.Fields(os.Getenv("PAGER")); len(configured) > 0 {
		if executable, err := exec.LookPath(configured[0]); err == nil {
			return exec.Command(executable, append(configured[1:], path)...), true
		}
	}
	if executable, err := exec.LookPath("bat"); err == nil {
		return exec.Command(executable, "--paging=always", path), true
	}
	if executable, err := exec.LookPath("less"); err == nil {
		return exec.Command(executable, "-R", path), true
	}
	return nil, false
}

func renderDetailStatus(r provider.Run) string {
	switch r.Status {
	case provider.StatusRunning:
		return styles.StatusRunning.Render(string(r.Status))
	case provider.StatusSuccess:
		return styles.StatusSuccess.Render(string(r.Status))
	case provider.StatusFailed:
		return styles.StatusFailed.Render(string(r.Status))
	case provider.StatusApproval:
		if r.Stage != "" {
			return styles.StatusApproval.Render("⏸ → " + r.Stage)
		}
		return styles.StatusApproval.Render("⏸ awaiting approval")
	case provider.StatusCancelled:
		return styles.StatusCancelled.Render(string(r.Status))
	case provider.StatusPending:
		return styles.StatusPending.Render(string(r.Status))
	default:
		return styles.StatusUnknown.Render(string(r.Status))
	}
}

func renderRunDot(s provider.Status) string {
	switch s {
	case provider.StatusSuccess:
		return styles.DotSuccess.Render(styles.DotFull)
	case provider.StatusFailed:
		return styles.DotFailed.Render(styles.DotFull)
	case provider.StatusRunning:
		return styles.DotRunning.Render(styles.DotFull)
	case provider.StatusApproval:
		return styles.DotApproval.Render(styles.DotFull)
	case provider.StatusCancelled:
		return styles.DotCancelled.Render(styles.DotFull)
	}
	return styles.DotOther.Render(styles.DotEmpty)
}

func humanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// backMsg signals the root model to pop this view.
type backMsg struct{}
