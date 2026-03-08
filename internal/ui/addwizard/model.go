package addwizard

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/credstatus"
	"github.com/thiagomarinho/joca/internal/provider"
	awslist "github.com/thiagomarinho/joca/internal/provider/aws"
	ghlist "github.com/thiagomarinho/joca/internal/provider/github"
	"github.com/thiagomarinho/joca/internal/ui/styles"
)

const pageSize = 10

type step int

const (
	stepProvider step = iota
	stepAWSCredentials
	stepGHRepo
	stepGHWorkflow
	stepAWSPipeline
)

// SavedMsg is emitted after all selected pipelines have been persisted.
type SavedMsg struct{ Entries []config.PipelineEntry }

// CancelledMsg is emitted when the user cancels the wizard.
type CancelledMsg struct{}

// internal async messages
type reposLoadedMsg struct {
	repos []ghlist.Repo
	token string
	err   error
}
type workflowsLoadedMsg struct {
	wfs []ghlist.WorkflowInfo
	err error
}
type awsPipelinesMsg struct {
	names []string
	err   error
}
type awsCredCheckMsg struct {
	status credstatus.Status
}

// Model is the guided add-pipeline wizard.
type Model struct {
	configFile string
	step       step

	// provider selection
	providerCursor int // 0=GitHub, 1=AWS

	// github token (filled on first GH API call)
	token string

	// github repo browser
	repos        []ghlist.Repo
	reposLoading bool
	reposFilter  string
	reposCursor  int
	reposOffset  int
	selectedRepo *ghlist.Repo

	// github workflow selector
	workflows  []ghlist.WorkflowInfo
	wfLoading  bool
	wfCursor   int
	wfSelected map[int]bool

	// aws credential inputs (stepAWSCredentials)
	awsRegion    string
	awsProfile   string
	awsCredField int  // 0=region, 1=profile
	awsChecking  bool // true while credential check is in-flight

	// aws pipeline browser
	awsPipelines []string
	awsLoading   bool
	awsFilter    string
	awsCursor    int
	awsOffset    int

	err string
}

// New creates a fresh wizard model.
func New(configFile string) Model {
	return Model{
		configFile: configFile,
		wfSelected: make(map[int]bool),
	}
}

func (m Model) Init() tea.Cmd { return nil }

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case reposLoadedMsg:
		m.reposLoading = false
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.repos = msg.repos
			m.token = msg.token
		}
		return m, nil

	case workflowsLoadedMsg:
		m.wfLoading = false
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.workflows = msg.wfs
		}
		return m, nil

	case awsCredCheckMsg:
		m.awsChecking = false
		if !msg.status.Present {
			if m.awsProfile != "" {
				m.err = fmt.Sprintf("no credentials found for profile %q", m.awsProfile)
			} else {
				m.err = "no AWS credentials found — set a profile or configure default credentials"
			}
			return m, nil
		}
		// Credentials OK — proceed to pipeline list.
		m.step = stepAWSPipeline
		m.awsLoading = true
		m.err = ""
		return m, loadAWSPipelinesCmd(m.awsRegion, m.awsProfile)

	case awsPipelinesMsg:
		m.awsLoading = false
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.awsPipelines = msg.names
		}
		return m, nil

	case tea.KeyMsg:
		switch m.step {
		case stepProvider:
			return m.updateProvider(msg)
		case stepAWSCredentials:
			return m.updateAWSCredentials(msg)
		case stepGHRepo:
			return m.updateGHRepo(msg)
		case stepGHWorkflow:
			return m.updateGHWorkflow(msg)
		case stepAWSPipeline:
			return m.updateAWSPipeline(msg)
		}
	}
	return m, nil
}

func (m Model) updateProvider(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return CancelledMsg{} }
	case "up", "k":
		if m.providerCursor > 0 {
			m.providerCursor--
		}
	case "down", "j":
		if m.providerCursor < 1 {
			m.providerCursor++
		}
	case "enter":
		if m.providerCursor == 0 {
			// GitHub → load repos
			m.step = stepGHRepo
			m.reposLoading = true
			m.err = ""
			return m, loadReposCmd()
		}
		// AWS → ask for region + profile first
		m.step = stepAWSCredentials
		m.awsCredField = 0
		m.err = ""
	}
	return m, nil
}

func (m Model) updateAWSCredentials(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.awsChecking {
		return m, nil // ignore input while check is in-flight
	}
	switch msg.String() {
	case "esc":
		m.step = stepProvider
		m.err = ""
	case "tab", "down":
		m.awsCredField = 1 - m.awsCredField
	case "up":
		m.awsCredField = 1 - m.awsCredField
	case "backspace":
		if m.awsCredField == 0 && len(m.awsRegion) > 0 {
			m.awsRegion = m.awsRegion[:len(m.awsRegion)-1]
		} else if m.awsCredField == 1 && len(m.awsProfile) > 0 {
			m.awsProfile = m.awsProfile[:len(m.awsProfile)-1]
		}
	case "enter":
		m.awsChecking = true
		m.err = ""
		profile := m.awsProfile
		return m, func() tea.Msg {
			return awsCredCheckMsg{status: credstatus.CheckAWS(profile)}
		}
	default:
		if msg.Type == tea.KeyRunes {
			if m.awsCredField == 0 {
				m.awsRegion += string(msg.Runes)
			} else {
				m.awsProfile += string(msg.Runes)
			}
		}
	}
	return m, nil
}

func (m Model) updateGHRepo(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filtered := m.filteredRepos()
	switch msg.String() {
	case "esc":
		m.step = stepProvider
		m.err = ""
	case "up":
		if m.reposCursor > 0 {
			m.reposCursor--
			if m.reposCursor < m.reposOffset {
				m.reposOffset = m.reposCursor
			}
		}
	case "down":
		if m.reposCursor < len(filtered)-1 {
			m.reposCursor++
			if m.reposCursor >= m.reposOffset+pageSize {
				m.reposOffset = m.reposCursor - pageSize + 1
			}
		}
	case "backspace":
		if len(m.reposFilter) > 0 {
			m.reposFilter = m.reposFilter[:len(m.reposFilter)-1]
			m.reposCursor = 0
			m.reposOffset = 0
		}
	case "enter":
		if len(filtered) == 0 {
			break
		}
		repo := filtered[m.reposCursor]
		m.selectedRepo = &repo
		m.step = stepGHWorkflow
		m.wfLoading = true
		m.wfCursor = 0
		m.wfSelected = make(map[int]bool)
		m.err = ""
		token := m.token
		return m, loadWorkflowsCmd(repo.Owner, repo.Name, token)
	default:
		if msg.Type == tea.KeyRunes {
			m.reposFilter += string(msg.Runes)
			m.reposCursor = 0
			m.reposOffset = 0
		}
	}
	return m, nil
}

func (m Model) updateGHWorkflow(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.step = stepGHRepo
		m.err = ""
	case "up", "k":
		if m.wfCursor > 0 {
			m.wfCursor--
		}
	case "down", "j":
		if m.wfCursor < len(m.workflows)-1 {
			m.wfCursor++
		}
	case " ":
		m.wfSelected[m.wfCursor] = !m.wfSelected[m.wfCursor]
	case "enter":
		return m.saveGHWorkflows()
	}
	return m, nil
}

func (m Model) updateAWSPipeline(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filtered := m.filteredAWSPipelines()
	switch msg.String() {
	case "esc":
		m.step = stepAWSCredentials
		m.awsPipelines = nil
		m.awsFilter = ""
		m.awsCursor = 0
		m.awsOffset = 0
		m.err = ""
	case "up":
		if m.awsCursor > 0 {
			m.awsCursor--
			if m.awsCursor < m.awsOffset {
				m.awsOffset = m.awsCursor
			}
		}
	case "down":
		if m.awsCursor < len(filtered)-1 {
			m.awsCursor++
			if m.awsCursor >= m.awsOffset+pageSize {
				m.awsOffset = m.awsCursor - pageSize + 1
			}
		}
	case "backspace":
		if len(m.awsFilter) > 0 {
			m.awsFilter = m.awsFilter[:len(m.awsFilter)-1]
			m.awsCursor = 0
			m.awsOffset = 0
		}
	case "enter":
		if len(filtered) == 0 {
			break
		}
		return m.saveAWSPipeline(filtered[m.awsCursor])
	default:
		if msg.Type == tea.KeyRunes {
			m.awsFilter += string(msg.Runes)
			m.awsCursor = 0
			m.awsOffset = 0
		}
	}
	return m, nil
}

// ── Save helpers ──────────────────────────────────────────────────────────────

func (m Model) saveGHWorkflows() (tea.Model, tea.Cmd) {
	var entries []config.PipelineEntry
	for i, wf := range m.workflows {
		if !m.wfSelected[i] {
			continue
		}
		name := m.selectedRepo.Name + " / " + wf.Name
		entries = append(entries, config.PipelineEntry{
			Name:     name,
			Provider: config.ProviderGitHub,
			Owner:    m.selectedRepo.Owner,
			Repo:     m.selectedRepo.Name,
			Workflow: wf.Filename,
		})
	}
	if len(entries) == 0 {
		m.err = "Select at least one workflow (space to toggle)"
		return m, nil
	}
	for _, e := range entries {
		if err := config.AddPipeline(m.configFile, e); err != nil {
			m.err = err.Error()
			return m, nil
		}
	}
	saved := entries
	return m, func() tea.Msg { return SavedMsg{Entries: saved} }
}

func (m Model) saveAWSPipeline(name string) (tea.Model, tea.Cmd) {
	entry := config.PipelineEntry{
		Name:         name,
		Provider:     config.ProviderAWS,
		PipelineName: name,
		AWSRegion:    m.awsRegion,
		AWSProfile:   m.awsProfile,
	}
	if err := config.AddPipeline(m.configFile, entry); err != nil {
		m.err = err.Error()
		return m, nil
	}
	return m, func() tea.Msg { return SavedMsg{Entries: []config.PipelineEntry{entry}} }
}

// ── Filter helpers ────────────────────────────────────────────────────────────

func (m *Model) filteredRepos() []ghlist.Repo {
	if m.reposFilter == "" {
		return m.repos
	}
	f := strings.ToLower(m.reposFilter)
	var out []ghlist.Repo
	for _, r := range m.repos {
		if strings.Contains(strings.ToLower(r.FullName), f) {
			out = append(out, r)
		}
	}
	return out
}

func (m *Model) filteredAWSPipelines() []string {
	if m.awsFilter == "" {
		return m.awsPipelines
	}
	f := strings.ToLower(m.awsFilter)
	var out []string
	for _, p := range m.awsPipelines {
		if strings.Contains(strings.ToLower(p), f) {
			out = append(out, p)
		}
	}
	return out
}

// ── Async commands ────────────────────────────────────────────────────────────

func loadReposCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		token, err := ghlist.ResolveToken()
		if err != nil {
			return reposLoadedMsg{err: fmt.Errorf("github auth: %w", err)}
		}
		repos, err := ghlist.ListUserRepos(ctx, token, nil)
		return reposLoadedMsg{repos: repos, token: token, err: err}
	}
}

func loadWorkflowsCmd(owner, repo, token string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		wfs, err := ghlist.ListWorkflows(ctx, owner, repo, token, nil)
		return workflowsLoadedMsg{wfs: wfs, err: err}
	}
}

func loadAWSPipelinesCmd(region, profile string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		names, err := awslist.ListPipelines(ctx, region, profile)
		return awsPipelinesMsg{names: names, err: err}
	}
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	switch m.step {
	case stepProvider:
		return m.viewProvider()
	case stepAWSCredentials:
		return m.viewAWSCredentials()
	case stepGHRepo:
		return m.viewGHRepo()
	case stepGHWorkflow:
		return m.viewGHWorkflow()
	case stepAWSPipeline:
		return m.viewAWSPipeline()
	}
	return ""
}

func (m Model) viewProvider() string {
	var sb strings.Builder
	sb.WriteString("\n  ")
	sb.WriteString(styles.FormTitle.Render("Add pipeline"))
	sb.WriteString("\n\n")

	options := []string{"GitHub", "AWS"}
	for i, opt := range options {
		cursor := "  "
		if i == m.providerCursor {
			cursor = styles.FormCursor.Render("> ")
			sb.WriteString(cursor + styles.FormInput.Render(opt) + "\n")
		} else {
			sb.WriteString(cursor + opt + "\n")
		}
	}

	sb.WriteString("\n")
	if m.err != "" {
		sb.WriteString("  " + styles.FormError.Render("✗ "+m.err) + "\n\n")
	}
	sb.WriteString("  " + styles.Footer.Render("↑↓: navigate  enter: select  esc: cancel"))
	return sb.String()
}

func (m Model) viewAWSCredentials() string {
	var sb strings.Builder
	sb.WriteString("\n  ")
	sb.WriteString(styles.FormTitle.Render("Add AWS pipeline  —  credentials"))
	sb.WriteString("\n\n")

	fields := []struct{ label, value string }{
		{"Region: ", m.awsRegion},
		{"Profile:", m.awsProfile},
	}
	for i, f := range fields {
		active := i == m.awsCredField
		label := styles.FormLabel.Render("  " + f.label + "  ")
		var val string
		if active {
			val = styles.FormInput.Render(f.value) + styles.FormCursor.Render("_")
		} else {
			val = styles.Footer.Render(f.value)
		}
		sb.WriteString(label + val + "\n")
	}
	sb.WriteString("  " + styles.Footer.Render("  (both optional)") + "\n")

	sb.WriteString("\n")
	if m.awsChecking {
		sb.WriteString("  " + styles.Footer.Render("Checking credentials…") + "\n\n")
	} else if m.err != "" {
		sb.WriteString("  " + styles.FormError.Render("✗ "+m.err) + "\n\n")
	}
	sb.WriteString("  " + styles.Footer.Render("tab/↑↓: switch field  enter: continue  esc: back"))
	return sb.String()
}

func (m Model) viewGHRepo() string {
	var sb strings.Builder
	sb.WriteString("\n  ")
	sb.WriteString(styles.FormTitle.Render("Add GitHub pipeline  —  select a repository"))
	sb.WriteString("\n\n")

	sb.WriteString("  Filter: " + styles.FormInput.Render(m.reposFilter) + styles.FormCursor.Render("_") + "\n\n")

	if m.reposLoading {
		sb.WriteString("  Loading repositories…\n")
	} else {
		filtered := m.filteredRepos()
		if len(filtered) == 0 {
			sb.WriteString("  " + styles.Footer.Render("No repositories found") + "\n")
		} else {
			end := m.reposOffset + pageSize
			if end > len(filtered) {
				end = len(filtered)
			}
			for i := m.reposOffset; i < end; i++ {
				r := filtered[i]
				if i == m.reposCursor {
					sb.WriteString(styles.FormCursor.Render("  > ") + styles.SelectedRow.Render(r.FullName) + "\n")
				} else {
					sb.WriteString("    " + r.FullName + "\n")
				}
			}
			if len(filtered) > pageSize {
				sb.WriteString("  " + styles.Footer.Render(fmt.Sprintf("  %d/%d", m.reposCursor+1, len(filtered))) + "\n")
			}
		}
	}

	sb.WriteString("\n")
	if m.err != "" {
		sb.WriteString("  " + styles.FormError.Render("✗ "+m.err) + "\n\n")
	}
	sb.WriteString("  " + styles.Footer.Render("↑↓: navigate  type: filter  enter: select  esc: back"))
	return sb.String()
}

func (m Model) viewGHWorkflow() string {
	var sb strings.Builder
	title := "select workflow(s)"
	if m.selectedRepo != nil {
		title = m.selectedRepo.FullName + "  —  " + title
	}
	sb.WriteString("\n  ")
	sb.WriteString(styles.FormTitle.Render(title))
	sb.WriteString("\n\n")

	switch {
	case m.wfLoading:
		sb.WriteString("  Loading workflows…\n")
	case len(m.workflows) == 0:
		sb.WriteString("  " + styles.Footer.Render("No workflows found") + "\n")
	default:
		for i, wf := range m.workflows {
			checked := "[ ]"
			if m.wfSelected[i] {
				checked = styles.MarkerSelected.Render("[x]")
			}
			dot := statusDot(wf.LastStatus)
			statusLabel := styles.Footer.Render(string(wf.LastStatus))
			line := fmt.Sprintf("  %s %-20s %s %-12s %s",
				checked,
				wf.Name,
				dot,
				statusLabel,
				styles.Footer.Render(wf.Filename),
			)
			if i == m.wfCursor {
				sb.WriteString(styles.FormCursor.Render("> ") + styles.SelectedRow.Render(line) + "\n")
			} else {
				sb.WriteString("  " + line + "\n")
			}
		}
	}

	sb.WriteString("\n")
	if m.err != "" {
		sb.WriteString("  " + styles.FormError.Render("✗ "+m.err) + "\n\n")
	}
	sb.WriteString("  " + styles.Footer.Render("↑↓: navigate  space: toggle  enter: confirm  esc: back"))
	return sb.String()
}

func (m Model) viewAWSPipeline() string {
	var sb strings.Builder

	title := "Add AWS pipeline  —  select a pipeline"
	if m.awsProfile != "" {
		title += fmt.Sprintf("  [profile: %s]", m.awsProfile)
	}
	if m.awsRegion != "" {
		title += fmt.Sprintf("  [region: %s]", m.awsRegion)
	}
	sb.WriteString("\n  ")
	sb.WriteString(styles.FormTitle.Render(title))
	sb.WriteString("\n\n")

	sb.WriteString("  Filter: " + styles.FormInput.Render(m.awsFilter) + styles.FormCursor.Render("_") + "\n\n")

	if m.awsLoading {
		sb.WriteString("  Loading pipelines…\n")
	} else {
		filtered := m.filteredAWSPipelines()
		if len(filtered) == 0 {
			sb.WriteString("  " + styles.Footer.Render("No pipelines found") + "\n")
		} else {
			end := m.awsOffset + pageSize
			if end > len(filtered) {
				end = len(filtered)
			}
			for i := m.awsOffset; i < end; i++ {
				p := filtered[i]
				if i == m.awsCursor {
					sb.WriteString(styles.FormCursor.Render("  > ") + styles.SelectedRow.Render(p) + "\n")
				} else {
					sb.WriteString("    " + p + "\n")
				}
			}
			if len(filtered) > pageSize {
				sb.WriteString("  " + styles.Footer.Render(fmt.Sprintf("  %d/%d", m.awsCursor+1, len(filtered))) + "\n")
			}
		}
	}

	sb.WriteString("\n")
	if m.err != "" {
		sb.WriteString("  " + styles.FormError.Render("✗ "+m.err) + "\n\n")
	}
	sb.WriteString("  " + styles.Footer.Render("↑↓: navigate  type: filter  enter: select  esc: back"))
	return sb.String()
}

func statusDot(s provider.Status) string {
	switch s {
	case provider.StatusSuccess:
		return styles.DotSuccess.Render(styles.DotFull)
	case provider.StatusFailed:
		return styles.DotFailed.Render(styles.DotFull)
	case provider.StatusRunning:
		return styles.DotRunning.Render(styles.DotFull)
	case provider.StatusCancelled:
		return styles.DotCancelled.Render(styles.DotFull)
	default:
		return styles.DotOther.Render(styles.DotEmpty)
	}
}
