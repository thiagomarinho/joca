package config_test

import (
	"testing"

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
