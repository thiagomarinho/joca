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
	Items      []PipelineItem
	cursor     int
	height     int
	width      int
	HidePaused bool // when true, paused pipelines are not rendered
}

// visibleIdx returns the indices of Items that are currently shown.
// When HidePaused is false every index is returned, keeping cursor semantics identical.
func (m Model) visibleIdx() []int {
	if !m.HidePaused {
		idx := make([]int, len(m.Items))
		for i := range m.Items {
			idx[i] = i
		}
		return idx
	}
	var idx []int
	for i, item := range m.Items {
		if !item.Paused {
			idx = append(idx, i)
		}
	}
	return idx
}

func New(items []PipelineItem) Model {
	return Model{Items: items}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		vi := m.visibleIdx()
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(vi)-1 {
				m.cursor++
			}
		case "h":
			m.HidePaused = !m.HidePaused
			vi = m.visibleIdx()
			if len(vi) == 0 {
				m.cursor = 0
			} else if m.cursor >= len(vi) {
				m.cursor = len(vi) - 1
			}
		case "enter":
			if len(vi) > 0 {
				item := m.Items[vi[m.cursor]]
				return m, func() tea.Msg { return OpenDetailMsg{Item: item} }
			}
		case "o":
			if len(vi) > 0 {
				item := m.Items[vi[m.cursor]]
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
			if len(vi) > 0 {
				idx := vi[m.cursor]
				return m, func() tea.Msg { return TogglePauseMsg{Index: idx} }
			}
		case "R":
			if len(vi) > 0 {
				idx := vi[m.cursor]
				return m, func() tea.Msg { return TriggerMsg{Index: idx} }
			}
		case "N":
			if len(vi) > 0 {
				idx := vi[m.cursor]
				return m, func() tea.Msg { return TriggerNewMsg{Index: idx} }
			}
		case "shift+up":
			if m.cursor > 0 {
				realCur, realPrev := vi[m.cursor], vi[m.cursor-1]
				m.Items[realCur], m.Items[realPrev] = m.Items[realPrev], m.Items[realCur]
				m.cursor--
				from, to := realCur, realPrev
				return m, func() tea.Msg { return MoveItemMsg{From: from, To: to} }
			}
		case "shift+down":
			if m.cursor < len(vi)-1 {
				realCur, realNext := vi[m.cursor], vi[m.cursor+1]
				m.Items[realCur], m.Items[realNext] = m.Items[realNext], m.Items[realCur]
				m.cursor++
				from, to := realCur, realNext
				return m, func() tea.Msg { return MoveItemMsg{From: from, To: to} }
			}
		}

	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width

	case FetchedMsg:
		if msg.Index >= 0 && msg.Index < len(m.Items) {
			m.Items[msg.Index] = msg.Item
			// A pipeline may have become paused/unpaused; re-clamp cursor.
			if vi := m.visibleIdx(); m.cursor >= len(vi) && len(vi) > 0 {
				m.cursor = len(vi) - 1
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	if len(m.Items) == 0 {
		return fmt.Sprintf("\n  No pipelines configured.\n  Press %s to add one.\n",
			styles.FormCursor.Render("a"))
	}

	vi := m.visibleIdx()
	if len(vi) == 0 {
		return fmt.Sprintf("\n  All pipelines are paused. Press %s to show them.\n",
			styles.FormCursor.Render("h"))
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
	for visPos, realIdx := range vi {
		sb.WriteString("  ")
		sb.WriteString(m.Items[realIdx].Render(visPos == m.cursor, nameWidth))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// Selected returns the currently highlighted item, or false if the visible list is empty.
func (m Model) Selected() (PipelineItem, bool) {
	vi := m.visibleIdx()
	if len(vi) == 0 {
		return PipelineItem{}, false
	}
	return m.Items[vi[m.cursor]], true
}
