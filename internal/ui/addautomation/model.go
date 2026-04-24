package addautomation

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

type step int

const (
	stepWatchPipeline step = iota
	stepOnStatus
	stepTriggerPipeline
	stepRepeat
)

// SavedMsg is emitted when the user completes the wizard.
type SavedMsg struct{ Rules []config.AutomationRule }

// CancelledMsg is emitted when the user cancels.
type CancelledMsg struct{}

var availableStatuses = []string{"success", "failed", "cancelled"}
var repeatOptions = []string{"every time (unlimited)", "once", "custom number of times"}

// Model guides the user through creating an automation rule.
type Model struct {
	pipelines     []string // available pipeline names
	allowChains   bool     // mirrors AppConfig.AutomationAllowChains
	existingRules []config.AutomationRule
	step          step

	watchCursor int

	statusCursor int

	triggerCursor   int
	triggerSelected map[int]bool

	repeatCursor   int
	customTimesStr string
	inputingTimes  bool

	err string
}

// New creates the wizard with the given available pipeline names.
func New(pipelines []string, existingRules []config.AutomationRule, allowChains bool) Model {
	return Model{
		pipelines:       pipelines,
		existingRules:   existingRules,
		allowChains:     allowChains,
		triggerSelected: make(map[int]bool),
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return CancelledMsg{} }

		case "up", "k":
			m.err = ""
			if !m.inputingTimes {
				m.moveCursorUp()
			}

		case "down", "j":
			m.err = ""
			if !m.inputingTimes {
				m.moveCursorDown()
			}

		case " ":
			if m.step == stepTriggerPipeline && !m.inputingTimes {
				m.err = ""
				m.triggerSelected[m.triggerCursor] = !m.triggerSelected[m.triggerCursor]
			}

		case "enter":
			m.err = ""
			if m.inputingTimes {
				if m.customTimesStr == "" {
					m.err = "enter a number of times"
					return m, nil
				}
				n := 0
				if _, err := fmt.Sscanf(m.customTimesStr, "%d", &n); err != nil || n <= 0 {
					m.err = "must be a positive integer"
					return m, nil
				}
				return m, m.finishCmd(n)
			}
			return m, m.advance()

		case "backspace":
			if m.inputingTimes && len(m.customTimesStr) > 0 {
				m.customTimesStr = m.customTimesStr[:len(m.customTimesStr)-1]
			}

		default:
			if m.inputingTimes {
				ch := msg.String()
				if len(ch) == 1 && ch >= "0" && ch <= "9" {
					m.customTimesStr += ch
				}
			}
		}

	case tea.WindowSizeMsg:
		// no-op
	}
	return m, nil
}

func (m *Model) moveCursorUp() {
	switch m.step {
	case stepWatchPipeline:
		if m.watchCursor > 0 {
			m.watchCursor--
		}
	case stepOnStatus:
		if m.statusCursor > 0 {
			m.statusCursor--
		}
	case stepTriggerPipeline:
		if m.triggerCursor > 0 {
			m.triggerCursor--
		}
	case stepRepeat:
		if m.repeatCursor > 0 {
			m.repeatCursor--
		}
	}
}

func (m *Model) moveCursorDown() {
	switch m.step {
	case stepWatchPipeline:
		if m.watchCursor < len(m.pipelines)-1 {
			m.watchCursor++
		}
	case stepOnStatus:
		if m.statusCursor < len(availableStatuses)-1 {
			m.statusCursor++
		}
	case stepTriggerPipeline:
		if m.triggerCursor < len(m.pipelines)-1 {
			m.triggerCursor++
		}
	case stepRepeat:
		if m.repeatCursor < len(repeatOptions)-1 {
			m.repeatCursor++
		}
	}
}

func (m *Model) advance() tea.Cmd {
	switch m.step {
	case stepWatchPipeline:
		if len(m.pipelines) == 0 {
			m.err = "no pipelines configured; add one first"
			return nil
		}
		m.step = stepOnStatus

	case stepOnStatus:
		m.step = stepTriggerPipeline

	case stepTriggerPipeline:
		if len(m.pipelines) == 0 {
			m.err = "no pipelines configured"
			return nil
		}
		if len(m.triggerSelected) == 0 {
			m.err = "select at least one pipeline"
			return nil
		}
		watch := m.pipelines[m.watchCursor]
		for idx := range m.triggerSelected {
			if !m.triggerSelected[idx] {
				continue
			}
			trigger := m.pipelines[idx]
			candidate := config.AutomationRule{
				WatchPipeline:   watch,
				TriggerPipeline: trigger,
			}
			if err := config.CheckAutomationCycle(m.existingRules, candidate, m.allowChains); err != nil {
				m.err = err.Error()
				return nil
			}
		}
		m.step = stepRepeat

	case stepRepeat:
		switch m.repeatCursor {
		case 0:
			return m.finishCmd(0)
		case 1:
			return m.finishCmd(1)
		case 2:
			m.inputingTimes = true
		}
	}
	return nil
}

func (m *Model) finishCmd(maxFires int) tea.Cmd {
	watch := m.pipelines[m.watchCursor]
	onStatus := availableStatuses[m.statusCursor]
	var rules []config.AutomationRule
	for idx := range m.triggerSelected {
		if !m.triggerSelected[idx] {
			continue
		}
		trigger := m.pipelines[idx]
		name := fmt.Sprintf("%s→%s→%s", watch, onStatus, trigger)
		rules = append(rules, config.AutomationRule{
			Name:            name,
			WatchPipeline:   watch,
			OnStatus:        onStatus,
			TriggerPipeline: trigger,
			MaxFires:        maxFires,
		})
	}
	return func() tea.Msg { return SavedMsg{Rules: rules} }
}

func (m Model) View() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styles.FormTitle.Render("  Add Automation Rule"))
	sb.WriteString("\n\n")

	steps := []string{"watch pipeline", "on status", "trigger pipeline", "repeat"}
	var crumbs []string
	for i, s := range steps {
		switch {
		case step(i) < m.step:
			crumbs = append(crumbs, styles.CredOK.Render("✓ "+s))
		case step(i) == m.step:
			crumbs = append(crumbs, styles.FormInput.Render(s))
		default:
			crumbs = append(crumbs, styles.Footer.Render(s))
		}
	}
	sb.WriteString("  ")
	sb.WriteString(strings.Join(crumbs, styles.Footer.Render(" › ")))
	sb.WriteString("\n\n")

	switch m.step {
	case stepWatchPipeline:
		sb.WriteString("  Which pipeline should be watched?\n\n")
		sb.WriteString(m.renderList(m.pipelines, m.watchCursor))

	case stepOnStatus:
		watch := ""
		if len(m.pipelines) > 0 {
			watch = m.pipelines[m.watchCursor]
		}
		fmt.Fprintf(&sb, "  Fire when %s transitions to:\n\n",
			styles.FormInput.Render(watch))
		sb.WriteString(m.renderStatusList())

	case stepTriggerPipeline:
		onStatus := availableStatuses[m.statusCursor]
		watch := ""
		if len(m.pipelines) > 0 {
			watch = m.pipelines[m.watchCursor]
		}
		fmt.Fprintf(&sb, "  When %s → %s, trigger which pipeline(s)?\n\n",
			styles.FormInput.Render(watch),
			styleStatus(onStatus).Render(onStatus))
		sb.WriteString(m.renderTriggerList())

	case stepRepeat:
		sb.WriteString("  How many times should this rule fire?\n\n")
		if m.inputingTimes {
			fmt.Fprintf(&sb, "  Number of times: %s%s\n",
				styles.FormInput.Render(m.customTimesStr),
				styles.FormCursor.Render("█"))
		} else {
			for i, opt := range repeatOptions {
				marker := "  "
				if i == m.repeatCursor {
					marker = styles.MarkerSelected.Render("▶ ")
				}
				fmt.Fprintf(&sb, "  %s%s\n", marker, opt)
			}
		}
	}

	if m.err != "" {
		sb.WriteString("\n  ")
		sb.WriteString(styles.FormError.Render(m.err))
		sb.WriteByte('\n')
	}

	sb.WriteString("\n")
	if m.step == stepTriggerPipeline {
		sb.WriteString(styles.Footer.Render("  ↑↓: navigate  space: toggle  enter: confirm  esc: cancel"))
	} else {
		sb.WriteString(styles.Footer.Render("  ↑↓: select  enter: confirm  esc: cancel"))
	}

	return sb.String()
}

func (m Model) renderList(items []string, cursor int) string {
	if len(items) == 0 {
		return styles.FormError.Render("  no pipelines configured") + "\n"
	}
	var sb strings.Builder
	for i, item := range items {
		marker := "  "
		if i == cursor {
			marker = styles.MarkerSelected.Render("▶ ")
		}
		fmt.Fprintf(&sb, "  %s%s\n", marker, item)
	}
	return sb.String()
}

func (m Model) renderTriggerList() string {
	if len(m.pipelines) == 0 {
		return styles.FormError.Render("  no pipelines configured") + "\n"
	}
	var sb strings.Builder
	for i, item := range m.pipelines {
		marker := "  "
		if i == m.triggerCursor {
			marker = styles.MarkerSelected.Render("▶ ")
		}
		checkbox := "[ ]"
		if m.triggerSelected[i] {
			checkbox = styles.CredOK.Render("[x]")
		}
		fmt.Fprintf(&sb, "  %s%s %s\n", marker, checkbox, item)
	}
	return sb.String()
}

func (m Model) renderStatusList() string {
	var sb strings.Builder
	for i, s := range availableStatuses {
		marker := "  "
		if i == m.statusCursor {
			marker = styles.MarkerSelected.Render("▶ ")
		}
		fmt.Fprintf(&sb, "  %s%s\n", marker, styleStatus(s).Render(s))
	}
	return sb.String()
}

func styleStatus(s string) lipgloss.Style {
	switch s {
	case "success":
		return styles.StatusSuccess
	case "failed":
		return styles.StatusFailed
	case "cancelled":
		return styles.StatusCancelled
	default:
		return styles.Footer
	}
}
