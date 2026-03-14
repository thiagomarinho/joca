package list

import (
	"strings"
	"testing"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/provider"
)

func TestRender_includesBranchRef(t *testing.T) {
	item := PipelineItem{
		Entry: config.PipelineEntry{Name: "my-pipeline", Provider: config.ProviderGitHub},
		Current: provider.Run{
			Branch: "main",
			Commit: "a1b2c3d",
			Status: provider.StatusSuccess,
		},
	}
	out := item.Render(false, 20)
	if !strings.Contains(out, "main@a1b2c3d") {
		t.Errorf("expected branch@commit in output, got: %q", out)
	}
}

func TestRender_branchRefWidth(t *testing.T) {
	item := PipelineItem{
		Entry:   config.PipelineEntry{Name: "p", Provider: config.ProviderGitHub},
		Current: provider.Run{Branch: "very-long-branch-name-exceeds-limit", Commit: "abc1234"},
	}
	out := item.Render(false, 20)
	// branchRef column should be truncated to branchRefWidth visible chars
	ref := renderBranchRef(item.Current.Branch, item.Current.Commit)
	if len(ref) > branchRefWidth+20 { // allow for ANSI codes
		t.Errorf("branchRef too wide: %d", len(ref))
	}
	_ = out
}

func TestRender_emptyBranchRef(t *testing.T) {
	item := PipelineItem{
		Entry:   config.PipelineEntry{Name: "p", Provider: config.ProviderGitHub},
		Current: provider.Run{Status: provider.StatusIdle},
	}
	out := item.Render(false, 20)
	// Should still render without panic
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestRender_runningWithStage(t *testing.T) {
	item := PipelineItem{
		Entry:   config.PipelineEntry{Name: "p", Provider: config.ProviderGitHub},
		Current: provider.Run{Status: provider.StatusRunning, Stage: "Deploy"},
	}
	out := item.Render(false, 20)
	if !strings.Contains(out, "● Deploy") {
		t.Errorf("expected '● Deploy' in output, got: %q", out)
	}
}

func TestRender_runningNoStage(t *testing.T) {
	item := PipelineItem{
		Entry:   config.PipelineEntry{Name: "p", Provider: config.ProviderGitHub},
		Current: provider.Run{Status: provider.StatusRunning},
	}
	out := item.Render(false, 20)
	if !strings.Contains(out, "● running") {
		t.Errorf("expected '● running' in output, got: %q", out)
	}
}

func TestRender_approvalWithStage(t *testing.T) {
	item := PipelineItem{
		Entry:   config.PipelineEntry{Name: "p", Provider: config.ProviderGitHub},
		Current: provider.Run{Status: provider.StatusApproval, Stage: "Deploy"},
	}
	out := item.Render(false, 20)
	if !strings.Contains(out, "⏸ → Deploy") {
		t.Errorf("expected '⏸ → Deploy' in output, got: %q", out)
	}
}

func TestRender_approvalNoStage(t *testing.T) {
	item := PipelineItem{
		Entry:   config.PipelineEntry{Name: "p", Provider: config.ProviderGitHub},
		Current: provider.Run{Status: provider.StatusApproval},
	}
	out := item.Render(false, 20)
	if !strings.Contains(out, "⏸ awaiting approval") {
		t.Errorf("expected '⏸ awaiting approval' in output, got: %q", out)
	}
}

func TestRenderBranchRef(t *testing.T) {
	tests := []struct {
		branch, commit string
		wantContains   string
	}{
		{"main", "a1b2c3d", "main@a1b2c3d"},
		{"main", "", "main"},
		{"", "a1b2c3d", "a1b2c3d"},
		{"", "", ""},
	}
	for _, tt := range tests {
		got := renderBranchRef(tt.branch, tt.commit)
		if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
			t.Errorf("renderBranchRef(%q, %q) = %q, want to contain %q", tt.branch, tt.commit, got, tt.wantContains)
		}
	}
}
