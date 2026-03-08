package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/thiagomarinho/joca/internal/provider"
)

// Repo is a GitHub repository visible to the authenticated user.
type Repo struct {
	Owner    string
	Name     string
	FullName string // "owner/name"
}

// WorkflowInfo describes a GitHub Actions workflow with its last-run status.
type WorkflowInfo struct {
	ID         int64
	Name       string
	Filename   string // e.g. "ci.yml"
	LastStatus provider.Status
}

// ResolveToken returns the GitHub token from the environment or gh CLI.
func ResolveToken() (string, error) {
	return resolveToken()
}

// ListUserRepos fetches all repositories visible to the token, sorted by last update.
func ListUserRepos(ctx context.Context, token string, httpClient *http.Client) ([]Repo, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	url := apiBase + "/user/repos?per_page=100&sort=updated&type=all"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	setGHHeaders(req, token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching repos: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	var raw []struct {
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding repos: %w", err)
	}

	repos := make([]Repo, 0, len(raw))
	for _, r := range raw {
		parts := strings.SplitN(r.FullName, "/", 2)
		if len(parts) != 2 {
			continue
		}
		repos = append(repos, Repo{
			Owner:    parts[0],
			Name:     parts[1],
			FullName: r.FullName,
		})
	}
	return repos, nil
}

// ListWorkflows fetches all workflows for a repo and the last-run status of each.
func ListWorkflows(ctx context.Context, owner, repo, token string, httpClient *http.Client) ([]WorkflowInfo, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}

	// 1. Fetch workflow definitions.
	wfURL := fmt.Sprintf("%s/repos/%s/%s/actions/workflows", apiBase, owner, repo)
	wfReq, err := http.NewRequestWithContext(ctx, http.MethodGet, wfURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	setGHHeaders(wfReq, token)

	wfResp, err := httpClient.Do(wfReq)
	if err != nil {
		return nil, fmt.Errorf("fetching workflows: %w", err)
	}
	defer func() { _ = wfResp.Body.Close() }()
	if wfResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d for workflows", wfResp.StatusCode)
	}

	var wfPayload struct {
		Workflows []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Path string `json:"path"` // e.g. ".github/workflows/ci.yml"
		} `json:"workflows"`
	}
	if err := json.NewDecoder(wfResp.Body).Decode(&wfPayload); err != nil {
		return nil, fmt.Errorf("decoding workflows: %w", err)
	}

	// 2. Fetch recent runs to determine last status per workflow.
	latestStatus := map[int64]provider.Status{}
	runsURL := fmt.Sprintf("%s/repos/%s/%s/actions/runs?per_page=100", apiBase, owner, repo)
	runsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, runsURL, nil)
	if err == nil {
		setGHHeaders(runsReq, token)
		if runsResp, err := httpClient.Do(runsReq); err == nil {
			defer func() { _ = runsResp.Body.Close() }()
			if runsResp.StatusCode == http.StatusOK {
				var runsPayload struct {
					WorkflowRuns []struct {
						WorkflowID int64  `json:"workflow_id"`
						Status     string `json:"status"`
						Conclusion string `json:"conclusion"`
					} `json:"workflow_runs"`
				}
				if err := json.NewDecoder(runsResp.Body).Decode(&runsPayload); err == nil {
					for _, r := range runsPayload.WorkflowRuns {
						if _, seen := latestStatus[r.WorkflowID]; !seen {
							latestStatus[r.WorkflowID] = mapGHStatus(r.Status, r.Conclusion)
						}
					}
				}
			}
		}
	}

	// 3. Build result slice.
	infos := make([]WorkflowInfo, 0, len(wfPayload.Workflows))
	for _, wf := range wfPayload.Workflows {
		// Extract bare filename from path like ".github/workflows/ci.yml".
		parts := strings.Split(wf.Path, "/")
		filename := parts[len(parts)-1]

		status := provider.StatusIdle
		if s, ok := latestStatus[wf.ID]; ok {
			status = s
		}
		infos = append(infos, WorkflowInfo{
			ID:         wf.ID,
			Name:       wf.Name,
			Filename:   filename,
			LastStatus: status,
		})
	}
	return infos, nil
}
