package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/thiagomarinho/joca/internal/provider"
)

const apiBase = "https://api.github.com"

// Client fetches GitHub Actions workflow runs for a given repo.
type Client struct {
	owner    string
	repo     string
	workflow string // optional: filename e.g. "ci.yml"; empty = all workflows
	token    string
	http     *http.Client
}

// New creates a GitHub Actions client. Auth is resolved automatically:
// GITHUB_TOKEN env var takes precedence, then the gh CLI token.
// workflow is optional (e.g. "ci.yml"); empty means all workflows.
func New(owner, repo, workflow string) (*Client, error) {
	token, err := resolveToken()
	if err != nil {
		return nil, fmt.Errorf("github auth: %w", err)
	}
	return &Client{
		owner:    owner,
		repo:     repo,
		workflow: workflow,
		token:    token,
		http:     &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// NewWithToken creates a client with an explicit token (useful in tests).
func NewWithToken(owner, repo, workflow, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{owner: owner, repo: repo, workflow: workflow, token: token, http: httpClient}
}

func resolveToken() (string, error) {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t, nil
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("no GITHUB_TOKEN set and gh CLI unavailable: %w", err)
	}
	t := strings.TrimSpace(string(out))
	if t == "" {
		return "", fmt.Errorf("gh auth token returned empty string")
	}
	return t, nil
}

func (c *Client) URL() string {
	if c.workflow != "" {
		return fmt.Sprintf("https://github.com/%s/%s/actions/workflows/%s", c.owner, c.repo, c.workflow)
	}
	return fmt.Sprintf("https://github.com/%s/%s/actions", c.owner, c.repo)
}

func (c *Client) CurrentStatus(ctx context.Context) (provider.Run, error) {
	runs, err := c.RecentRuns(ctx, 1)
	if err != nil {
		return provider.Run{}, err
	}
	if len(runs) == 0 {
		return provider.Run{Status: provider.StatusIdle}, nil
	}
	return runs[0], nil
}

func (c *Client) RecentRuns(ctx context.Context, n int) ([]provider.Run, error) {
	var url string
	if c.workflow != "" {
		url = fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/runs?per_page=%d", apiBase, c.owner, c.repo, c.workflow, n)
	} else {
		url = fmt.Sprintf("%s/repos/%s/%s/actions/runs?per_page=%d", apiBase, c.owner, c.repo, n)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	setGHHeaders(req, c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching runs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	var payload struct {
		WorkflowRuns []struct {
			ID         int64     `json:"id"`
			HeadBranch string    `json:"head_branch"`
			HeadSHA    string    `json:"head_sha"`
			Status     string    `json:"status"`
			Conclusion string    `json:"conclusion"`
			CreatedAt  time.Time `json:"created_at"`
			HTMLURL    string    `json:"html_url"`
		} `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	runs := make([]provider.Run, 0, len(payload.WorkflowRuns))
	for _, r := range payload.WorkflowRuns {
		runID := r.ID // capture for closure
		runs = append(runs, provider.Run{
			ID:        fmt.Sprintf("%d", r.ID),
			Branch:    r.HeadBranch,
			Commit:    shortSHA(r.HeadSHA),
			Status:    mapGHStatus(r.Status, r.Conclusion),
			StartedAt: r.CreatedAt,
			URL:       r.HTMLURL,
			Logs: func(ctx context.Context) (string, error) {
				return c.fetchLogs(ctx, runID)
			},
		})
	}
	return runs, nil
}

func (c *Client) TriggerNew(ctx context.Context) error {
	if c.workflow == "" {
		return fmt.Errorf("workflow file required for new run; add it with `joca add github owner/repo workflow.yml`")
	}
	// Use the last run's branch as ref, falling back to "main".
	ref := "main"
	if runs, err := c.RecentRuns(ctx, 1); err == nil && len(runs) > 0 && runs[0].Branch != "" {
		ref = runs[0].Branch
	}
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/dispatches", apiBase, c.owner, c.repo, c.workflow)
	body := strings.NewReader(fmt.Sprintf(`{"ref":%q}`, ref))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	setGHHeaders(req, c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("dispatching workflow: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusUnprocessableEntity:
		return fmt.Errorf("workflow does not support manual dispatch; add `workflow_dispatch:` to %s", c.workflow)
	default:
		return fmt.Errorf("github API returned %d", resp.StatusCode)
	}
}

func (c *Client) Trigger(ctx context.Context) error {
	// Re-run the latest failed/cancelled run via the re-run API.
	runs, err := c.RecentRuns(ctx, 1)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		return fmt.Errorf("no runs to re-trigger")
	}
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%s/rerun", apiBase, c.owner, c.repo, runs[0].ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	setGHHeaders(req, c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("triggering run: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("github API returned %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) fetchLogs(ctx context.Context, runID int64) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/logs", apiBase, c.owner, c.repo, runID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	setGHHeaders(req, c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching logs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// GitHub returns a redirect to a zip; for now return the URL for the user to open.
	return fmt.Sprintf("Log download URL: %s\nOpen in browser: %s", resp.Header.Get("Location"), c.URL()), nil
}

func setGHHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

func shortSHA(s string) string {
	if len(s) >= 7 {
		return s[:7]
	}
	return s
}

func mapGHStatus(status, conclusion string) provider.Status {
	switch status {
	case "in_progress", "queued":
		return provider.StatusRunning
	case "waiting":
		return provider.StatusApproval
	case "completed":
		switch conclusion {
		case "success":
			return provider.StatusSuccess
		case "failure", "timed_out":
			return provider.StatusFailed
		case "cancelled":
			return provider.StatusCancelled
		default:
			return provider.StatusUnknown
		}
	}
	return provider.StatusUnknown
}
