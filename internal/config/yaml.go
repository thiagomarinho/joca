package config

import (
	"errors"
	"fmt"
	"os"

	"go.yaml.in/yaml/v4"
)

// Load reads the AppConfig from path. If the file does not exist, an empty
// AppConfig with sensible defaults is returned.
func Load(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &AppConfig{RefreshInterval: "30s"}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.RefreshInterval == "" {
		cfg.RefreshInterval = "30s"
	}
	return &cfg, nil
}

// Save writes cfg to path, creating parent directories as needed.
func Save(path string, cfg *AppConfig) error {
	if err := os.MkdirAll(parentDir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// AddPipeline appends entry to the config at path, returning an error if a
// pipeline with the same name already exists.
func AddPipeline(path string, entry PipelineEntry) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}
	for _, p := range cfg.Pipelines {
		if p.Name == entry.Name {
			return fmt.Errorf("pipeline %q already exists in config", entry.Name)
		}
	}
	cfg.Pipelines = append(cfg.Pipelines, entry)
	return Save(path, cfg)
}

func parentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
