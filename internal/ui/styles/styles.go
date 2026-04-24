package styles

import "github.com/charmbracelet/lipgloss"

var (
	// Status dot colors
	DotSuccess   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	DotFailed    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	DotRunning   = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	DotApproval  = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // purple
	DotCancelled = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // grey
	DotOther     = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // grey

	// Provider badges
	BadgeGH  = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("237")).Padding(0, 1)
	BadgeAWS = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Background(lipgloss.Color("237")).Padding(0, 1)

	// Status text
	StatusRunning   = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	StatusSuccess   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	StatusFailed    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	StatusApproval  = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	StatusCancelled = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	StatusPending   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	StatusIdle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	StatusUnknown   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	// List row
	SelectedRow    = lipgloss.NewStyle().Background(lipgloss.Color("237")).Bold(true)
	ApprovalRow    = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // purple, matches DotApproval
	UnselectedRow  = lipgloss.NewStyle()
	PausedRow      = lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // dark grey
	MarkerSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

	// Name column
	PipelineName = lipgloss.NewStyle()

	// Branch/commit reference column
	BranchRef = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan

	// Header / footer
	Header = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).BorderBottom(true).BorderStyle(lipgloss.NormalBorder())
	Footer = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	// Detail view
	DetailTitle  = lipgloss.NewStyle().Bold(true).MarginBottom(1)
	DetailLabel  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Width(12)
	DetailValue  = lipgloss.NewStyle()
	LogContainer = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).MarginTop(1)

	// Credential status
	CredOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	CredMissing = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red

	// Add form
	FormTitle  = lipgloss.NewStyle().Bold(true).MarginBottom(1)
	FormLabel  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Width(16)
	FormInput  = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	FormCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	FormError  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

	// Automation hints shown in the pipeline list row.
	// AutomationWatch = pipeline is watched by a rule (will trigger another).
	// AutomationTarget = pipeline is a trigger target of a rule.
	// Dim variants are used when all matching rules are disabled/exhausted.
	AutomationWatch     = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	AutomationTarget    = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan
	AutomationWatchDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // grey
	AutomationTargetDim = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // grey
)

const (
	DotFull  = "●"
	DotEmpty = "○"
)
