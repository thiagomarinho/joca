package telemetry

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/telemetry"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

// ClearMsg asks the root model to clear the recorder.
type ClearMsg struct{}

// ToggleMsg asks the root model to toggle tracking.
type ToggleMsg struct{}

// backMsg signals the root model to pop this view.
type backMsg struct{}

// Model is the telemetry page view.
type Model struct {
	recorder *telemetry.Recorder
}

// New creates a telemetry view backed by the given recorder.
func New(recorder *telemetry.Recorder) Model {
	return Model{recorder: recorder}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			return m, func() tea.Msg { return backMsg{} }
		case "c":
			return m, func() tea.Msg { return ClearMsg{} }
		case "t":
			return m, func() tea.Msg { return ToggleMsg{} }
		}
	}
	return m, nil
}

func (m Model) View() string {
	s := m.recorder.Summary()
	var sb strings.Builder

	// Title line
	trackingLabel := "off"
	if s.Enabled {
		trackingLabel = "on"
	}
	sb.WriteString("\n  ")
	sb.WriteString(styles.DetailTitle.Render(fmt.Sprintf("API Call Telemetry  [tracking: %s]", trackingLabel)))
	sb.WriteString("\n\n")

	if s.TotalCalls == 0 {
		sb.WriteString("  No calls recorded yet.\n")
	} else {
		sinceAgo := ""
		if !s.Since.IsZero() {
			sinceAgo = fmt.Sprintf("since %s ago", humanDur(time.Since(s.Since)))
		}

		tw := tabwriter.NewWriter(&sb, 0, 0, 3, ' ', 0)

		// Summary totals
		_, _ = fmt.Fprintf(tw, "  Total\t%d calls\t%d errors\tavg %s\t%s\n",
			s.TotalCalls, s.TotalErrors, humanDur(s.AvgDuration()), sinceAgo)
		_, _ = fmt.Fprintln(tw)

		// By provider
		_, _ = fmt.Fprintf(tw, "  %s\n", styles.DetailLabel.Render("By provider"))
		providers := sortedKeys(s.ByProvider)
		for _, prov := range providers {
			ps := s.ByProvider[prov]
			_, _ = fmt.Fprintf(tw, "  %s\t%d calls\t%d errors\tavg %s\n",
				prov, ps.Calls, ps.Errors, humanDur(ps.TotalDur/time.Duration(ps.Calls)))
		}
		_, _ = fmt.Fprintln(tw)

		// By method
		_, _ = fmt.Fprintf(tw, "  %s\n", styles.DetailLabel.Render("By method"))
		methods := sortedKeys(s.ByMethod)
		for _, method := range methods {
			ms := s.ByMethod[method]
			_, _ = fmt.Fprintf(tw, "  %s\t%d calls\n", method, ms.Calls)
		}
		_, _ = fmt.Fprintln(tw)

		// By pipeline
		_, _ = fmt.Fprintf(tw, "  %s\n", styles.DetailLabel.Render("By pipeline"))
		pipelines := sortedKeys(s.ByPipeline)
		for _, name := range pipelines {
			pl := s.ByPipeline[name]
			provBadge := strings.ToUpper(pl.Provider)
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%d calls\t%d errors\n",
				name, provBadge, pl.Calls, pl.Errors)
		}

		_ = tw.Flush()
	}

	sb.WriteString("\n  ")
	sb.WriteString(styles.Footer.Render("esc: back  c: clear  t: toggle tracking"))
	return sb.String()
}

func humanDur(d time.Duration) string {
	switch {
	case d == 0:
		return "0ms"
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
