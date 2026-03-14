package credstatus

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

// Status holds whether credentials are present and their source description.
type Status struct {
	Present bool
	Pending bool   // true while async check is in progress
	Source  string // e.g. "GITHUB_TOKEN", "gh session", `profile "prod"`, "env vars", "~/.aws/credentials"
	Err     error  // non-nil when Present=false; used for diagnostic hints
}

// CheckGitHub reports whether GitHub credentials are available and where they come from.
func CheckGitHub() Status {
	if os.Getenv("GITHUB_TOKEN") != "" {
		return Status{Present: true, Source: "GITHUB_TOKEN"}
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		return Status{Present: true, Source: "gh session"}
	}
	return Status{Present: false}
}

// CheckAWS reports whether AWS credentials are resolvable and where they come from.
// profile is the named AWS profile configured for this pipeline (empty = default chain).
// The SDK is used to verify credentials actually resolve; the source description is
// derived from the environment so we don't depend on SDK-internal provider name strings.
func CheckAWS(profile string) Status {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	opts := []func(*awsconfig.LoadOptions) error{}
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return Status{Present: false, Err: err}
	}
	if _, err := cfg.Credentials.Retrieve(ctx); err != nil {
		return Status{Present: false, Err: err}
	}

	return Status{Present: true, Source: awsSource(profile)}
}

// awsSource returns a human-readable description of where AWS credentials come from,
// using the same priority order as the SDK credential chain.
func awsSource(profile string) string {
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" {
		return "env vars"
	}
	if profile != "" {
		return fmt.Sprintf("profile %q", profile)
	}
	if p := os.Getenv("AWS_PROFILE"); p != "" {
		return fmt.Sprintf("profile %q", p)
	}
	return "~/.aws/credentials"
}
