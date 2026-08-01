package addwizard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/credstatus"
)

func TestLoadAWSProfileSuggestionsMergesConfigAndCredentials(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	credentialsFile := filepath.Join(dir, "credentials")
	if err := os.WriteFile(configFile, []byte(`
[default]
region = us-east-1

[profile production]
sso_session = company
sso_role_name = AdministratorAccess
region = ca-central-1

[sso-session company]
sso_start_url = https://example.awsapps.com/start
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentialsFile, []byte(`
[legacy]
aws_access_key_id = test
region = eu-west-1
`), 0o600); err != nil {
		t.Fatal(err)
	}

	got := loadAWSProfileSuggestions(configFile, credentialsFile)
	if len(got) != 3 {
		t.Fatalf("profiles = %#v, want 3", got)
	}
	if got[0].Name != "default" || got[0].Region != "us-east-1" {
		t.Errorf("default profile = %#v", got[0])
	}
	if got[2].Name != "production" || got[2].Role != "AdministratorAccess" || got[2].Region != "ca-central-1" {
		t.Errorf("production profile = %#v", got[2])
	}
}

func TestLoadAWSProfileSuggestionsIgnoresMissingFiles(t *testing.T) {
	if got := loadAWSProfileSuggestions(filepath.Join(t.TempDir(), "missing")); len(got) != 0 {
		t.Errorf("profiles = %#v, want none", got)
	}
}

func TestApplyAWSProfileSuggestionCompletesProfileAndRegion(t *testing.T) {
	m := Model{
		awsProfile: "pro",
		awsProfiles: []awsProfileSuggestion{
			{Name: "development", Region: "us-west-2"},
			{Name: "production", Role: "AdministratorAccess", Region: "ca-central-1"},
		},
	}
	suggestions := m.awsCredentialSuggestions()
	if len(suggestions) != 1 || suggestions[0].value != "production" {
		t.Fatalf("suggestions = %#v", suggestions)
	}
	if suggestions[0].detail != "role: AdministratorAccess  region: ca-central-1" {
		t.Errorf("suggestion detail = %q", suggestions[0].detail)
	}

	m = m.applyAWSSuggestion()
	if m.awsProfile != "production" {
		t.Errorf("profile = %q, want production", m.awsProfile)
	}
	if m.awsRegion != "ca-central-1" {
		t.Errorf("region = %q, want ca-central-1", m.awsRegion)
	}
}

func TestAWSRegionSuggestionsFilterByPrefix(t *testing.T) {
	m := Model{awsCredField: 1, awsRegion: "ca-"}
	suggestions := m.awsCredentialSuggestions()
	if len(suggestions) == 0 {
		t.Fatal("expected Canadian region suggestions")
	}
	for _, suggestion := range suggestions {
		if len(suggestion.value) < 3 || suggestion.value[:3] != "ca-" {
			t.Errorf("unexpected region suggestion %q", suggestion.value)
		}
	}
}

func TestEnterAppliesSelectedProfileBeforeCheckingCredentials(t *testing.T) {
	m := Model{
		step: stepAWSCredentials,
		awsProfiles: []awsProfileSuggestion{
			{Name: "development", Region: "us-west-2"},
			{Name: "production", Region: "ca-central-1"},
		},
		awsSuggestion: 1,
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected first enter to apply suggestion without checking credentials")
	}
	m = updated.(Model)
	if m.awsProfile != "production" || m.awsRegion != "ca-central-1" {
		t.Errorf("selected profile was not applied: profile=%q region=%q", m.awsProfile, m.awsRegion)
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected second enter to start credential check")
	}
}

func TestCredentialTimeoutIsNotReportedAsMissingCredentials(t *testing.T) {
	m := Model{step: stepAWSCredentials, awsProfile: "default", awsChecking: true}
	updated, _ := m.Update(awsCredCheckMsg{status: credstatus.Status{Err: context.DeadlineExceeded}})
	m = updated.(Model)

	if !strings.Contains(m.err, "credential check timed out") {
		t.Errorf("error = %q, want timeout diagnosis", m.err)
	}
	if strings.Contains(m.err, "no credentials found") {
		t.Errorf("timeout was misreported as missing credentials: %q", m.err)
	}
	if strings.Contains(strings.ToLower(m.err), "sso") {
		t.Errorf("generic credential timeout assumed SSO: %q", m.err)
	}
}
