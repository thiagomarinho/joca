package automations

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

// SavedMsg is emitted after the user adds a rule via the add-automation wizard.
type SavedMsg struct{ Rule config.AutomationRule }

// DeletedMsg is emitted when the user deletes a rule.
type DeletedMsg struct{ Name string }

// ToggledMsg is emitted when the user enables/disables or resets a rule.
type ToggledMsg struct{ Name string }

// OpenAddMsg asks the root model to push the add-automation wizard.
type OpenAddMsg struct{}

// backMsg pops this view off the stack.
type backMsg struct{}

// Model is the automation rules list view.
type Model struct {
	Rules  []config.AutomationRule
	cursor int
	height int
	width  int
	err    string
}

// New creates the model with the given rules slice.
func New(rules []config.AutomationRule) Model {
	r := make([]config.AutomationRule, len(rules))
	copy(r, rules)
	return Model{Rules: r}
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
			if m.cursor < len(m.Rules)-1 {
				m.cursor++
			}
		case "esc", "q":
			return m, func() tea.Msg { return backMsg{} }
		case "n", "a":
			return m, func() tea.Msg { return OpenAddMsg{} }
		case "d":
			if len(m.Rules) > 0 {
				name := m.Rules[m.cursor].Name
				m.Rules = append(m.Rules[:m.cursor], m.Rules[m.cursor+1:]...)
				if m.cursor > 0 && m.cursor >= len(m.Rules) {
					m.cursor = len(m.Rules) - 1
				}
				n := name
				return m, func() tea.Msg { return DeletedMsg{Name: n} }
			}
		case "e":
			if len(m.Rules) > 0 {
				name := m.Rules[m.cursor].Name
				m.Rules[m.cursor].Disabled = !m.Rules[m.cursor].Disabled
				n := name
				return m, func() tea.Msg { return ToggledMsg{Name: n} }
			}
		case "r":
			if len(m.Rules) > 0 {
				m.Rules[m.cursor].FireCount = 0
				m.Rules[m.cursor].Disabled = false
				name := m.Rules[m.cursor].Name
				n := name
				return m, func() tea.Msg { return ToggledMsg{Name: n} }
			}
		}

	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width

	case SavedMsg:
		m.Rules = append(m.Rules, msg.Rule)
	}
	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(styles.FormTitle.Render("  Automation Rules"))
	sb.WriteString("\n\n")

	if len(m.Rules) == 0 {
		fmt.Fprintf(&sb, "  No rules configured. Press %s to add one.\n",
			styles.FormCursor.Render("n"))
	} else {
		for i, r := range m.Rules {
			selected := i == m.cursor
			marker := "  "
			if selected {
				marker = styles.MarkerSelected.Render("▶ ")
			}

			maxStr := "∞"
			if r.MaxFires > 0 {
				maxStr = fmt.Sprintf("%d", r.MaxFires)
			}

			statusStyle := styles.CredOK
			statusLabel := "active"
			if r.Disabled {
				statusStyle = styles.CredMissing
				if r.MaxFires > 0 && r.FireCount >= r.MaxFires {
					statusLabel = "exhausted"
				} else {
					statusLabel = "disabled"
				}
			}

			onStyle := styles.StatusSuccess
			switch r.OnStatus {
			case "failed":
				onStyle = styles.StatusFailed
			case "cancelled":
				onStyle = styles.StatusCancelled
			}

			line := fmt.Sprintf("%s%-22s  when %-20s → %-8s  trigger %-20s  fires %s/%s  [%s]",
				marker,
				truncate(r.Name, 22),
				truncate(r.WatchPipeline, 20),
				onStyle.Render(r.OnStatus),
				truncate(r.TriggerPipeline, 20),
				styles.Footer.Render(fmt.Sprintf("%d", r.FireCount)),
				styles.Footer.Render(maxStr),
				statusStyle.Render(statusLabel),
			)
			if selected {
				sb.WriteString(styles.SelectedRow.Render(line))
			} else {
				sb.WriteString(line)
			}
			sb.WriteByte('\n')
		}
	}

	if m.err != "" {
		sb.WriteString("\n  ")
		sb.WriteString(styles.FormError.Render(m.err))
		sb.WriteByte('\n')
	}

	sb.WriteString("\n")
	sb.WriteString(styles.Footer.Render(
		"  n: add  d: delete  e: enable/disable  r: reset  ↑↓: navigate  esc: back",
	))

	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
