package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gh "github.com/thiagomarinho/joca/internal/provider/github"
)

func makeServer(t *testing.T, runs []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"workflow_runs": runs})
	}))
}

func TestRecentRuns_parsesResponse(t *testing.T) {
	srv := makeServer(t, []map[string]any{
		{
			"id":          int64(1),
			"head_branch": "main",
			"head_sha":    "a1b2c3d4e5f6789",
			"status":      "completed",
			"conclusion":  "success",
			"created_at":  time.Now().Format(time.RFC3339),
			"html_url":    "https://github.com/a/b/actions/runs/1",
		},
		{
			"id":          int64(2),
			"head_branch": "feat",
			"head_sha":    "deadbeef1234567",
			"status":      "completed",
			"conclusion":  "failure",
			"created_at":  time.Now().Add(-time.Hour).Format(time.RFC3339),
			"html_url":    "https://github.com/a/b/actions/runs/2",
		},
	})
	defer srv.Close()

	// Swap the API base by using a custom transport that rewrites the host.
	transport := rewriteTransport(srv.URL)
	client := gh.NewWithToken("owner", "repo", "", "tok", &http.Client{Transport: transport})

	runs, err := client.RecentRuns(context.Background(), 2)
	if err != nil {
		t.Fatalf("RecentRuns() error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	if runs[0].Branch != "main" {
		t.Errorf("run[0].Branch = %q, want main", runs[0].Branch)
	}
	if runs[0].Commit != "a1b2c3d" {
		t.Errorf("run[0].Commit = %q, want a1b2c3d", runs[0].Commit)
	}
}

func TestCurrentStatus_empty_returnsIdle(t *testing.T) {
	srv := makeServer(t, nil)
	defer srv.Close()
	transport := rewriteTransport(srv.URL)
	client := gh.NewWithToken("owner", "repo", "", "tok", &http.Client{Transport: transport})

	run, err := client.CurrentStatus(context.Background())
	if err != nil {
		t.Fatalf("CurrentStatus() error: %v", err)
	}
	if run.Status != "idle" {
		t.Errorf("expected idle, got %q", run.Status)
	}
}

func TestCurrentStatus_running_fetchesJobStage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"workflow_runs": []map[string]any{
				{
					"id":          int64(42),
					"head_branch": "main",
					"head_sha":    "a1b2c3d4e5f6789",
					"status":      "in_progress",
					"conclusion":  "",
					"created_at":  time.Now().Format(time.RFC3339),
					"html_url":    "https://github.com/owner/repo/actions/runs/42",
				},
			},
		})
	})
	mux.HandleFunc("/repos/owner/repo/actions/runs/42/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jobs": []map[string]any{
				{"name": "Build", "status": "in_progress"},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	transport := rewriteTransport(srv.URL)
	client := gh.NewWithToken("owner", "repo", "", "tok", &http.Client{Transport: transport})

	run, err := client.CurrentStatus(context.Background())
	if err != nil {
		t.Fatalf("CurrentStatus() error: %v", err)
	}
	if run.Stage != "Build" {
		t.Errorf("Stage = %q, want %q", run.Stage, "Build")
	}
}

func TestCurrentStatus_approval_fetchesJobStage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"workflow_runs": []map[string]any{
				{
					"id":          int64(99),
					"head_branch": "main",
					"head_sha":    "a1b2c3d4e5f6789",
					"status":      "waiting",
					"conclusion":  "",
					"created_at":  time.Now().Format(time.RFC3339),
					"html_url":    "https://github.com/owner/repo/actions/runs/99",
				},
			},
		})
	})
	mux.HandleFunc("/repos/owner/repo/actions/runs/99/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jobs": []map[string]any{
				{"name": "Deploy to prod", "status": "waiting"},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	transport := rewriteTransport(srv.URL)
	client := gh.NewWithToken("owner", "repo", "", "tok", &http.Client{Transport: transport})

	run, err := client.CurrentStatus(context.Background())
	if err != nil {
		t.Fatalf("CurrentStatus() error: %v", err)
	}
	if run.Stage != "Deploy to prod" {
		t.Errorf("Stage = %q, want %q", run.Stage, "Deploy to prod")
	}
}

// rewriteTransport returns an http.RoundTripper that rewrites all requests to baseURL.
type rewriteTransport string

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(string(rt), "http://")
	return http.DefaultTransport.RoundTrip(req)
}
