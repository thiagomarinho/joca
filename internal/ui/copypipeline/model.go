package copypipeline

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

// SavedMsg is emitted after the copied pipeline has been persisted.
type SavedMsg struct{ Entry config.PipelineEntry }

// CancelledMsg is emitted when the user cancels the form.
type CancelledMsg struct{}

type formField struct {
	label string
	hint  string
	value string
}

// Model is a prefilled form for copying a pipeline.
type Model struct {
	configFile string
	source     config.PipelineEntry
	fields     []formField
	cursor     int
	err        string
}

// New creates a copy form prefilled from source.
func New(configFile string, source config.PipelineEntry) Model {
	fields := []formField{{label: "Display name", value: source.Name + " copy"}}
	switch source.Provider {
	case config.ProviderGitHub:
		fields = append(fields,
			formField{label: "Owner", value: source.Owner},
			formField{label: "Repository", value: source.Repo},
			formField{label: "Workflow", hint: "optional; filename or workflow ID", value: source.Workflow},
		)
	case config.ProviderAWS:
		fields = append(fields,
			formField{label: "Pipeline", value: source.PipelineName},
			formField{label: "Profile", hint: "optional", value: source.AWSProfile},
			formField{label: "Region", hint: "optional", value: source.AWSRegion},
		)
	}
	return Model{configFile: configFile, source: source, fields: fields}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "esc":
		return m, func() tea.Msg { return CancelledMsg{} }
	case "tab", "down":
		m.cursor = (m.cursor + 1) % len(m.fields)
		m.err = ""
	case "shift+tab", "up":
		m.cursor = (m.cursor - 1 + len(m.fields)) % len(m.fields)
		m.err = ""
	case "enter":
		return m.save()
	case "backspace", "ctrl+h":
		runes := []rune(m.fields[m.cursor].value)
		if len(runes) > 0 {
			m.fields[m.cursor].value = string(runes[:len(runes)-1])
		}
	default:
		if key.Type == tea.KeyRunes {
			m.fields[m.cursor].value += string(key.Runes)
		}
	}
	return m, nil
}

func (m Model) save() (tea.Model, tea.Cmd) {
	entry := m.source
	entry.Name = strings.TrimSpace(m.fields[0].value)
	entry.Paused = false
	if entry.Name == "" {
		m.err = "Display name is required"
		return m, nil
	}

	switch entry.Provider {
	case config.ProviderGitHub:
		entry.Owner = strings.TrimSpace(m.fields[1].value)
		entry.Repo = strings.TrimSpace(m.fields[2].value)
		entry.Workflow = strings.TrimSpace(m.fields[3].value)
		if entry.Owner == "" || entry.Repo == "" {
			m.err = "Owner and repository are required"
			return m, nil
		}
	case config.ProviderAWS:
		entry.PipelineName = strings.TrimSpace(m.fields[1].value)
		entry.AWSProfile = strings.TrimSpace(m.fields[2].value)
		entry.AWSRegion = strings.TrimSpace(m.fields[3].value)
		if entry.PipelineName == "" {
			m.err = "Pipeline name is required"
			return m, nil
		}
	default:
		m.err = fmt.Sprintf("Unsupported provider %q", entry.Provider)
		return m, nil
	}

	if err := config.AddPipeline(m.configFile, entry); err != nil {
		m.err = err.Error()
		return m, nil
	}
	return m, func() tea.Msg { return SavedMsg{Entry: entry} }
}

func (m Model) View() string {
	var sb strings.Builder
	providerName := strings.ToUpper(string(m.source.Provider))
	fmt.Fprintf(&sb, "\n  %s\n\n", styles.FormTitle.Render("Copy "+providerName+" pipeline"))

	for i, field := range m.fields {
		cursor := " "
		value := field.value
		if i == m.cursor {
			cursor = styles.FormCursor.Render(">")
			value += styles.FormCursor.Render("_")
		}
		fmt.Fprintf(&sb, "  %s %s  %s\n", cursor, styles.FormLabel.Render(field.label+":"), styles.FormInput.Render(value))
		if field.hint != "" {
			fmt.Fprintf(&sb, "      %s\n", styles.Footer.Render(field.hint))
		}
		sb.WriteByte('\n')
	}

	if m.err != "" {
		fmt.Fprintf(&sb, "  %s\n\n", styles.FormError.Render("✗ "+m.err))
	}
	sb.WriteString("  " + styles.Footer.Render("tab/↓: next  shift+tab/↑: previous  enter: copy  esc: cancel"))
	return sb.String()
}
