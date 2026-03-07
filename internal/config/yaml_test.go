package config_test

import (
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

func TestSaveAndLoad_roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := &config.AppConfig{
		RefreshInterval: "60s",
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
