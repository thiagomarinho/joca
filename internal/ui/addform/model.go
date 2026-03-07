package addform

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

// SavedMsg is emitted when the user successfully saves a new pipeline.
type SavedMsg struct{ Entry config.PipelineEntry }

// CancelledMsg is emitted when the user cancels the form.
type CancelledMsg struct{}

type field int

const (
	fieldProvider field = iota
	fieldTarget
	fieldName
	fieldAWSProfile
	fieldAWSRegion
	fieldCount
)

// Model is the inline add-pipeline form.
type Model struct {
	configFile string
	cursor     field
	values     [fieldCount]string
	err        string
}

func New(configFile string) Model {
	m := Model{configFile: configFile}
	m.values[fieldProvider] = "github" // default
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return CancelledMsg{} }
		case "tab", "down":
			m.cursor = (m.cursor + 1) % fieldCount
			m.err = ""
		case "shift+tab", "up":
			m.cursor = (m.cursor - 1 + fieldCount) % fieldCount
			m.err = ""
		case "enter":
			return m.trySave()
		case "backspace":
			v := m.values[m.cursor]
			if len(v) > 0 {
				m.values[m.cursor] = v[:len(v)-1]
			}
		default:
			if len(msg.String()) == 1 {
				m.values[m.cursor] += msg.String()
			}
		}
	}
	return m, nil
}

func (m Model) trySave() (tea.Model, tea.Cmd) {
	providerRaw := strings.ToLower(strings.TrimSpace(m.values[fieldProvider]))
	target := strings.TrimSpace(m.values[fieldTarget])
	name := strings.TrimSpace(m.values[fieldName])

	var entry config.PipelineEntry

	switch providerRaw {
	case "github", "gh":
		parts := strings.SplitN(target, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			m.err = "GitHub target must be owner/repo"
			return m, nil
		}
		if name == "" {
			name = parts[1]
		}
		entry = config.PipelineEntry{
			Name:     name,
			Provider: config.ProviderGitHub,
			Owner:    parts[0],
			Repo:     parts[1],
		}
	case "aws", "codepipeline":
		if target == "" {
			m.err = "Pipeline name is required"
			return m, nil
		}
		if name == "" {
			name = target
		}
		entry = config.PipelineEntry{
			Name:         name,
			Provider:     config.ProviderAWS,
			PipelineName: target,
			AWSProfile:   strings.TrimSpace(m.values[fieldAWSProfile]),
			AWSRegion:    strings.TrimSpace(m.values[fieldAWSRegion]),
		}
	default:
		m.err = fmt.Sprintf("Unknown provider %q (use github or aws)", providerRaw)
		return m, nil
	}

	if err := config.AddPipeline(m.configFile, entry); err != nil {
		m.err = err.Error()
		return m, nil
	}

	saved := entry
	return m, func() tea.Msg { return SavedMsg{Entry: saved} }
}

func (m Model) View() string {
	var sb strings.Builder
	sb.WriteString("\n  ")
	sb.WriteString(styles.FormTitle.Render("Add pipeline"))
	sb.WriteString("\n\n")

	labels := [fieldCount]string{
		"Provider",
		"Target",
		"Display name",
		"AWS profile",
		"AWS region",
	}
	hints := [fieldCount]string{
		"github or aws",
		"owner/repo  or  pipeline-name",
		"(optional, defaults to target)",
		"(optional)",
		"(optional, e.g. us-east-1)",
	}

	for i := field(0); i < fieldCount; i++ {
		cursor := " "
		if i == m.cursor {
			cursor = styles.FormCursor.Render(">")
		}
		val := m.values[i]
		if i == m.cursor {
			val += styles.FormCursor.Render("_")
		}
		fmt.Fprintf(&sb, "  %s %s  %s\n",
			cursor,
			styles.FormLabel.Render(labels[i]+":"),
			styles.FormInput.Render(val),
		)
		fmt.Fprintf(&sb, "       %s%s\n\n",
			strings.Repeat(" ", 16),
			styles.Footer.Render(hints[i]),
		)
	}

	if m.err != "" {
		sb.WriteString("  ")
		sb.WriteString(styles.FormError.Render("✗ " + m.err))
		sb.WriteString("\n\n")
	}

	sb.WriteString("  ")
	sb.WriteString(styles.Footer.Render("tab/↓: next  shift+tab/↑: prev  enter: save  esc: cancel"))
	return sb.String()
}

// Footer re-export for root model to use.
var Footer = styles.Footer
