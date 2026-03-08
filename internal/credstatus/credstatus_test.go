package credstatus_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thiagomarinho/joca/internal/credstatus"
)

func TestCheckGitHub_Token(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_testtoken")
	s := credstatus.CheckGitHub()
	if !s.Present {
		t.Fatal("expected Present=true when GITHUB_TOKEN is set")
	}
	if s.Source != "GITHUB_TOKEN" {
		t.Fatalf("expected Source=%q, got %q", "GITHUB_TOKEN", s.Source)
	}
}

func TestCheckGitHub_NoCredentials(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	s := credstatus.CheckGitHub()
	if !s.Present && s.Source != "" {
		t.Fatalf("expected empty Source when not present, got %q", s.Source)
	}
}

func TestCheckAWS_EnvVars(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
	s := credstatus.CheckAWS("")
	if !s.Present {
		t.Fatal("expected Present=true when AWS env vars are set")
	}
	if s.Source != "env vars" {
		t.Fatalf("expected Source=%q, got %q", "env vars", s.Source)
	}
}

func TestCheckAWS_NamedProfile(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_PROFILE", "")

	dir := t.TempDir()
	credFile := filepath.Join(dir, "credentials")
	if err := os.WriteFile(credFile, []byte("[myprofile]\naws_access_key_id = AKID\naws_secret_access_key = SECRET\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credFile)
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")

	s := credstatus.CheckAWS("myprofile")
	if !s.Present {
		t.Fatal("expected Present=true when named profile exists in credentials file")
	}
	if s.Source != `profile "myprofile"` {
		t.Fatalf("expected Source=%q, got %q", `profile "myprofile"`, s.Source)
	}
}

func TestCheckAWS_EnvProfile(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_PROFILE", "staging")

	dir := t.TempDir()
	credFile := filepath.Join(dir, "credentials")
	if err := os.WriteFile(credFile, []byte("[staging]\naws_access_key_id = AKID\naws_secret_access_key = SECRET\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credFile)
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")

	s := credstatus.CheckAWS("")
	if !s.Present {
		t.Fatal("expected Present=true when AWS_PROFILE env is set and profile exists")
	}
	if s.Source != `profile "staging"` {
		t.Fatalf("expected Source=%q, got %q", `profile "staging"`, s.Source)
	}
}

func TestCheckAWS_NoCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
	s := credstatus.CheckAWS("")
	if s.Present {
		t.Fatal("expected Present=false when no credentials are available")
	}
	if s.Source != "" {
		t.Fatalf("expected empty Source when not present, got %q", s.Source)
	}
}
