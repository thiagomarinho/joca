package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ProviderKind identifies a CI/CD provider.
type ProviderKind string

const (
	ProviderGitHub ProviderKind = "github"
	ProviderAWS    ProviderKind = "aws"

	DefaultRefreshInterval = 30 * time.Second
	MinRefreshInterval     = 10 * time.Second
)

// EffectiveRefreshInterval returns a safe runtime refresh interval. Invalid
// values use the default, while values below the minimum are clamped.
func EffectiveRefreshInterval(value string) time.Duration {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return DefaultRefreshInterval
	}
	if duration < MinRefreshInterval {
		return MinRefreshInterval
	}
	return duration
}

// NormalizeRefreshInterval preserves valid configured values and replaces
// invalid or too-small values with their safe effective interval.
func NormalizeRefreshInterval(value string) string {
	duration, err := time.ParseDuration(value)
	if err != nil || duration < MinRefreshInterval {
		return EffectiveRefreshInterval(value).String()
	}
	return value
}

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
	RefreshInterval   string           `yaml:"refresh_interval"`
	DefaultAWSProfile string           `yaml:"default_aws_profile,omitempty"`
	DefaultAWSRegion  string           `yaml:"default_aws_region,omitempty"`
	LogEditor         string           `yaml:"log_editor,omitempty"`
	Pipelines         []PipelineEntry  `yaml:"pipelines"`
	Automations       []AutomationRule `yaml:"automations,omitempty"`
	SavedLogSearches  []SavedLogSearch `yaml:"saved_log_searches,omitempty"`
	// AutomationAllowChains controls whether automation rules may form chains
	// (A triggers B, B triggers C). Self-references are always prohibited.
	// Defaults to true when omitted from YAML.
	AutomationAllowChains *bool `yaml:"automation_allow_chains,omitempty"`
}

// SavedLogSearch stores a reusable CodeBuild log-search configuration.
// Pipeline is empty for searches available to every AWS pipeline.
type SavedLogSearch struct {
	Name            string `yaml:"name"`
	Pipeline        string `yaml:"pipeline,omitempty"`
	Expression      string `yaml:"expression"`
	Executions      int    `yaml:"executions"`
	ContextLines    int    `yaml:"context_lines,omitempty"`
	Regex           bool   `yaml:"regex,omitempty"`
	CaseInsensitive bool   `yaml:"case_insensitive,omitempty"`
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
