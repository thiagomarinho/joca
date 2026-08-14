package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thiagomarinho/joca/internal/config"
)

func TestLoad_missingFile_returnsDefaults(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.RefreshInterval != "30s" {
		t.Errorf("expected default refresh_interval=30s, got %q", cfg.RefreshInterval)
	}
}

func TestLoad_clampsRefreshIntervalToMinimum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("refresh_interval: 2s\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.RefreshInterval != config.MinRefreshInterval.String() {
		t.Errorf("refresh_interval = %q, want %q", cfg.RefreshInterval, config.MinRefreshInterval)
	}
}

func TestLoad_invalidRefreshIntervalUsesDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("refresh_interval: immediately\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.RefreshInterval != config.DefaultRefreshInterval.String() {
		t.Errorf("refresh_interval = %q, want %q", cfg.RefreshInterval, config.DefaultRefreshInterval)
	}
}

func TestSaveAndLoad_roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := &config.AppConfig{
		RefreshInterval:   "60s",
		DefaultAWSProfile: "production",
		DefaultAWSRegion:  "ca-central-1",
		LogEditor:         "code-insiders --wait",
		Pipelines: []config.PipelineEntry{
			{Name: "my-api", Provider: config.ProviderGitHub, Owner: "acme", Repo: "my-api"},
			{Name: "infra", Provider: config.ProviderAWS, PipelineName: "infra-deploy", AWSRegion: "us-east-1"},
		},
	}
	if err := config.Save(path, original); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.RefreshInterval != "60s" {
		t.Errorf("refresh_interval: got %q, want 60s", loaded.RefreshInterval)
	}
	if loaded.DefaultAWSProfile != "production" {
		t.Errorf("default_aws_profile: got %q, want production", loaded.DefaultAWSProfile)
	}
	if loaded.DefaultAWSRegion != "ca-central-1" {
		t.Errorf("default_aws_region: got %q, want ca-central-1", loaded.DefaultAWSRegion)
	}
	if loaded.LogEditor != "code-insiders --wait" {
		t.Errorf("log_editor: got %q, want code-insiders --wait", loaded.LogEditor)
	}
	if len(loaded.Pipelines) != 2 {
		t.Fatalf("expected 2 pipelines, got %d", len(loaded.Pipelines))
	}
	if loaded.Pipelines[0].Name != "my-api" {
		t.Errorf("pipeline[0].Name: got %q, want my-api", loaded.Pipelines[0].Name)
	}
}

func TestAddPipeline_appendsEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	entry := config.PipelineEntry{Name: "new-pipe", Provider: config.ProviderGitHub, Owner: "acme", Repo: "new-pipe"}
	if err := config.AddPipeline(path, entry); err != nil {
		t.Fatalf("AddPipeline() error: %v", err)
	}
	cfg, _ := config.Load(path)
	if len(cfg.Pipelines) != 1 || cfg.Pipelines[0].Name != "new-pipe" {
		t.Errorf("unexpected pipelines: %+v", cfg.Pipelines)
	}
}

func TestAddPipeline_duplicateReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	entry := config.PipelineEntry{Name: "dup", Provider: config.ProviderGitHub, Owner: "a", Repo: "b"}
	_ = config.AddPipeline(path, entry)
	if err := config.AddPipeline(path, entry); err == nil {
		t.Error("expected error for duplicate pipeline name, got nil")
	}
}

func TestDeletePipelineRemovesPipelineAndReferencingAutomations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.AppConfig{
		Pipelines: []config.PipelineEntry{
			{Name: "build", Provider: config.ProviderGitHub},
			{Name: "deploy", Provider: config.ProviderAWS},
		},
		Automations: []config.AutomationRule{
			{Name: "build then deploy", WatchPipeline: "build", TriggerPipeline: "deploy"},
			{Name: "deploy then notify", WatchPipeline: "deploy", TriggerPipeline: "notify"},
			{Name: "unrelated", WatchPipeline: "lint", TriggerPipeline: "notify"},
		},
	}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	removedRules, err := config.DeletePipeline(path, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if removedRules != 2 {
		t.Errorf("removed automation rules = %d, want 2", removedRules)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pipelines) != 1 || got.Pipelines[0].Name != "build" {
		t.Errorf("pipelines after delete = %#v", got.Pipelines)
	}
	if len(got.Automations) != 1 || got.Automations[0].Name != "unrelated" {
		t.Errorf("automations after delete = %#v", got.Automations)
	}
}

func TestDeletePipelineReturnsErrorWhenNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(path, &config.AppConfig{
		Pipelines: []config.PipelineEntry{{Name: "build", Provider: config.ProviderGitHub}},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := config.DeletePipeline(path, "missing"); err == nil {
		t.Fatal("expected missing pipeline error")
	}
}
