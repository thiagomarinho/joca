package settings

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

// SavedMsg contains validated application settings.
type SavedMsg struct {
	RefreshInterval   string
	DefaultAWSProfile string
	DefaultAWSRegion  string
}

// CancelledMsg is emitted when the user leaves without saving.
type CancelledMsg struct{}

type field int

const (
	fieldRefreshInterval field = iota
	fieldDefaultAWSProfile
	fieldDefaultAWSRegion
	fieldCount
)

// Model edits application-wide configuration defaults.
type Model struct {
	cursor field
	values [fieldCount]string
	err    string
}

// New creates a settings form prefilled from cfg.
func New(cfg config.AppConfig) Model {
	m := Model{}
	m.values[fieldRefreshInterval] = cfg.RefreshInterval
	m.values[fieldDefaultAWSProfile] = cfg.DefaultAWSProfile
	m.values[fieldDefaultAWSRegion] = cfg.DefaultAWSRegion
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "q":
		return m, func() tea.Msg { return CancelledMsg{} }
	case "tab", "down":
		m.cursor = (m.cursor + 1) % fieldCount
		m.err = ""
	case "shift+tab", "up":
		m.cursor = (m.cursor - 1 + fieldCount) % fieldCount
		m.err = ""
	case "enter", "ctrl+s":
		return m.save()
	case "backspace", "ctrl+h":
		runes := []rune(m.values[m.cursor])
		if len(runes) > 0 {
			m.values[m.cursor] = string(runes[:len(runes)-1])
		}
	case "ctrl+u":
		m.values[m.cursor] = ""
	default:
		if key.Type == tea.KeyRunes {
			m.values[m.cursor] += string(key.Runes)
		}
	}
	return m, nil
}

func (m Model) save() (tea.Model, tea.Cmd) {
	interval := strings.TrimSpace(m.values[fieldRefreshInterval])
	duration, err := time.ParseDuration(interval)
	if err != nil {
		m.err = "Refresh interval must be a duration such as 30s or 5m"
		return m, nil
	}
	if duration < config.MinRefreshInterval {
		m.err = fmt.Sprintf("Refresh interval must be at least %s", config.MinRefreshInterval)
		return m, nil
	}
	msg := SavedMsg{
		RefreshInterval:   interval,
		DefaultAWSProfile: strings.TrimSpace(m.values[fieldDefaultAWSProfile]),
		DefaultAWSRegion:  strings.TrimSpace(m.values[fieldDefaultAWSRegion]),
	}
	return m, func() tea.Msg { return msg }
}

func (m Model) View() string {
	var sb strings.Builder
	sb.WriteString("\n  ")
	sb.WriteString(styles.FormTitle.Render("Configuration"))
	sb.WriteString("\n\n")

	labels := [fieldCount]string{
		"Refresh interval",
		"Default AWS profile/role",
		"Default AWS region",
	}
	hints := [fieldCount]string{
		fmt.Sprintf("Minimum %s; examples: 30s, 2m, 1h", config.MinRefreshInterval),
		"Optional; prefills new AWS pipelines",
		"Optional; prefills new AWS pipelines",
	}
	for i := field(0); i < fieldCount; i++ {
		marker := " "
		value := m.values[i]
		if i == m.cursor {
			marker = styles.FormCursor.Render(">")
			value += styles.FormCursor.Render("_")
		}
		fmt.Fprintf(&sb, "  %s %s  %s\n", marker, styles.FormLabel.Render(labels[i]+":"), styles.FormInput.Render(value))
		fmt.Fprintf(&sb, "      %s\n\n", styles.Footer.Render(hints[i]))
	}
	if m.err != "" {
		fmt.Fprintf(&sb, "  %s\n\n", styles.FormError.Render("✗ "+m.err))
	}
	sb.WriteString("  " + styles.Footer.Render("tab/↑↓: switch field  ctrl+u: clear  enter: save  esc: cancel"))
	return sb.String()
}
