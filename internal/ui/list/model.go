package list

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
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

// OpenCopyMsg asks the root model to copy the selected pipeline.
type OpenCopyMsg struct{ Entry config.PipelineEntry }

// OpenDeleteMsg asks the root model to confirm deletion of the selected pipeline.
type OpenDeleteMsg struct{ Entry config.PipelineEntry }

// OpenAutomationsMsg asks the root model to push the automation rules view.
type OpenAutomationsMsg struct{}

// OpenBrowserMsg asks the OS to open a URL.
type OpenBrowserMsg struct{ URL string }

// TogglePauseMsg asks the root model to pause/resume refreshing a pipeline.
type TogglePauseMsg struct{ Index int }

// FocusMsg asks the root model to resume one pipeline and pause all others.
type FocusMsg struct{ Index int }

// TriggerMsg asks the root model to re-run the selected pipeline's latest execution.
type TriggerMsg struct{ Index int }

// TriggerNewMsg asks the root model to start a brand-new execution of the selected pipeline.
type TriggerNewMsg struct{ Index int }

// MoveItemMsg is emitted when the user reorders a pipeline row.
type MoveItemMsg struct{ From, To int }

// Model is the pipeline list view.
type Model struct {
	Items         []PipelineItem
	cursor        int
	height        int
	width         int
	viewportStart int
	HidePaused    bool   // when true, paused pipelines are not rendered
	searching     bool   // true while the search prompt is open
	query         string // current fuzzy-filter query
}

// IsSearching reports whether the search prompt is currently open.
func (m Model) IsSearching() bool { return m.searching }

// visibleIdx returns the indices of Items that are currently shown,
// applying both the HidePaused and fuzzy-query filters.
func (m Model) visibleIdx() []int {
	var idx []int
	for i, item := range m.Items {
		if m.HidePaused && item.Paused {
			continue
		}
		if m.query != "" && !fuzzyMatch(m.query, item.Entry.Name) {
			continue
		}
		idx = append(idx, i)
	}
	return idx
}

func (m Model) pageRows() int {
	if m.height <= 0 {
		return len(m.Items)
	}
	rows := m.height - 10
	if m.query != "" || m.searching {
		rows -= 2
	}
	if rows < 1 {
		return 1
	}
	return rows
}

func (m *Model) ensureCursorVisible() {
	visible := len(m.visibleIdx())
	if visible == 0 {
		m.cursor = 0
		m.viewportStart = 0
		return
	}
	if m.cursor >= visible {
		m.cursor = visible - 1
	}
	rows := m.pageRows()
	if m.cursor < m.viewportStart {
		m.viewportStart = m.cursor
	} else if m.cursor >= m.viewportStart+rows {
		m.viewportStart = m.cursor - rows + 1
	}
	maxStart := visible - rows
	if maxStart < 0 {
		maxStart = 0
	}
	if m.viewportStart > maxStart {
		m.viewportStart = maxStart
	}
}

// fuzzyMatch reports whether all runes in query appear as a subsequence
// (in order, case-insensitive) in target. This is the same algorithm used
// by fzf and vim-style fuzzy finders — no external dependency required.
func fuzzyMatch(query, target string) bool {
	qr := []rune(strings.ToLower(query))
	qi := 0
	for _, r := range strings.ToLower(target) {
		if qi < len(qr) && qr[qi] == r {
			qi++
		}
	}
	return qi == len(qr)
}

func New(items []PipelineItem) Model {
	return Model{Items: items}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Search mode: capture printable chars; allow navigation; esc/enter exit.
		if m.searching {
			switch msg.String() {
			case "esc":
				m.searching = false
				m.query = ""
				m.cursor = 0
			case "enter":
				m.searching = false
			case "backspace", "ctrl+h":
				if len(m.query) > 0 {
					runes := []rune(m.query)
					m.query = string(runes[:len(runes)-1])
					m.cursor = 0
				}
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				vi := m.visibleIdx()
				if m.cursor < len(vi)-1 {
					m.cursor++
				}
			default:
				if len(msg.Runes) > 0 {
					m.query += string(msg.Runes)
					m.cursor = 0
				}
			}
			vi := m.visibleIdx()
			if len(vi) > 0 && m.cursor >= len(vi) {
				m.cursor = len(vi) - 1
			}
			m.ensureCursorVisible()
			return m, nil
		}

		vi := m.visibleIdx()
		switch msg.String() {
		case "/":
			m.searching = true
		case "esc":
			if m.query != "" {
				m.query = ""
				m.cursor = 0
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(vi)-1 {
				m.cursor++
			}
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			if len(vi) > 0 {
				m.cursor = len(vi) - 1
			}
		case "pgup", "ctrl+u":
			m.cursor -= m.pageRows()
			if m.cursor < 0 {
				m.cursor = 0
			}
		case "pgdown", "ctrl+d":
			m.cursor += m.pageRows()
			if len(vi) > 0 && m.cursor >= len(vi) {
				m.cursor = len(vi) - 1
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
		case "c":
			if len(vi) > 0 {
				entry := m.Items[vi[m.cursor]].Entry
				return m, func() tea.Msg { return OpenCopyMsg{Entry: entry} }
			}
		case "d":
			if len(vi) > 0 {
				entry := m.Items[vi[m.cursor]].Entry
				return m, func() tea.Msg { return OpenDeleteMsg{Entry: entry} }
			}
		case "A":
			return m, func() tea.Msg { return OpenAutomationsMsg{} }
		case " ":
			if len(vi) > 0 {
				idx := vi[m.cursor]
				return m, func() tea.Msg { return TogglePauseMsg{Index: idx} }
			}
		case "f":
			if len(vi) > 0 {
				idx := vi[m.cursor]
				return m, func() tea.Msg { return FocusMsg{Index: idx} }
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
				m.ensureCursorVisible()
				from, to := realCur, realPrev
				return m, func() tea.Msg { return MoveItemMsg{From: from, To: to} }
			}
		case "shift+down":
			if m.cursor < len(vi)-1 {
				realCur, realNext := vi[m.cursor], vi[m.cursor+1]
				m.Items[realCur], m.Items[realNext] = m.Items[realNext], m.Items[realCur]
				m.cursor++
				m.ensureCursorVisible()
				from, to := realCur, realNext
				return m, func() tea.Msg { return MoveItemMsg{From: from, To: to} }
			}
		}
		m.ensureCursorVisible()

	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		m.ensureCursorVisible()

	case FetchedMsg:
		if msg.Index >= 0 && msg.Index < len(m.Items) {
			m.Items[msg.Index] = msg.Item
			// A pipeline may have become paused/unpaused; re-clamp cursor.
			m.ensureCursorVisible()
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
		var sb strings.Builder
		if m.query != "" {
			fmt.Fprintf(&sb, "\n  No pipelines match %s\n", styles.FormInput.Render(m.query))
		} else {
			fmt.Fprintf(&sb, "\n  All pipelines are paused. Press %s to show them.\n",
				styles.FormCursor.Render("h"))
		}
		sb.WriteString(m.searchBarView())
		return sb.String()
	}
	m.ensureCursorVisible()
	start := m.viewportStart
	end := start + m.pageRows()
	if end > len(vi) {
		end = len(vi)
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
	if start > 0 {
		fmt.Fprintf(&sb, "  %s\n", styles.Footer.Render(fmt.Sprintf("↑ %d more", start)))
	}
	for visPos := start; visPos < end; visPos++ {
		realIdx := vi[visPos]
		sb.WriteString("  ")
		sb.WriteString(m.Items[realIdx].Render(visPos == m.cursor, nameWidth))
		sb.WriteByte('\n')
	}
	if end < len(vi) {
		fmt.Fprintf(&sb, "  %s\n", styles.Footer.Render(fmt.Sprintf("↓ %d more", len(vi)-end)))
	}
	sb.WriteString(m.searchBarView())
	return sb.String()
}

// searchBarView returns the search prompt line (or filter indicator) to append
// at the bottom of the list content.
func (m Model) searchBarView() string {
	if m.searching {
		return "\n  " + styles.FormCursor.Render("/") + " " + m.query + styles.FormCursor.Render("▋") + "\n"
	}
	if m.query != "" {
		return "\n  " + styles.Footer.Render("filter: "+m.query+"  (esc to clear)") + "\n"
	}
	return ""
}

// Selected returns the currently highlighted item, or false if the visible list is empty.
func (m Model) Selected() (PipelineItem, bool) {
	vi := m.visibleIdx()
	if len(vi) == 0 {
		return PipelineItem{}, false
	}
	if m.cursor < 0 || m.cursor >= len(vi) {
		return PipelineItem{}, false
	}
	return m.Items[vi[m.cursor]], true
}

// Position returns the one-based cursor position and total visible pipelines.
func (m Model) Position() (int, int) {
	total := len(m.visibleIdx())
	if total == 0 || m.cursor < 0 || m.cursor >= total {
		return 0, total
	}
	return m.cursor + 1, total
}

// SetFocused pauses every pipeline except selectedIndex and keeps the focused
// pipeline selected, even when paused pipelines are hidden.
func (m Model) SetFocused(selectedIndex int) Model {
	for i := range m.Items {
		paused := i != selectedIndex
		m.Items[i].Paused = paused
		m.Items[i].Entry.Paused = paused
	}
	m.HidePaused = true
	visible := m.visibleIdx()
	for position, index := range visible {
		if index == selectedIndex {
			m.cursor = position
			break
		}
	}
	m.ensureCursorVisible()
	return m
}
