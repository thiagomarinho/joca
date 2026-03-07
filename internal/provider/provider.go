package provider

import (
	"context"
	"strings"
	"time"
)

// Status represents the current state of a pipeline run.
type Status string

const (
	StatusRunning  Status = "running"
	StatusSuccess  Status = "success"
	StatusFailed   Status = "failed"
	StatusPending  Status = "pending"
	StatusApproval Status = "awaiting approval"
	StatusIdle     Status = "idle"
	StatusUnknown  Status = "unknown"
)

// Run describes a single pipeline execution.
type Run struct {
	ID        string
	Branch    string
	Status    Status
	StartedAt time.Time
	URL       string
	// Logs is a lazy loader for log output. May be nil if not supported.
	Logs func(ctx context.Context) (string, error)
}

// IsCredentialError reports whether err is likely caused by missing or invalid
// credentials rather than a transient network or API error.
func IsCredentialError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "github_token") ||
		strings.Contains(msg, "gh cli") ||
		strings.Contains(msg, "nocredentialproviders") ||
		strings.Contains(msg, "no credential") ||
		strings.Contains(msg, "failed to refresh cached credentials") ||
		strings.Contains(msg, "unable to find credentials")
}

// Provider is the abstraction over a CI/CD backend.
type Provider interface {
	// CurrentStatus returns the most recent run.
	CurrentStatus(ctx context.Context) (Run, error)
	// RecentRuns returns the last n runs (used for history dots).
	RecentRuns(ctx context.Context, n int) ([]Run, error)
	// Trigger re-runs the latest execution.
	Trigger(ctx context.Context) error
	// URL returns the web URL for this pipeline.
	URL() string
}
