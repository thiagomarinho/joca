package list

import (
	"fmt"
	"strings"

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
}

// Render returns the single-line string for this row.
// highlighted controls whether the selected-row style is applied.
// marked controls whether the ◉ watch-pin marker is shown.
func (p PipelineItem) Render(highlighted, marked bool) string {
	marker := " "
	if marked {
		marker = styles.MarkerSelected.Render("◉")
	}
	name := styles.PipelineName.Render(truncate(p.Entry.Name, 20))
	badge := renderBadge(p.Entry.Provider)
	status := renderStatus(p.Current.Status, p.Err)
	dots := renderDots(p.History)

	line := fmt.Sprintf("%s %s %s  %-22s %s", marker, name, badge, status, dots)
	if highlighted {
		return styles.SelectedRow.Render(line)
	}
	return styles.UnselectedRow.Render(line)
}

func renderBadge(k config.ProviderKind) string {
	switch k {
	case config.ProviderGitHub:
		return styles.BadgeGH.Render("GH")
	case config.ProviderAWS:
		return styles.BadgeAWS.Render("AWS")
	}
	return "   "
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
