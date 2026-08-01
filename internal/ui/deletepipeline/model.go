package deletepipeline

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

// DeletedMsg is emitted after the pipeline and its dependent rules are removed.
type DeletedMsg struct {
	Name               string
	AutomationsDeleted int
}

// CancelledMsg is emitted when the user cancels deletion.
type CancelledMsg struct{}

// Model asks the user to confirm deletion of a pipeline.
type Model struct {
	configFile         string
	entry              config.PipelineEntry
	automationsDeleted int
	err                string
}

// New creates a delete confirmation for entry.
func New(configFile string, entry config.PipelineEntry, rules []config.AutomationRule) Model {
	count := 0
	for _, rule := range rules {
		if rule.WatchPipeline == entry.Name || rule.TriggerPipeline == entry.Name {
			count++
		}
	}
	return Model{configFile: configFile, entry: entry, automationsDeleted: count}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "n", "q":
		return m, func() tea.Msg { return CancelledMsg{} }
	case "enter", "y":
		removed, err := config.DeletePipeline(m.configFile, m.entry.Name)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		name := m.entry.Name
		return m, func() tea.Msg {
			return DeletedMsg{Name: name, AutomationsDeleted: removed}
		}
	}
	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder
	sb.WriteString("\n  ")
	sb.WriteString(styles.FormTitle.Render("Delete pipeline"))
	sb.WriteString("\n\n")
	fmt.Fprintf(&sb, "  Permanently delete %s?\n", styles.FormInput.Render(m.entry.Name))
	fmt.Fprintf(&sb, "  Provider: %s\n", strings.ToUpper(string(m.entry.Provider)))
	if m.automationsDeleted > 0 {
		fmt.Fprintf(&sb, "\n  %s\n", styles.FormError.Render(fmt.Sprintf(
			"Warning: %d associated automation rule(s) will also be deleted.",
			m.automationsDeleted,
		)))
	}
	if m.err != "" {
		fmt.Fprintf(&sb, "\n  %s\n", styles.FormError.Render("✗ "+m.err))
	}
	sb.WriteString("\n  " + styles.Footer.Render("y/enter: delete  n/esc: cancel"))
	return sb.String()
}
