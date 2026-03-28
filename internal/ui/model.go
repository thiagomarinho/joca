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
	"github.com/thiagomarinho/joca/internal/ui/addwizard"
	"github.com/thiagomarinho/joca/internal/ui/detail"
	"github.com/thiagomarinho/joca/internal/ui/list"
	"github.com/thiagomarinho/joca/internal/ui/styles"
	uitelemetry "github.com/thiagomarinho/joca/internal/ui/telemetry"
)

type tickMsg time.Time
type uiTickMsg time.Time
type clearStatusMsg struct{}

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
	paused     map[int]bool // pipelines with auto-refresh disabled
	recorder   *telemetry.Recorder
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
		recorder:        telemetry.NewRecorder(),
		ghCred:          credstatus.Status{Pending: true},
		awsCreds:        awsCreds,
	}

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
		}
		return m, m.forwardToActive(msg)

	case credCheckDoneMsg:
		m.ghCred = msg.gh
		m.awsCreds = msg.aws
		return m, nil

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
		m.maybeNotify(msg.item)
		// Update the list view's item
		listView, ok := m.stack[0].(list.Model)
		if ok && msg.index >= 0 && msg.index < len(listView.Items) {
			listView.Items[msg.index] = msg.item
			m.stack[0] = listView
		}
		return m, nil

	case list.TogglePauseMsg:
		idx := msg.Index
		m.paused[idx] = !m.paused[idx]
		m.appCfg.Pipelines[idx].Paused = m.paused[idx]
		if listView, ok := m.stack[0].(list.Model); ok {
			listView.Items[idx].Paused = m.paused[idx]
			m.stack[0] = listView
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
	triggerHint := "R: re-run  N: new run" // default: GitHub
	if listView, ok := m.stack[0].(list.Model); ok {
		if item, ok := listView.Selected(); ok && item.Entry.Provider == config.ProviderAWS {
			triggerHint = "R: new run"
		}
	}
	footer := "  ↑↓: navigate  S↑↓: reorder  enter: detail  space: pause/resume  o: browser  a: add  r: refresh  " + triggerHint
	if m.recorder.IsEnabled() {
		footer += "  t: tracking ✓  T: telemetry"
	} else {
		footer += "  t: tracking"
	}
	footer += "  q: quit"
	if m.statusMsg != "" {
		footer = "  " + m.statusMsg
	} else if !m.lastRefresh.IsZero() {
		nextIn := time.Until(m.lastRefresh.Add(m.refreshInterval)).Round(time.Second)
		if nextIn < 0 {
			nextIn = 0
		}
		footer += fmt.Sprintf("   refresh in %s", humanDur(nextIn))
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
