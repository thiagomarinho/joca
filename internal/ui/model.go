package ui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/credstatus"
	"github.com/thiagomarinho/joca/internal/notify"
	"github.com/thiagomarinho/joca/internal/provider"
	awsprovider "github.com/thiagomarinho/joca/internal/provider/aws"
	ghprovider "github.com/thiagomarinho/joca/internal/provider/github"
	"github.com/thiagomarinho/joca/internal/ui/addwizard"
	"github.com/thiagomarinho/joca/internal/ui/detail"
	"github.com/thiagomarinho/joca/internal/ui/list"
	"github.com/thiagomarinho/joca/internal/ui/styles"
	"github.com/thiagomarinho/joca/internal/ui/watch"
)

type tickMsg time.Time

type fetchResultMsg struct {
	index int
	item  list.PipelineItem
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
	prevStatus map[string]provider.Status
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

	m := &RootModel{
		appCfg:          appCfg,
		resolvedCfg:     resolvedCfg,
		providers:       providers,
		refreshInterval: interval,
		stack:           []tea.Model{list.New(items)},
		prevStatus:      make(map[string]provider.Status),
	}

	// Check credentials for all providers, regardless of what is configured.
	m.ghCred = credstatus.CheckGitHub()
	m.awsCreds = make(map[string]credstatus.Status)
	awsProfiles := map[string]bool{"": true} // always check default chain
	for _, e := range appCfg.Pipelines {
		if e.Provider == config.ProviderAWS {
			awsProfiles[e.AWSProfile] = true
		}
	}
	for profile := range awsProfiles {
		m.awsCreds[profile] = credstatus.CheckAWS(profile)
	}

	return m
}

func (m *RootModel) Init() tea.Cmd {
	return tea.Batch(
		m.tickCmd(),
		m.fetchAllCmd(),
	)
}

func (m *RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, m.forwardToActive(msg)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if len(m.stack) == 1 {
				return m, tea.Quit
			}
		case "r":
			if len(m.stack) == 1 {
				m.statusMsg = "Refreshing…"
				return m, m.fetchAllCmd()
			}
		}
		return m, m.forwardToActive(msg)

	case tickMsg:
		return m, tea.Batch(m.tickCmd(), m.fetchAllCmd())

	case fetchResultMsg:
		m.lastRefresh = time.Now()
		m.statusMsg = ""
		m.maybeNotify(msg.item)
		// Update the list view's item
		listView, ok := m.stack[0].(list.Model)
		if ok && msg.index >= 0 && msg.index < len(listView.Items) {
			listView.Items[msg.index] = msg.item
			m.stack[0] = listView
		}
		// Forward update to watch view if present in stack
		for i, v := range m.stack {
			if wv, ok := v.(watch.Model); ok {
				m.stack[i] = wv.UpdateItem(msg.item)
			}
		}
		return m, nil

	// Messages from child views
	case list.OpenWatchMsg:
		m.stack = append(m.stack, watch.New(msg.Items))
		return m, nil

	case list.OpenDetailMsg:
		m.stack = append(m.stack, detail.New(msg.Item))
		return m, nil

	case list.OpenAddFormMsg:
		m.stack = append(m.stack, addwizard.New(m.resolvedCfg.ConfigFile))
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
			m.stack[0] = list.New(makeItems(newCfg.Pipelines))
		}
		return m, m.fetchAllCmd()

	case addwizard.CancelledMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, nil

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
	footer := "  ↑↓: navigate  enter: detail  space: pin  w: watch  o: browser  a: add  r: refresh  q: quit"
	if m.statusMsg != "" {
		footer = "  " + m.statusMsg
	} else if !m.lastRefresh.IsZero() {
		footer += fmt.Sprintf("   last updated %s ago", humanDur(time.Since(m.lastRefresh)))
	}
	sb.WriteString(styles.Footer.Render(footer))

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
	if !m.ghCred.Present {
		helpLines = append(helpLines,
			"  GitHub Actions — option 1: export GITHUB_TOKEN=<your-token>",
			"                  option 2: run `gh auth login`",
		)
	}
	for _, profile := range m.missingAWSProfiles() {
		if profile == "" {
			helpLines = append(helpLines,
				"  AWS CodePipeline — option 1: run `aws configure` (creates a default profile)",
				"                    option 2: export AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY",
				"                    option 3: run `joca add aws` and set a named profile",
			)
		} else {
			helpLines = append(helpLines,
				fmt.Sprintf("  AWS profile %q — run `aws configure --profile %s`", profile, profile),
			)
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
	if s.Present {
		return styles.CredOK.Render(name + " ✓ " + s.Source)
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
		if s.Present {
			parts = append(parts, styles.CredOK.Render(label+" ✓"))
		} else {
			parts = append(parts, styles.CredMissing.Render(label+" ✗"))
		}
	}
	return "AWS " + strings.Join(parts, "  ")
}

// missingAWSProfiles returns the sorted list of profile keys with Present=false.
func (m *RootModel) missingAWSProfiles() []string {
	var missing []string
	for profile, s := range m.awsCreds {
		if !s.Present {
			missing = append(missing, profile)
		}
	}
	sort.Strings(missing)
	return missing
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

func (m *RootModel) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *RootModel) fetchAllCmd() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.providers))
	for i, p := range m.providers {
		i, p := i, p
		entry := m.appCfg.Pipelines[i]
		cmds[i] = func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			current, err := p.CurrentStatus(ctx)
			history, _ := p.RecentRuns(ctx, 6)

			// RecentRuns only has execution-level status, so it can't detect
			// approval (which requires stage/action detail). Find the matching
			// history entry by execution ID and promote its dot to approval.
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
					Current: current,
					History: history,
					Err:     err,
				},
			}
		}
	}
	return tea.Batch(cmds...)
}

func buildProviders(entries []config.PipelineEntry) []provider.Provider {
	providers := make([]provider.Provider, len(entries))
	for i, e := range entries {
		switch e.Provider {
		case config.ProviderGitHub:
			p, err := ghprovider.New(e.Owner, e.Repo, e.Workflow)
			if err != nil {
				providers[i] = &errorProvider{err: err, url: fmt.Sprintf("https://github.com/%s/%s/actions", e.Owner, e.Repo)}
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
	}
	return providers
}

func makeItems(entries []config.PipelineEntry) []list.PipelineItem {
	items := make([]list.PipelineItem, len(entries))
	for i, e := range entries {
		items[i] = list.PipelineItem{Entry: e}
	}
	return items
}

func openBrowser(url string) {
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

// isBackMsg detects unexported backMsg types from child views via type name.
func isBackMsg(msg tea.Msg) bool {
	t := fmt.Sprintf("%T", msg)
	return t == "detail.backMsg" || t == "watch.backMsg"
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
func (e *errorProvider) Trigger(_ context.Context) error { return e.err }
func (e *errorProvider) URL() string                     { return e.url }
