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
			return &AppConfig{RefreshInterval: DefaultRefreshInterval.String()}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.RefreshInterval = NormalizeRefreshInterval(cfg.RefreshInterval)
	if cfg.AutomationAllowChains == nil {
		v := true
		cfg.AutomationAllowChains = &v
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

// DeletePipeline removes the named pipeline and any automation rules that
// watch or trigger it. It returns the number of automation rules removed.
func DeletePipeline(path, name string) (int, error) {
	cfg, err := Load(path)
	if err != nil {
		return 0, err
	}

	found := false
	pipelines := make([]PipelineEntry, 0, len(cfg.Pipelines))
	for _, pipeline := range cfg.Pipelines {
		if pipeline.Name == name {
			found = true
			continue
		}
		pipelines = append(pipelines, pipeline)
	}
	if !found {
		return 0, fmt.Errorf("pipeline %q not found", name)
	}

	automations := make([]AutomationRule, 0, len(cfg.Automations))
	removedRules := 0
	for _, rule := range cfg.Automations {
		if rule.WatchPipeline == name || rule.TriggerPipeline == name {
			removedRules++
			continue
		}
		automations = append(automations, rule)
	}
	cfg.Pipelines = pipelines
	cfg.Automations = automations
	if err := Save(path, cfg); err != nil {
		return 0, err
	}
	return removedRules, nil
}

// AddAutomation appends rule to the config at path, returning an error if a
// rule with the same name already exists or a cycle would be introduced.
func AddAutomation(path string, rule AutomationRule) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}
	for _, r := range cfg.Automations {
		if r.Name == rule.Name {
			return fmt.Errorf("automation rule %q already exists", rule.Name)
		}
	}
	allowChains := cfg.AllowChains()
	if err := CheckAutomationCycle(cfg.Automations, rule, allowChains); err != nil {
		return err
	}
	cfg.Automations = append(cfg.Automations, rule)
	return Save(path, cfg)
}

// DeleteAutomation removes the rule with the given name from the config at path.
func DeleteAutomation(path string, name string) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}
	found := false
	keep := cfg.Automations[:0]
	for _, r := range cfg.Automations {
		if r.Name == name {
			found = true
		} else {
			keep = append(keep, r)
		}
	}
	if !found {
		return fmt.Errorf("automation rule %q not found", name)
	}
	cfg.Automations = keep
	return Save(path, cfg)
}

// UpdateAutomation replaces the rule matching rule.Name in the config at path.
func UpdateAutomation(path string, rule AutomationRule) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}
	for i, r := range cfg.Automations {
		if r.Name == rule.Name {
			cfg.Automations[i] = rule
			return Save(path, cfg)
		}
	}
	return fmt.Errorf("automation rule %q not found", rule.Name)
}

func parentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
