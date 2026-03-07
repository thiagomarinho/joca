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

// OpenBrowserMsg asks the OS to open a URL.
type OpenBrowserMsg struct{ URL string }

// OpenWatchMsg asks the root model to push the watch dashboard.
type OpenWatchMsg struct{ Items []PipelineItem }

// Model is the pipeline list view.
type Model struct {
	Items    []PipelineItem
	selected map[int]bool
	cursor   int
	height   int
	width    int
}

func New(items []PipelineItem) Model {
	return Model{Items: items, selected: make(map[int]bool)}
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
				return m, func() tea.Msg { return OpenBrowserMsg{URL: m.Items[m.cursor].Current.URL} }
			}
		case "a":
			return m, func() tea.Msg { return OpenAddFormMsg{} }
		case " ":
			if len(m.Items) > 0 {
				m.selected[m.cursor] = !m.selected[m.cursor]
			}
		case "w":
			if items := m.selectedItems(); len(items) > 0 {
				return m, func() tea.Msg { return OpenWatchMsg{Items: items} }
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

	var sb strings.Builder
	for i, item := range m.Items {
		sb.WriteString("  ")
		sb.WriteString(item.Render(i == m.cursor, m.selected[i]))
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

func (m Model) selectedItems() []PipelineItem {
	var out []PipelineItem
	for i, item := range m.Items {
		if m.selected[i] {
			out = append(out, item)
		}
	}
	return out
}
