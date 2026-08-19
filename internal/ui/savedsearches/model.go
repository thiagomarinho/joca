// Package savedsearches provides a picker for reusable CodeBuild log searches.
package savedsearches

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

// RunMsg asks the root model to run a saved search for the current pipeline.
type RunMsg struct {
	Pipeline string
	Search   config.SavedLogSearch
}

// DeleteMsg asks the root model to remove a saved search.
type DeleteMsg struct {
	Name     string
	Pipeline string
}

type backMsg struct{}

// Model lists searches applicable to one AWS pipeline.
type Model struct {
	pipeline string
	searches []config.SavedLogSearch
	cursor   int
}

// New includes global searches and searches saved specifically for pipeline.
func New(pipeline string, searches []config.SavedLogSearch) Model {
	filtered := make([]config.SavedLogSearch, 0, len(searches))
	for _, search := range searches {
		if search.Pipeline == "" || search.Pipeline == pipeline {
			filtered = append(filtered, search)
		}
	}
	return Model{pipeline: pipeline, searches: filtered}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "q":
		return m, func() tea.Msg { return backMsg{} }
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.searches)-1 {
			m.cursor++
		}
	case "enter", "r":
		if len(m.searches) > 0 {
			search := m.searches[m.cursor]
			return m, func() tea.Msg { return RunMsg{Pipeline: m.pipeline, Search: search} }
		}
	case "d":
		if len(m.searches) > 0 {
			search := m.searches[m.cursor]
			m.searches = append(m.searches[:m.cursor], m.searches[m.cursor+1:]...)
			if m.cursor >= len(m.searches) && m.cursor > 0 {
				m.cursor--
			}
			return m, func() tea.Msg { return DeleteMsg{Name: search.Name, Pipeline: search.Pipeline} }
		}
	}
	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n  %s\n\n", styles.DetailTitle.Render("Saved log searches — "+m.pipeline))
	if len(m.searches) == 0 {
		sb.WriteString("  No saved searches for this pipeline.\n")
	} else {
		for i, search := range m.searches {
			scope := "global"
			if search.Pipeline != "" {
				scope = "pipeline"
			}
			mode := "literal"
			if search.Regex {
				mode = "regex"
			}
			if search.CaseInsensitive {
				mode += ", insensitive"
			}
			line := fmt.Sprintf("  %-24s  %-10s  %q  (%s · %d runs · %d context)",
				search.Name, scope, search.Expression, mode, search.Executions, search.ContextLines)
			if i == m.cursor {
				line = styles.SelectedRow.Render(line)
			}
			sb.WriteString(line + "\n")
		}
	}
	sb.WriteString("\n  " + styles.Footer.Render("↑↓: select  enter/r: run  d: delete  esc: pipeline detail"))
	return sb.String()
}
