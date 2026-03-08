package detail

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/provider"
	"github.com/thiagomarinho/joca/internal/ui/list"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

type logLoadedMsg struct{ content string }

// Model shows the detail view for a selected pipeline.
type Model struct {
	item      list.PipelineItem
	cursor    int // -1 = no run selected (opens pipeline page); 0..n = history index
	logOffset int
	logs      string
	loading   bool
	height    int
}

func New(item list.PipelineItem) Model {
	return Model{item: item, cursor: -1}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
			if m.item.Current.Logs != nil && m.logs == "" && !m.loading {
				m.loading = true
				logFn := m.item.Current.Logs
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
			line := fmt.Sprintf("  %s  #%s  %-8s  %s%s",
				dot, r.ID, r.Branch, string(r.Status), age)
			if i == m.cursor {
				sb.WriteString(styles.SelectedRow.Render(line))
			} else {
				sb.WriteString(line)
			}
			sb.WriteByte('\n')
		}
	}

	switch {
	case m.logs != "":
		sb.WriteString("\n")
		sb.WriteString(styles.LogContainer.Render(m.logs))
	case m.loading:
		sb.WriteString("\n  Loading logs…\n")
	case item.Current.Logs != nil:
		sb.WriteString("\n  ")
		sb.WriteString(styles.Footer.Render("Press l to load logs"))
		sb.WriteByte('\n')
	}

	sb.WriteString("\n  ")
	sb.WriteString(styles.Footer.Render("↑↓: select run  o: open in browser  esc: back"))
	return sb.String()
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
