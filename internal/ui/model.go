package ui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/credstatus"
	"github.com/thiagomarinho/joca/internal/notify"
	"github.com/thiagomarinho/joca/internal/provider"
	awsprovider "github.com/thiagomarinho/joca/internal/provider/aws"
	ghprovider "github.com/thiagomarinho/joca/internal/provider/github"
	"github.com/thiagomarinho/joca/internal/telemetry"
	"github.com/thiagomarinho/joca/internal/ui/addautomation"
	"github.com/thiagomarinho/joca/internal/ui/addwizard"
	"github.com/thiagomarinho/joca/internal/ui/automations"
	"github.com/thiagomarinho/joca/internal/ui/copypipeline"
	"github.com/thiagomarinho/joca/internal/ui/detail"
	"github.com/thiagomarinho/joca/internal/ui/list"
	"github.com/thiagomarinho/joca/internal/ui/styles"
	uitelemetry "github.com/thiagomarinho/joca/internal/ui/telemetry"
)

type tickMsg time.Time
type uiTickMsg time.Time
type clearStatusMsg struct{}

type ssoLoginDoneMsg struct {
	profile string
	err     error
}

type credCheckDoneMsg struct {
	gh  credstatus.Status
	aws map[string]credstatus.Status
}

type fetchResultMsg struct {
	index int
	item  list.PipelineItem
}

type triggerDoneMsg struct {
	index int
	err   error
}

type triggerNewDoneMsg struct {
	index int
	err   error
}

type automationFiredMsg struct {
	ruleName       string
	targetPipeline string
	err            error
}

// RootModel is the top-level Bubbletea model managing a view stack.
type RootModel struct {
	appCfg          *config.AppConfig
	resolvedCfg     config.Config
	providers       []provider.Provider
	stack           []tea.Model // stack[len-1] is the active view
	refreshInterval time.Duration
	lastRefresh     time.Time
	width           int
	height          int
	statusMsg       string
	// prevStatus tracks the last known status per pipeline name so we can
	// detect changes and fire OS notifications. A nil map entry means the
	// pipeline has never been fetched yet (suppress notification on first load).
	prevStatus   map[string]provider.Status
	paused       map[int]bool // pipelines with auto-refresh disabled
	globalPaused bool         // session-only global pause; not persisted
	recorder     *telemetry.Recorder
	// Credential status, always checked on startup regardless of configured pipelines.
	ghCred   credstatus.Status
	awsCreds map[string]credstatus.Status // key: aws_profile (or "" for default chain)
}

// New builds the root model from the loaded app config.
func New(appCfg *config.AppConfig, resolvedCfg config.Config) *RootModel {
	interval := 30 * time.Second
	if d, err := time.ParseDuration(appCfg.RefreshInterval); err == nil {
		interval = d
	}

	providers := buildProviders(appCfg.Pipelines)
	items := makeItems(appCfg.Pipelines)
	for i, p := range providers {
		items[i].URL = p.URL()
	}

	paused := make(map[int]bool)
	for i, e := range appCfg.Pipelines {
		if e.Paused {
			paused[i] = true
		}
	}
	globalPaused := len(appCfg.Pipelines) > 0 && len(paused) == len(appCfg.Pipelines)

	// Pre-populate awsCreds keys with pending sentinels so the view has the
	// right set of profiles before the async check completes.
	awsCreds := make(map[string]credstatus.Status)
	awsCreds[""] = credstatus.Status{Pending: true}
	for _, e := range appCfg.Pipelines {
		if e.Provider == config.ProviderAWS {
			awsCreds[e.AWSProfile] = credstatus.Status{Pending: true}
		}
	}

	m := &RootModel{
		appCfg:          appCfg,
		resolvedCfg:     resolvedCfg,
		providers:       providers,
		refreshInterval: interval,
		stack:           []tea.Model{list.New(items)},
		prevStatus:      make(map[string]provider.Status),
		paused:          paused,
		globalPaused:    globalPaused,
		recorder:        telemetry.NewRecorder(),
		ghCred:          credstatus.Status{Pending: true},
		awsCreds:        awsCreds,
	}

	m.applyAutomationHints()
	return m
}

func (m *RootModel) Init() tea.Cmd {
	return tea.Batch(
		m.tickCmd(),
		m.uiTickCmd(),
		m.fetchAllCmd(),
		m.checkCredsCmd(),
	)
}

func (m *RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, m.forwardToActive(msg)

	case tea.KeyMsg:
		// ctrl+c always quits.
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// When the list's search prompt is open, forward every key to it so
		// that letters like q/p/r/t type into the query instead of firing shortcuts.
		if lm, ok := m.stack[0].(list.Model); ok && lm.IsSearching() {
			return m, m.forwardToActive(msg)
		}
		switch msg.String() {
		case "q":
			if len(m.stack) == 1 {
				return m, tea.Quit
			}
		case "p":
			if len(m.stack) == 1 {
				if m.globalPaused {
					allPaused := true
					for i := range m.providers {
						if !m.paused[i] {
							allPaused = false
							break
						}
					}
					if allPaused {
						m.statusMsg = "Resume individual pipelines first (space)"
						return m, m.clearStatusAfter(3 * time.Second)
					}
					m.globalPaused = false
					m.statusMsg = "Auto-refresh resumed"
					return m, tea.Batch(m.clearStatusAfter(2*time.Second), m.fetchAllCmd())
				}
				m.globalPaused = true
				m.statusMsg = "Auto-refresh paused  (p to resume)"
				return m, m.clearStatusAfter(2 * time.Second)
			}
		case "r":
			if len(m.stack) == 1 {
				m.statusMsg = "Refreshing…"
				return m, m.fetchAllCmdInner()
			}
		case "t":
			if len(m.stack) == 1 {
				m.recorder.SetEnabled(!m.recorder.IsEnabled())
				if m.recorder.IsEnabled() {
					m.statusMsg = "Tracking on"
				} else {
					m.statusMsg = "Tracking off"
				}
				return m, m.clearStatusAfter(2 * time.Second)
			}
		case "T":
			if len(m.stack) == 1 && m.recorder.IsEnabled() {
				m.stack = append(m.stack, uitelemetry.New(m.recorder))
				return m, nil
			}
		case "l":
			if len(m.stack) == 1 {
				if profile, ok := m.expiredSSOProfile(); ok {
					if profile == "" {
						m.statusMsg = "Opening AWS SSO login…"
					} else {
						m.statusMsg = fmt.Sprintf("Opening AWS SSO login for profile %q…", profile)
					}
					return m, awsSSOLoginCmd(profile)
				}
			}
		}
		return m, m.forwardToActive(msg)

	case credCheckDoneMsg:
		m.ghCred = msg.gh
		m.awsCreds = msg.aws
		return m, nil

	case ssoLoginDoneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("AWS SSO login failed: %v", msg.err)
			return m, m.clearStatusAfter(4 * time.Second)
		}
		m.awsCreds[msg.profile] = credstatus.Status{Pending: true}
		m.statusMsg = "AWS SSO credentials refreshed ✓"
		return m, tea.Batch(m.checkCredsCmd(), m.fetchAllCmd(), m.clearStatusAfter(3*time.Second))

	case clearStatusMsg:
		m.statusMsg = ""
		return m, nil

	case uiTickMsg:
		return m, m.uiTickCmd()

	case tickMsg:
		return m, tea.Batch(m.tickCmd(), m.fetchAllCmd())

	case fetchResultMsg:
		m.lastRefresh = time.Now()
		m.statusMsg = ""
		prev, seen := m.prevStatus[msg.item.Entry.Name]
		m.maybeNotify(msg.item)
		// Update the list view's item
		listView, ok := m.stack[0].(list.Model)
		if ok && msg.index >= 0 && msg.index < len(listView.Items) {
			msg.item.AutomationHints = listView.Items[msg.index].AutomationHints
			listView.Items[msg.index] = msg.item
			m.stack[0] = listView
		}
		// Evaluate automation rules on status transitions.
		newStatus := msg.item.Current.Status
		if msg.item.Err != nil {
			newStatus = provider.StatusUnknown
		}
		var automationCmd tea.Cmd
		if seen && prev != newStatus {
			automationCmd = m.evaluateAutomations(msg.item.Entry.Name, prev, newStatus)
		}
		return m, automationCmd

	case list.TogglePauseMsg:
		idx := msg.Index
		m.paused[idx] = !m.paused[idx]
		m.appCfg.Pipelines[idx].Paused = m.paused[idx]
		if listView, ok := m.stack[0].(list.Model); ok {
			item := listView.Items[idx]
			item.Paused = m.paused[idx]
			updated, _ := listView.Update(list.FetchedMsg{Index: idx, Item: item})
			m.stack[0] = updated
		}
		if m.globalPaused && !m.paused[idx] {
			m.globalPaused = false
			m.statusMsg = "Auto-refresh resumed"
			return m, tea.Batch(saveConfigCmd(m.resolvedCfg.ConfigFile, m.appCfg), m.clearStatusAfter(2*time.Second), m.fetchAllCmd())
		} else if !m.globalPaused && len(m.providers) > 0 {
			allPaused := true
			for i := range m.providers {
				if !m.paused[i] {
					allPaused = false
					break
				}
			}
			if allPaused {
				m.globalPaused = true
			}
		}
		return m, saveConfigCmd(m.resolvedCfg.ConfigFile, m.appCfg)

	case list.MoveItemMsg:
		from, to := msg.From, msg.To
		m.appCfg.Pipelines[from], m.appCfg.Pipelines[to] = m.appCfg.Pipelines[to], m.appCfg.Pipelines[from]
		m.providers[from], m.providers[to] = m.providers[to], m.providers[from]
		pFrom, pTo := m.paused[from], m.paused[to]
		delete(m.paused, from)
		delete(m.paused, to)
		if pFrom {
			m.paused[to] = true
		}
		if pTo {
			m.paused[from] = true
		}
		return m, saveConfigCmd(m.resolvedCfg.ConfigFile, m.appCfg)

	case list.TriggerMsg:
		m.statusMsg = "Re-running…"
		return m, m.triggerCmd(msg.Index)

	case triggerDoneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Re-run failed: %v", msg.err)
			return m, m.clearStatusAfter(3 * time.Second)
		}
		m.statusMsg = "Re-run triggered ✓"
		return m, tea.Batch(m.clearStatusAfter(3*time.Second), m.fetchOneCmd(msg.index))

	case list.TriggerNewMsg:
		m.statusMsg = "Starting new run…"
		return m, m.triggerNewCmd(msg.Index)

	case triggerNewDoneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("New run failed: %v", msg.err)
			return m, m.clearStatusAfter(3 * time.Second)
		}
		m.statusMsg = "New run started ✓"
		return m, tea.Batch(m.clearStatusAfter(3*time.Second), m.fetchOneCmd(msg.index))

	case automationFiredMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("automation %q failed: %v", msg.ruleName, msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("automation: triggered %q ✓", msg.targetPipeline)
		}
		return m, m.clearStatusAfter(4 * time.Second)

	case uitelemetry.ClearMsg:
		m.recorder.Clear()
		return m, nil

	case uitelemetry.ToggleMsg:
		m.recorder.SetEnabled(!m.recorder.IsEnabled())
		return m, nil

	// Messages from child views
	case list.OpenDetailMsg:
		m.stack = append(m.stack, detail.New(msg.Item))
		return m, nil

	case list.OpenAddFormMsg:
		m.stack = append(m.stack, addwizard.New(m.resolvedCfg.ConfigFile))
		return m, nil

	case list.OpenCopyMsg:
		m.stack = append(m.stack, copypipeline.New(m.resolvedCfg.ConfigFile, msg.Entry))
		return m, nil

	case list.OpenBrowserMsg:
		openBrowser(msg.URL)
		return m, nil

	case addwizard.SavedMsg:
		// Reload config, rebuild list
		m.stack = m.stack[:len(m.stack)-1] // pop wizard
		if len(msg.Entries) == 1 {
			m.statusMsg = fmt.Sprintf("Added %q", msg.Entries[0].Name)
		} else {
			m.statusMsg = fmt.Sprintf("Added %d pipelines", len(msg.Entries))
		}
		newCfg, err := config.Load(m.resolvedCfg.ConfigFile)
		if err == nil {
			m.appCfg = newCfg
			m.providers = buildProviders(newCfg.Pipelines)
			newItems := makeItems(newCfg.Pipelines)
			for i, p := range m.providers {
				newItems[i].URL = p.URL()
			}
			m.stack[0] = list.New(newItems)
		}
		m.applyAutomationHints()
		return m, m.fetchAllCmd()

	case addwizard.CancelledMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, nil

	case copypipeline.SavedMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		m.statusMsg = fmt.Sprintf("Copied %q", msg.Entry.Name)
		newCfg, err := config.Load(m.resolvedCfg.ConfigFile)
		if err == nil {
			m.appCfg = newCfg
			m.providers = buildProviders(newCfg.Pipelines)
			newItems := makeItems(newCfg.Pipelines)
			m.paused = make(map[int]bool)
			m.awsCreds = map[string]credstatus.Status{"": {Pending: true}}
			for i, entry := range newCfg.Pipelines {
				if entry.Paused {
					m.paused[i] = true
				}
				if entry.Provider == config.ProviderAWS {
					m.awsCreds[entry.AWSProfile] = credstatus.Status{Pending: true}
				}
				newItems[i].URL = m.providers[i].URL()
			}
			m.globalPaused = len(newCfg.Pipelines) > 0 && len(m.paused) == len(newCfg.Pipelines)
			m.stack[0] = list.New(newItems)
		}
		m.applyAutomationHints()
		return m, tea.Batch(m.checkCredsCmd(), m.fetchAllCmd(), m.clearStatusAfter(3*time.Second))

	case copypipeline.CancelledMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, nil

	// ── Automation views ─────────────────────────────────────────────────────
	case list.OpenAutomationsMsg:
		m.stack = append(m.stack, automations.New(m.appCfg.Automations))
		return m, nil

	case automations.OpenAddMsg:
		names := make([]string, len(m.appCfg.Pipelines))
		for i, p := range m.appCfg.Pipelines {
			names[i] = p.Name
		}
		m.stack = append(m.stack, addautomation.New(names, m.appCfg.Automations, m.appCfg.AllowChains()))
		return m, nil

	case addautomation.SavedMsg:
		// Pop the add wizard, add rules to config and persist.
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		m.appCfg.Automations = append(m.appCfg.Automations, msg.Rules...)
		// Update the automations list view if it's still on the stack.
		for i, v := range m.stack {
			if av, ok := v.(automations.Model); ok {
				av.Rules = append(av.Rules, msg.Rules...)
				m.stack[i] = av
				break
			}
		}
		if len(msg.Rules) == 1 {
			m.statusMsg = fmt.Sprintf("Automation rule %q added", msg.Rules[0].Name)
		} else {
			m.statusMsg = fmt.Sprintf("%d automation rules added", len(msg.Rules))
		}
		m.applyAutomationHints()
		return m, tea.Batch(saveConfigCmd(m.resolvedCfg.ConfigFile, m.appCfg), m.clearStatusAfter(3*time.Second))

	case addautomation.CancelledMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, nil

	case automations.DeletedMsg:
		// Persist deletion.
		keep := m.appCfg.Automations[:0]
		for _, r := range m.appCfg.Automations {
			if r.Name != msg.Name {
				keep = append(keep, r)
			}
		}
		m.appCfg.Automations = keep
		m.applyAutomationHints()
		return m, saveConfigCmd(m.resolvedCfg.ConfigFile, m.appCfg)

	case automations.ToggledMsg:
		// Persist enable/disable or reset changes.
		for i, v := range m.stack {
			if av, ok := v.(automations.Model); ok {
				// Sync changed rule back to appCfg.
				for _, r := range av.Rules {
					if r.Name == msg.Name {
						for j, ar := range m.appCfg.Automations {
							if ar.Name == msg.Name {
								m.appCfg.Automations[j] = r
								break
							}
						}
						break
					}
				}
				m.stack[i] = av
				break
			}
		}
		m.applyAutomationHints()
		return m, saveConfigCmd(m.resolvedCfg.ConfigFile, m.appCfg)

	default:
		// detail back message — handled via type assertion on string
		if isBackMsg(msg) {
			if len(m.stack) > 1 {
				m.stack = m.stack[:len(m.stack)-1]
			}
			return m, nil
		}
		return m, m.forwardToActive(msg)
	}
}

func (m *RootModel) View() string {
	var sb strings.Builder

	// Header
	sb.WriteString(styles.Header.Render("  joca — CI/CD pipeline dashboard"))
	sb.WriteByte('\n')

	// Active view
	if len(m.stack) > 0 {
		sb.WriteString(m.stack[len(m.stack)-1].View())
	}

	// Credential status (only on the list screen)
	sb.WriteString(m.credStatusView())

	// Footer bar
	sb.WriteByte('\n')
	triggerHint := "R: re-run  N: new run" // default: GitHub
	if listView, ok := m.stack[0].(list.Model); ok {
		if item, ok := listView.Selected(); ok && item.Entry.Provider == config.ProviderAWS {
			triggerHint = "R: new run"
		}
	}

	// Two-line footer: line1 = primary actions, line2 = secondary actions + status
	footerLine1 := "  ↑↓: navigate  enter: detail  o: browser  /: search  space: pause  r: refresh  " + triggerHint + "  q: quit"
	footerLine2 := "  S↑↓: reorder  p: pause all  h: hide/show paused  a: add  c: copy  A: automations"
	if _, ok := m.expiredSSOProfile(); ok {
		footerLine2 += "  l: SSO login"
	}
	if m.recorder.IsEnabled() {
		footerLine2 += "  t: tracking ✓  T: telemetry"
	} else {
		footerLine2 += "  t: tracking"
	}

	switch {
	case m.statusMsg != "":
		footerLine1 = "  " + m.statusMsg
	case m.globalPaused:
		footerLine1 += "   ⏸ auto-refresh paused"
	case !m.lastRefresh.IsZero():
		nextIn := time.Until(m.lastRefresh.Add(m.refreshInterval)).Round(time.Second)
		if nextIn < 0 {
			nextIn = 0
		}
		footerLine1 += fmt.Sprintf("   refresh in %s", humanDur(nextIn))
	}
	sb.WriteString(styles.Footer.Render(footerLine1))
	sb.WriteByte('\n')
	sb.WriteString(styles.Footer.Render(footerLine2))

	return sb.String()
}

// maybeNotify fires an OS notification if the pipeline's status changed since
// the last fetch. The first fetch for each pipeline is silently recorded
// without notifying to avoid a burst of notifications on startup.
func (m *RootModel) maybeNotify(item list.PipelineItem) {
	name := item.Entry.Name
	newStatus := item.Current.Status
	if item.Err != nil {
		newStatus = provider.StatusUnknown
	}

	prev, seen := m.prevStatus[name]
	m.prevStatus[name] = newStatus

	if !seen || prev == newStatus {
		return
	}

	notify.Send("joca — "+name, statusNotifyMessage(newStatus, item.Current), item.Current.URL)
}

func statusNotifyMessage(s provider.Status, run provider.Run) string {
	var base string
	switch s {
	case provider.StatusSuccess:
		base = "succeeded"
	case provider.StatusFailed:
		base = "failed"
	case provider.StatusRunning:
		base = "started running"
	case provider.StatusApproval:
		base = "awaiting approval"
	case provider.StatusCancelled:
		base = "cancelled"
	case provider.StatusPending:
		base = "pending"
	case provider.StatusIdle:
		base = "idle"
	default:
		base = "status changed"
	}

	var details []string
	if run.ID != "" {
		details = append(details, "#"+run.ID)
	}
	if run.Branch != "" {
		details = append(details, run.Branch)
	}
	if run.Stage != "" {
		details = append(details, run.Stage)
	}
	if len(details) == 0 {
		return base
	}
	return base + "  " + strings.Join(details, "  ")
}

// credStatusView renders a compact credential status bar on the list screen.
// It always shows both GitHub and AWS status, and appends setup hints for any
// provider that has no credentials configured.
func (m *RootModel) credStatusView() string {
	if len(m.stack) != 1 {
		return ""
	}

	var sb strings.Builder

	// Build compact bar: "  creds  GH ✓ source  ·  AWS ✓ source"
	ghPart := credLabel("GH", m.ghCred)
	awsPart := m.awsCredPart()

	sb.WriteString("\n  ")
	sb.WriteString(styles.Footer.Render("creds"))
	sb.WriteString("  ")
	sb.WriteString(ghPart)
	sb.WriteString("  ·  ")
	sb.WriteString(awsPart)

	// Append setup help for any missing provider.
	var helpLines []string
	if !m.ghCred.Pending && !m.ghCred.Present {
		helpLines = append(helpLines,
			"  GitHub Actions — option 1: export GITHUB_TOKEN=<your-token>",
			"                  option 2: run `gh auth login`",
		)
	}
	if !m.anyAWSPresent() {
		for _, profile := range m.missingAWSProfiles() {
			s := m.awsCreds[profile]
			switch {
			case profile == "":
				helpLines = append(helpLines,
					"  AWS CodePipeline — option 1: run `aws configure` (creates a default profile)",
					"                    option 2: export AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY",
					"                    option 3: run `joca add aws` and set a named profile",
				)
			case provider.IsSSOError(s.Err):
				helpLines = append(helpLines,
					fmt.Sprintf("  AWS profile %q — SSO token expired, run `aws sso login --profile %s`", profile, profile),
				)
			default:
				helpLines = append(helpLines,
					fmt.Sprintf("  AWS profile %q — run `aws configure --profile %s`", profile, profile),
				)
			}
		}
	}
	if len(helpLines) > 0 {
		sb.WriteByte('\n')
		for _, l := range helpLines {
			sb.WriteByte('\n')
			sb.WriteString(styles.Footer.Render(l))
		}
	}

	return sb.String()
}

// credLabel formats a single provider credential entry for the status bar.
func credLabel(name string, s credstatus.Status) string {
	if s.Pending {
		return styles.Footer.Render(name + " …")
	}
	if s.Present {
		return styles.CredOK.Render(name + " ✓ " + s.Source)
	}
	if provider.IsSSOError(s.Err) {
		return styles.CredMissing.Render(name + " ✗ SSO expired")
	}
	return styles.CredMissing.Render(name + " ✗ not configured")
}

// awsCredPart renders the AWS portion of the creds bar.
// With a single entry it shows "AWS ✓ source"; with multiple profiles it shows
// each profile's status inline: "AWS prod ✓  staging ✗".
func (m *RootModel) awsCredPart() string {
	if len(m.awsCreds) <= 1 {
		return credLabel("AWS", m.awsCreds[""])
	}
	// Multiple profiles — show each one.
	profiles := make([]string, 0, len(m.awsCreds))
	for k := range m.awsCreds {
		profiles = append(profiles, k)
	}
	sort.Strings(profiles)

	var parts []string
	for _, p := range profiles {
		s := m.awsCreds[p]
		label := p
		if label == "" {
			label = "(default)"
		}
		switch {
		case s.Pending:
			parts = append(parts, styles.Footer.Render(label+" …"))
		case s.Present:
			parts = append(parts, styles.CredOK.Render(label+" ✓"))
		default:
			parts = append(parts, styles.CredMissing.Render(label+" ✗"))
		}
	}
	return "AWS " + strings.Join(parts, "  ")
}

// anyAWSPresent reports whether at least one AWS profile has working credentials.
func (m *RootModel) anyAWSPresent() bool {
	for _, s := range m.awsCreds {
		if s.Present {
			return true
		}
	}
	return false
}

// missingAWSProfiles returns the sorted list of profile keys with Present=false.
func (m *RootModel) missingAWSProfiles() []string {
	var missing []string
	for profile, s := range m.awsCreds {
		if !s.Pending && !s.Present {
			missing = append(missing, profile)
		}
	}
	sort.Strings(missing)
	return missing
}

// expiredSSOProfile returns the AWS profile for the selected pipeline when its
// cached SSO credentials need to be refreshed.
func (m *RootModel) expiredSSOProfile() (string, bool) {
	if len(m.stack) != 1 {
		return "", false
	}
	listView, ok := m.stack[0].(list.Model)
	if !ok {
		return "", false
	}
	item, ok := listView.Selected()
	if !ok || item.Entry.Provider != config.ProviderAWS {
		return "", false
	}
	profile := item.Entry.AWSProfile
	status, ok := m.awsCreds[profile]
	return profile, ok && provider.IsSSOError(status.Err)
}

func awsSSOLoginCommand(profile string) *exec.Cmd {
	args := []string{"sso", "login"}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	return exec.Command("aws", args...)
}

func awsSSOLoginCmd(profile string) tea.Cmd {
	return tea.ExecProcess(awsSSOLoginCommand(profile), func(err error) tea.Msg {
		return ssoLoginDoneMsg{profile: profile, err: err}
	})
}

// forwardToActive sends msg to the active view and replaces it in the stack.
func (m *RootModel) forwardToActive(msg tea.Msg) tea.Cmd {
	if len(m.stack) == 0 {
		return nil
	}
	idx := len(m.stack) - 1
	updated, cmd := m.stack[idx].Update(msg)
	m.stack[idx] = updated
	return cmd
}

func saveConfigCmd(path string, cfg *config.AppConfig) tea.Cmd {
	return func() tea.Msg {
		_ = config.Save(path, cfg)
		return nil
	}
}

func (m *RootModel) clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearStatusMsg{} })
}

func (m *RootModel) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *RootModel) uiTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return uiTickMsg(t)
	})
}

func (m *RootModel) fetchAllCmd() tea.Cmd {
	if m.globalPaused {
		return nil
	}
	return m.fetchAllCmdInner()
}

func (m *RootModel) fetchAllCmdInner() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.providers))
	for i, p := range m.providers {
		i, p := i, p
		if m.paused[i] {
			continue
		}
		entry := m.appCfg.Pipelines[i]
		rec := m.recorder
		cmds[i] = func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			t0 := time.Now()
			current, err := p.CurrentStatus(ctx)
			rec.Record(telemetry.CallRecord{
				Pipeline: entry.Name, Provider: string(entry.Provider),
				Method: "CurrentStatus", At: t0, Duration: time.Since(t0), Err: err,
			})

			t1 := time.Now()
			history, hErr := p.RecentRuns(ctx, 6)
			rec.Record(telemetry.CallRecord{
				Pipeline: entry.Name, Provider: string(entry.Provider),
				Method: "RecentRuns", At: t1, Duration: time.Since(t1), Err: hErr,
			})

			// RecentRuns only has execution-level status, so it can't detect
			// approval (which requires stage/action detail). Find the matching
			// history entry by execution ID and promote its dot to approval.
			_ = hErr
			if err == nil && current.Status == provider.StatusApproval && current.ID != "" {
				for j := range history {
					if history[j].ID == current.ID {
						history[j].Status = provider.StatusApproval
						break
					}
				}
			}

			return fetchResultMsg{
				index: i,
				item: list.PipelineItem{
					Entry:   entry,
					URL:     p.URL(),
					Current: current,
					History: history,
					Err:     err,
				},
			}
		}
	}
	return tea.Batch(cmds...)
}

func (m *RootModel) fetchOneCmd(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.providers) {
		return nil
	}
	p := m.providers[idx]
	entry := m.appCfg.Pipelines[idx]
	rec := m.recorder
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		t0 := time.Now()
		current, err := p.CurrentStatus(ctx)
		rec.Record(telemetry.CallRecord{
			Pipeline: entry.Name, Provider: string(entry.Provider),
			Method: "CurrentStatus", At: t0, Duration: time.Since(t0), Err: err,
		})

		t1 := time.Now()
		history, hErr := p.RecentRuns(ctx, 6)
		rec.Record(telemetry.CallRecord{
			Pipeline: entry.Name, Provider: string(entry.Provider),
			Method: "RecentRuns", At: t1, Duration: time.Since(t1), Err: hErr,
		})

		_ = hErr
		if err == nil && current.Status == provider.StatusApproval && current.ID != "" {
			for j := range history {
				if history[j].ID == current.ID {
					history[j].Status = provider.StatusApproval
					break
				}
			}
		}

		return fetchResultMsg{
			index: idx,
			item: list.PipelineItem{
				Entry:   entry,
				URL:     p.URL(),
				Current: current,
				History: history,
				Err:     err,
			},
		}
	}
}

func (m *RootModel) triggerCmd(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.providers) {
		return nil
	}
	p := m.providers[idx]
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := p.Trigger(ctx)
		return triggerDoneMsg{index: idx, err: err}
	}
}

func (m *RootModel) triggerNewCmd(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.providers) {
		return nil
	}
	p := m.providers[idx]
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := p.TriggerNew(ctx)
		return triggerNewDoneMsg{index: idx, err: err}
	}
}

// applyAutomationHints recomputes AutomationHints for all items in the list
// view based on the current automation rules.
func (m *RootModel) applyAutomationHints() {
	listView, ok := m.stack[0].(list.Model)
	if !ok {
		return
	}
	// Index pipeline name → item index for quick lookup.
	nameToIdx := make(map[string]int, len(listView.Items))
	for i, item := range listView.Items {
		nameToIdx[item.Entry.Name] = i
		listView.Items[i].AutomationHints = list.AutomationHints{}
	}
	for _, rule := range m.appCfg.Automations {
		if rule.Disabled {
			if idx, ok := nameToIdx[rule.WatchPipeline]; ok {
				if !listView.Items[idx].AutomationHints.Watched {
					listView.Items[idx].AutomationHints.WatchedDisabled = true
				}
			}
			if idx, ok := nameToIdx[rule.TriggerPipeline]; ok {
				if !listView.Items[idx].AutomationHints.Target {
					listView.Items[idx].AutomationHints.TargetDisabled = true
				}
			}
			continue
		}
		if idx, ok := nameToIdx[rule.WatchPipeline]; ok {
			listView.Items[idx].AutomationHints.Watched = true
			listView.Items[idx].AutomationHints.WatchedDisabled = false
		}
		if idx, ok := nameToIdx[rule.TriggerPipeline]; ok {
			listView.Items[idx].AutomationHints.Target = true
			listView.Items[idx].AutomationHints.TargetDisabled = false
		}
	}
	m.stack[0] = listView
}

// findProviderByName returns the index of the provider whose pipeline entry has
// the given name, or (-1, false) if none found.
func (m *RootModel) findProviderByName(name string) (int, bool) {
	for i, e := range m.appCfg.Pipelines {
		if e.Name == name {
			return i, true
		}
	}
	return -1, false
}

// evaluateAutomations checks all enabled automation rules that watch pipelineName
// transitioning to nextStatus. Matching rules fire TriggerNew on their target
// pipeline. FireCount is incremented and rules are marked disabled when exhausted.
func (m *RootModel) evaluateAutomations(pipelineName string, prev, next provider.Status) tea.Cmd {
	_ = prev // used only for the transition guard in the caller
	nextStr := string(next)

	var cmds []tea.Cmd
	for i, rule := range m.appCfg.Automations {
		if rule.Disabled {
			continue
		}
		if rule.WatchPipeline != pipelineName {
			continue
		}
		if rule.OnStatus != nextStr {
			continue
		}

		// Capture loop vars.
		i := i
		rule := rule

		idx, ok := m.findProviderByName(rule.TriggerPipeline)
		if !ok {
			continue
		}
		p := m.providers[idx]
		targetName := rule.TriggerPipeline
		ruleName := rule.Name

		// Update fire count and disabled flag immediately (optimistic).
		m.appCfg.Automations[i].FireCount++
		if rule.MaxFires > 0 && m.appCfg.Automations[i].FireCount >= rule.MaxFires {
			m.appCfg.Automations[i].Disabled = true
		}

		configFile := m.resolvedCfg.ConfigFile
		appCfg := m.appCfg

		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := p.TriggerNew(ctx)
			// Persist updated fire count / disabled flag.
			_ = config.Save(configFile, appCfg)
			return automationFiredMsg{ruleName: ruleName, targetPipeline: targetName, err: err}
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// checkCredsCmd runs all credential checks concurrently and returns a single
// credCheckDoneMsg when all are complete.
func (m *RootModel) checkCredsCmd() tea.Cmd {
	// Snapshot the set of AWS profiles we need to check.
	profiles := make([]string, 0, len(m.awsCreds))
	for p := range m.awsCreds {
		profiles = append(profiles, p)
	}
	return func() tea.Msg {
		var mu sync.Mutex
		var wg sync.WaitGroup
		awsResults := make(map[string]credstatus.Status, len(profiles))

		wg.Add(1)
		var ghResult credstatus.Status
		go func() {
			defer wg.Done()
			ghResult = credstatus.CheckGitHub()
		}()

		for _, p := range profiles {
			p := p
			wg.Add(1)
			go func() {
				defer wg.Done()
				s := credstatus.CheckAWS(p)
				mu.Lock()
				awsResults[p] = s
				mu.Unlock()
			}()
		}

		wg.Wait()
		return credCheckDoneMsg{gh: ghResult, aws: awsResults}
	}
}

func buildProviders(entries []config.PipelineEntry) []provider.Provider {
	providers := make([]provider.Provider, len(entries))
	var wg sync.WaitGroup
	for i, e := range entries {
		i, e := i, e
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch e.Provider {
			case config.ProviderGitHub:
				p, err := ghprovider.New(e.Owner, e.Repo, e.Workflow)
				if err != nil {
					ghURL := fmt.Sprintf("https://github.com/%s/%s/actions", e.Owner, e.Repo)
					if e.Workflow != "" {
						ghURL = fmt.Sprintf("https://github.com/%s/%s/actions/workflows/%s", e.Owner, e.Repo, e.Workflow)
					}
					providers[i] = &errorProvider{err: err, url: ghURL}
				} else {
					providers[i] = p
				}
			case config.ProviderAWS:
				p, err := awsprovider.New(context.Background(), e.PipelineName, e.AWSRegion, e.AWSProfile)
				if err != nil {
					providers[i] = &errorProvider{err: err}
				} else {
					providers[i] = p
				}
			default:
				providers[i] = &errorProvider{err: fmt.Errorf("unknown provider %q", e.Provider)}
			}
		}()
	}
	wg.Wait()
	return providers
}

func makeItems(entries []config.PipelineEntry) []list.PipelineItem {
	items := make([]list.PipelineItem, len(entries))
	for i, e := range entries {
		items[i] = list.PipelineItem{Entry: e, Paused: e.Paused}
	}
	return items
}

func openBrowser(url string) {
	if url == "" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}

func humanDur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

// isBackMsg detects unexported backMsg types from child views via type name suffix.
func isBackMsg(msg tea.Msg) bool {
	return strings.HasSuffix(fmt.Sprintf("%T", msg), ".backMsg")
}

// errorProvider is a no-op provider used when initialization fails.
type errorProvider struct {
	err error
	url string
}

func (e *errorProvider) CurrentStatus(_ context.Context) (provider.Run, error) {
	return provider.Run{Status: provider.StatusUnknown}, e.err
}
func (e *errorProvider) RecentRuns(_ context.Context, _ int) ([]provider.Run, error) {
	return nil, e.err
}
func (e *errorProvider) Trigger(_ context.Context) error    { return e.err }
func (e *errorProvider) TriggerNew(_ context.Context) error { return e.err }
func (e *errorProvider) URL() string                        { return e.url }
