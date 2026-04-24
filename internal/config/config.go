package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ProviderKind identifies a CI/CD provider.
type ProviderKind string

const (
	ProviderGitHub ProviderKind = "github"
	ProviderAWS    ProviderKind = "aws"
)

// PipelineEntry describes a single pipeline to track.
type PipelineEntry struct {
	Name     string       `yaml:"name"`
	Provider ProviderKind `yaml:"provider"`

	// GitHub-specific
	Owner    string `yaml:"owner,omitempty"`
	Repo     string `yaml:"repo,omitempty"`
	Workflow string `yaml:"workflow,omitempty"` // e.g. ci.yml; empty = all workflows

	// AWS-specific
	PipelineName string `yaml:"pipeline_name,omitempty"`
	AWSProfile   string `yaml:"aws_profile,omitempty"`
	AWSRegion    string `yaml:"aws_region,omitempty"`

	Paused bool `yaml:"paused,omitempty"`
}

// AppConfig holds the persistent user configuration stored in ~/.joca/config.yaml.
type AppConfig struct {
	RefreshInterval string           `yaml:"refresh_interval"`
	Pipelines       []PipelineEntry  `yaml:"pipelines"`
	Automations     []AutomationRule `yaml:"automations,omitempty"`
	// AutomationAllowChains controls whether automation rules may form chains
	// (A triggers B, B triggers C). Self-references are always prohibited.
	// Defaults to true when omitted from YAML.
	AutomationAllowChains *bool `yaml:"automation_allow_chains,omitempty"`
}

// AllowChains reports whether automation rule chaining is permitted.
// Returns true when the field is unset (default).
func (c *AppConfig) AllowChains() bool {
	return c.AutomationAllowChains == nil || *c.AutomationAllowChains
}

// Config holds values derived from global flags and environment.
// It is populated by root.go's cobra.OnInitialize hook.
type Config struct {
	Dir     string // --dir flag value
	Verbose bool
	Debug   bool

	// Resolved paths — set by Resolve(), not by flags.
	JocaDir    string // ~/.joca
	ConfigFile string // ~/.joca/config.yaml
}

// Resolve fills in the path fields of cfg using the user's home directory.
// It does not create directories — that is Setup()'s job.
func Resolve(cfg *Config) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	cfg.JocaDir = filepath.Join(home, ".joca")
	cfg.ConfigFile = filepath.Join(cfg.JocaDir, "config.yaml")
}

// Setup creates the ~/.joca directory tree. Idempotent.
func Setup() error {
	var cfg Config
	Resolve(&cfg)
	if cfg.JocaDir == "" {
		return fmt.Errorf("could not determine home directory")
	}
	if err := os.MkdirAll(cfg.JocaDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", cfg.JocaDir, err)
	}
	return nil
}

// IsInitialised returns true if the ~/.joca directory exists.
func IsInitialised(cfg Config) bool {
	_, err := os.Stat(cfg.JocaDir)
	return err == nil
}
