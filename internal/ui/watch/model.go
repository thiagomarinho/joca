package watch

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/provider"
	"github.com/thiagomarinho/joca/internal/ui/list"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

type backMsg struct{}

// Model is the watch dashboard view.
type Model struct {
	items  []list.PipelineItem
	cursor int
	height int
	width  int
}

func New(items []list.PipelineItem) Model {
	return Model{items: items}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "esc", "q":
			return m, func() tea.Msg { return backMsg{} }
		case "o":
			if len(m.items) > 0 {
				item := m.items[m.cursor]
				url := item.Current.URL
				if url == "" {
					url = item.URL
				}
				return m, func() tea.Msg { return list.OpenBrowserMsg{URL: url} }
			}
		}
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
	}
	return m, nil
}

func (m Model) View() string {
	if len(m.items) == 0 {
		return "\n  No pipelines selected.\n"
	}

	var sb strings.Builder
	sb.WriteString("\n")
	for i, item := range m.items {
		sb.WriteString("  ")
		sb.WriteString(renderRow(item, i == m.cursor))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// UpdateItem replaces the matching item (by name) with the updated one.
func (m Model) UpdateItem(item list.PipelineItem) Model {
	for i, it := range m.items {
		if it.Entry.Name == item.Entry.Name {
			m.items[i] = item
			break
		}
	}
	return m
}

func renderRow(item list.PipelineItem, highlighted bool) string {
	name := styles.PipelineName.Render(truncate(item.Entry.Name, 20))
	badge := renderBadge(item.Entry.Provider)
	status := renderStatus(item.Current.Status, item.Err)

	stage := fmt.Sprintf("%-12s", "")
	if item.Current.Stage != "" {
		stage = fmt.Sprintf("%-12s", "▸ "+truncate(item.Current.Stage, 9))
	}

	branch := fmt.Sprintf("%-12s", truncate(item.Current.Branch, 11))

	age := "       "
	if !item.Current.StartedAt.IsZero() {
		age = humanAge(time.Since(item.Current.StartedAt))
	}

	line := fmt.Sprintf("%s %s  %-22s  %s %s %s", name, badge, status, stage, branch, age)
	if highlighted {
		return styles.SelectedRow.Render(line)
	}
	return styles.UnselectedRow.Render(line)
}

func renderBadge(k config.ProviderKind) string {
	switch k {
	case config.ProviderGitHub:
		return styles.BadgeGH.Render("GH")
	case config.ProviderAWS:
		return styles.BadgeAWS.Render("AWS")
	}
	return "   "
}

func renderStatus(s provider.Status, err error) string {
	if err != nil {
		if provider.IsSSOError(err) {
			return styles.StatusFailed.Render("✗ SSO expired")
		}
		if provider.IsCredentialError(err) {
			return styles.StatusFailed.Render("✗ no credentials")
		}
		return styles.StatusFailed.Render("✗ error")
	}
	switch s {
	case provider.StatusRunning:
		return styles.StatusRunning.Render("● running")
	case provider.StatusSuccess:
		return styles.StatusSuccess.Render("✓ idle")
	case provider.StatusFailed:
		return styles.StatusFailed.Render("✗ failed")
	case provider.StatusApproval:
		return styles.StatusApproval.Render("⏸ awaiting approval")
	case provider.StatusCancelled:
		return styles.StatusCancelled.Render("⊘ cancelled")
	case provider.StatusPending:
		return styles.StatusPending.Render("… pending")
	case provider.StatusIdle:
		return styles.StatusIdle.Render("  idle")
	}
	return styles.StatusUnknown.Render("? unknown")
}

func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
