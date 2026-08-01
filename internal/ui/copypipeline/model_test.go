package copypipeline

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
)

func TestNewPrefillsAWSFieldsAndSavesUnpausedCopy(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	source := config.PipelineEntry{
		Name:         "deploy",
		Provider:     config.ProviderAWS,
		PipelineName: "deploy-prod",
		AWSProfile:   "production",
		AWSRegion:    "ca-central-1",
		Paused:       true,
	}
	m := New(configFile, source)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected save command, form error: %q", updated.(Model).err)
	}
	msg := cmd().(SavedMsg)
	if msg.Entry.Name != "deploy copy" {
		t.Errorf("name = %q, want %q", msg.Entry.Name, "deploy copy")
	}
	if msg.Entry.PipelineName != source.PipelineName || msg.Entry.AWSProfile != source.AWSProfile || msg.Entry.AWSRegion != source.AWSRegion {
		t.Errorf("AWS fields were not preserved: %#v", msg.Entry)
	}
	if msg.Entry.Paused {
		t.Error("expected copied pipeline to start unpaused")
	}
}

func TestNewPrefillsGitHubFields(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	source := config.PipelineEntry{
		Name:     "CI",
		Provider: config.ProviderGitHub,
		Owner:    "acme",
		Repo:     "widgets",
		Workflow: "ci.yml",
	}
	m := New(configFile, source)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected save command")
	}
	entry := cmd().(SavedMsg).Entry
	if entry.Owner != source.Owner || entry.Repo != source.Repo || entry.Workflow != source.Workflow {
		t.Errorf("GitHub fields were not preserved: %#v", entry)
	}
}

func TestSaveRejectsExistingDisplayName(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	source := config.PipelineEntry{Name: "CI", Provider: config.ProviderGitHub, Owner: "acme", Repo: "widgets"}
	if err := config.AddPipeline(configFile, config.PipelineEntry{Name: "CI copy", Provider: config.ProviderGitHub, Owner: "acme", Repo: "other"}); err != nil {
		t.Fatal(err)
	}
	m := New(configFile, source)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected existing name to prevent saving")
	}
	if updated.(Model).err == "" {
		t.Fatal("expected existing-name validation error")
	}
}
