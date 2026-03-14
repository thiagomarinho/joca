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
	URL     string // pipeline-level page (e.g. /actions or AWS console)
	Current provider.Run
	History []provider.Run // most recent first, up to historyDots
	Err     error          // set if last fetch failed
	Paused  bool           // true when the user has disabled auto-refresh
}

// Render returns the single-line string for this row.
// highlighted controls whether the selected-row style is applied.
// nameWidth is the visible character width to use for the name column.
func (p PipelineItem) Render(highlighted bool, nameWidth int) string {
	marker := " "
	if p.Paused {
		marker = styles.DotOther.Render("⏸")
	}
	name := styles.PipelineName.Width(nameWidth).Render(truncate(p.Entry.Name, nameWidth))
	badge := renderBadge(p.Entry.Provider)
	branchRef := renderBranchRef(p.Current.Branch, p.Current.Commit)
	status := renderStatus(p.Current.Status, p.Current.Stage, p.Err)
	dots := renderDots(p.History)

	// Pad status to a fixed visible width so the dots column stays aligned
	// regardless of ANSI escape codes in the status string.
	const statusWidth = 22
	if pad := statusWidth - lipgloss.Width(status); pad > 0 {
		status += strings.Repeat(" ", pad)
	}

	line := fmt.Sprintf("%s %s %s  %s  %s %s", marker, name, badge, branchRef, status, dots)
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

func renderStatus(s provider.Status, stage string, err error) string {
	if err != nil {
		if provider.IsCredentialError(err) {
			return styles.StatusFailed.Render("✗ no credentials")
		}
		return styles.StatusFailed.Render("✗ error")
	}
	switch s {
	case provider.StatusRunning:
		if stage != "" {
			return styles.StatusRunning.Render("● " + truncate(stage, 20))
		}
		return styles.StatusRunning.Render("● running")
	case provider.StatusSuccess:
		return styles.StatusSuccess.Render("✓ idle")
	case provider.StatusFailed:
		return styles.StatusFailed.Render("✗ failed")
	case provider.StatusApproval:
		if stage != "" {
			return styles.StatusApproval.Render("⏸ → " + truncate(stage, 17))
		}
		return styles.StatusApproval.Render("⏸ awaiting approval")
	case provider.StatusCancelled:
		return styles.StatusCancelled.Render("⊘ cancelled")
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
		case provider.StatusCancelled:
			sb.WriteString(styles.DotCancelled.Render(styles.DotFull))
		default:
			sb.WriteString(styles.DotOther.Render(styles.DotEmpty))
		}
	}
	return sb.String()
}

const branchRefWidth = 16

// renderBranchRef returns a fixed-width (branchRefWidth) string showing
// "branch@sha", "branch", "sha", or spaces when neither is available.
func renderBranchRef(branch, commit string) string {
	var ref string
	switch {
	case branch != "" && commit != "":
		ref = branch + "@" + commit
	case branch != "":
		ref = branch
	case commit != "":
		ref = commit
	}
	if len(ref) > branchRefWidth {
		ref = ref[:branchRefWidth-1] + "…"
	}
	if pad := branchRefWidth - len(ref); pad > 0 {
		ref += strings.Repeat(" ", pad)
	}
	return styles.BranchRef.Render(ref)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
