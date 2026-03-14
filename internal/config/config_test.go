package config_test

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"

	"github.com/thiagomarinho/joca/internal/config"
)

func TestResolve_setsDir(t *testing.T) {
	var cfg config.Config
	config.Resolve(&cfg)
	if cfg.JocaDir == "" {
		t.Error("Resolve() must set JocaDir")
	}
}

func TestSetup_idempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for range 2 {
		if err := config.Setup(); err != nil {
			t.Fatalf("Setup() error: %v", err)
		}
	}
}

func TestPipelineEntry_PausedRoundtrip(t *testing.T) {
	entry := config.PipelineEntry{Name: "ci", Provider: config.ProviderGitHub, Paused: true}
	data, err := yaml.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got config.PipelineEntry
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Paused {
		t.Error("expected Paused=true after roundtrip")
	}
}

func TestPipelineEntry_PausedOmitted(t *testing.T) {
	entry := config.PipelineEntry{Name: "ci", Provider: config.ProviderGitHub, Paused: false}
	data, err := yaml.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "paused") {
		t.Errorf("expected no 'paused' field when Paused=false, got:\n%s", data)
	}
}

func TestIsInitialised(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var cfg config.Config
	config.Resolve(&cfg)
	if config.IsInitialised(cfg) {
		t.Error("should not be initialised before Setup()")
	}
	if err := config.Setup(); err != nil {
		t.Fatal(err)
	}
	if !config.IsInitialised(cfg) {
		t.Error("should be initialised after Setup()")
	}
}
