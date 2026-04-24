package list

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/ui/styles"
)

// FetchedMsg is sent when pipeline data has been refreshed.
type FetchedMsg struct {
	Index int
	Item  PipelineItem
}

// OpenDetailMsg asks the root model to push the detail view.
type OpenDetailMsg struct{ Item PipelineItem }

// OpenAddFormMsg asks the root model to push the add-pipeline form.
type OpenAddFormMsg struct{}

// OpenAutomationsMsg asks the root model to push the automation rules view.
type OpenAutomationsMsg struct{}

// OpenBrowserMsg asks the OS to open a URL.
type OpenBrowserMsg struct{ URL string }

// TogglePauseMsg asks the root model to pause/resume refreshing a pipeline.
type TogglePauseMsg struct{ Index int }

// TriggerMsg asks the root model to re-run the selected pipeline's latest execution.
type TriggerMsg struct{ Index int }

// TriggerNewMsg asks the root model to start a brand-new execution of the selected pipeline.
type TriggerNewMsg struct{ Index int }

// MoveItemMsg is emitted when the user reorders a pipeline row.
type MoveItemMsg struct{ From, To int }

// Model is the pipeline list view.
type Model struct {
	Items  []PipelineItem
	cursor int
	height int
	width  int
}

func New(items []PipelineItem) Model {
	return Model{Items: items}
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
			if m.cursor < len(m.Items)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.Items) > 0 {
				return m, func() tea.Msg { return OpenDetailMsg{Item: m.Items[m.cursor]} }
			}
		case "o":
			if len(m.Items) > 0 {
				item := m.Items[m.cursor]
				url := item.Current.URL
				if url == "" {
					url = item.URL
				}
				return m, func() tea.Msg { return OpenBrowserMsg{URL: url} }
			}
		case "a":
			return m, func() tea.Msg { return OpenAddFormMsg{} }
		case "A":
			return m, func() tea.Msg { return OpenAutomationsMsg{} }
		case " ":
			if len(m.Items) > 0 {
				idx := m.cursor
				return m, func() tea.Msg { return TogglePauseMsg{Index: idx} }
			}
		case "R":
			if len(m.Items) > 0 {
				idx := m.cursor
				return m, func() tea.Msg { return TriggerMsg{Index: idx} }
			}
		case "N":
			if len(m.Items) > 0 {
				idx := m.cursor
				return m, func() tea.Msg { return TriggerNewMsg{Index: idx} }
			}
		case "shift+up":
			if m.cursor > 0 {
				m.Items[m.cursor], m.Items[m.cursor-1] = m.Items[m.cursor-1], m.Items[m.cursor]
				m.cursor--
				from, to := m.cursor+1, m.cursor
				return m, func() tea.Msg { return MoveItemMsg{From: from, To: to} }
			}
		case "shift+down":
			if m.cursor < len(m.Items)-1 {
				m.Items[m.cursor], m.Items[m.cursor+1] = m.Items[m.cursor+1], m.Items[m.cursor]
				m.cursor++
				from, to := m.cursor-1, m.cursor
				return m, func() tea.Msg { return MoveItemMsg{From: from, To: to} }
			}
		}

	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width

	case FetchedMsg:
		if msg.Index >= 0 && msg.Index < len(m.Items) {
			m.Items[msg.Index] = msg.Item
		}
	}
	return m, nil
}

func (m Model) View() string {
	if len(m.Items) == 0 {
		return fmt.Sprintf("\n  No pipelines configured.\n  Press %s to add one.\n",
			styles.FormCursor.Render("a"))
	}

	// Fixed overhead per row (excluding name column):
	// 2 (indent) + 1 (marker) + 1 (space) + 4 (hints) + 1 (space) + nameWidth +
	// 1 (space) + 5 (badge) + 2 (spaces) + 16 (branchRef) + 2 (spaces) +
	// 22 (status) + 1 (space) + 11 (dots)
	const fixedOverhead = 69
	nameWidth := 30
	if m.width > fixedOverhead+10 {
		nameWidth = m.width - fixedOverhead
	}

	var sb strings.Builder
	for i, item := range m.Items {
		sb.WriteString("  ")
		sb.WriteString(item.Render(i == m.cursor, nameWidth))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// Selected returns the currently highlighted item, or false if list is empty.
func (m Model) Selected() (PipelineItem, bool) {
	if len(m.Items) == 0 {
		return PipelineItem{}, false
	}
	return m.Items[m.cursor], true
}
