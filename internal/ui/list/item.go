package list

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/provider"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

const historyDots = 6

// PipelineItem holds the runtime state of a single pipeline row.
type PipelineItem struct {
	Entry   config.PipelineEntry
	Current provider.Run
	History []provider.Run // most recent first, up to historyDots
	Err     error          // set if last fetch failed
	Paused  bool           // true when the user has disabled auto-refresh
}

// Render returns the single-line string for this row.
// highlighted controls whether the selected-row style is applied.
func (p PipelineItem) Render(highlighted bool) string {
	marker := " "
	if p.Paused {
		marker = styles.DotOther.Render("⏸")
	}
	name := styles.PipelineName.Render(truncate(p.Entry.Name, 20))
	badge := renderBadge(p.Entry.Provider)
	status := renderStatus(p.Current.Status, p.Err)
	dots := renderDots(p.History)

	// Pad status to a fixed visible width so the dots column stays aligned
	// regardless of ANSI escape codes in the status string.
	const statusWidth = 22
	if pad := statusWidth - lipgloss.Width(status); pad > 0 {
		status += strings.Repeat(" ", pad)
	}

	line := fmt.Sprintf("%s %s %s  %s %s", marker, name, badge, status, dots)
	switch {
	case highlighted && p.Paused:
		return styles.SelectedRow.Render(styles.PausedRow.Render(line))
	case highlighted:
		return styles.SelectedRow.Render(line)
	case p.Paused:
		return styles.PausedRow.Render(line)
	default:
		return styles.UnselectedRow.Render(line)
	}
}

func renderBadge(k config.ProviderKind) string {
	// All badges must render to the same visible width so columns stay aligned.
	// " AWS " (Padding 0,1) = 5 visible chars; " GH " = 4, so pad GH by 1.
	const badgeWidth = 5
	var rendered string
	switch k {
	case config.ProviderGitHub:
		rendered = styles.BadgeGH.Render("GH")
	case config.ProviderAWS:
		rendered = styles.BadgeAWS.Render("AWS")
	default:
		return strings.Repeat(" ", badgeWidth)
	}
	if pad := badgeWidth - lipgloss.Width(rendered); pad > 0 {
		rendered += strings.Repeat(" ", pad)
	}
	return rendered
}

func renderStatus(s provider.Status, err error) string {
	if err != nil {
		if provider.IsCredentialError(err) {
			return styles.StatusFailed.Render("✗ no credentials")
		}
		return styles.StatusFailed.Render("✗ error")
	}
	switch s {
	case provider.StatusRunning:
		return styles.StatusRunning.Render("● running")
	case provider.StatusSuccess:
		return styles.StatusSuccess.Render("✓ idle")
	case provider.StatusFailed:
		return styles.StatusFailed.Render("✗ failed")
	case provider.StatusApproval:
		return styles.StatusApproval.Render("⏸ awaiting approval")
	case provider.StatusPending:
		return styles.StatusPending.Render("… pending")
	case provider.StatusIdle:
		return styles.StatusIdle.Render("  idle")
	}
	return styles.StatusUnknown.Render("? unknown")
}

func renderDots(history []provider.Run) string {
	var sb strings.Builder
	for i := 0; i < historyDots; i++ {
		if i > 0 {
			sb.WriteByte(' ')
		}
		if i >= len(history) {
			sb.WriteString(styles.DotOther.Render(styles.DotEmpty))
			continue
		}
		switch history[i].Status {
		case provider.StatusSuccess:
			sb.WriteString(styles.DotSuccess.Render(styles.DotFull))
		case provider.StatusFailed:
			sb.WriteString(styles.DotFailed.Render(styles.DotFull))
		case provider.StatusRunning:
			sb.WriteString(styles.DotRunning.Render(styles.DotFull))
		case provider.StatusApproval:
			sb.WriteString(styles.DotApproval.Render(styles.DotFull))
		default:
			sb.WriteString(styles.DotOther.Render(styles.DotEmpty))
		}
	}
	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
