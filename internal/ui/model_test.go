package ui

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/credstatus"
	"github.com/thiagomarinho/joca/internal/ui/list"
)

func TestTogglePauseWhilePausedPipelinesHiddenClampsCursor(t *testing.T) {
	appCfg := &config.AppConfig{
		Pipelines: []config.PipelineEntry{
			{Name: "first"},
			{Name: "second"},
		},
	}
	m := New(appCfg, config.Config{ConfigFile: filepath.Join(t.TempDir(), "config.yaml")})

	listView := m.stack[0].(list.Model)
	listView.HidePaused = true
	updated, _ := listView.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.stack[0] = updated

	m.Update(list.TogglePauseMsg{Index: 1})

	// View asks the list for its selected pipeline to render provider-specific
	// shortcuts. It must remain safe after the selected row disappears.
	m.View()

	listView = m.stack[0].(list.Model)
	selected, ok := listView.Selected()
	if !ok {
		t.Fatal("expected the remaining visible pipeline to be selected")
	}
	if selected.Entry.Name != "first" {
		t.Errorf("selected pipeline = %q, want %q", selected.Entry.Name, "first")
	}
}

func TestFocusResumesSelectedPipelineAndPausesOthers(t *testing.T) {
	appCfg := &config.AppConfig{
		Pipelines: []config.PipelineEntry{
			{Name: "first", Paused: false},
			{Name: "second", Paused: true},
			{Name: "third", Paused: false},
		},
	}
	m := New(appCfg, config.Config{ConfigFile: filepath.Join(t.TempDir(), "config.yaml")})

	_, cmd := m.Update(list.FocusMsg{Index: 1})
	if cmd == nil {
		t.Fatal("expected focus persistence and refresh commands")
	}
	for i, pipeline := range m.appCfg.Pipelines {
		wantPaused := i != 1
		if pipeline.Paused != wantPaused {
			t.Errorf("pipeline %d paused = %v, want %v", i, pipeline.Paused, wantPaused)
		}
	}
	if m.globalPaused {
		t.Error("focused state must not be globally paused")
	}
	listView := m.stack[0].(list.Model)
	if !listView.HidePaused {
		t.Error("expected focus to enable hide-paused mode")
	}
	item, ok := listView.Selected()
	if !ok || item.Entry.Name != "second" {
		t.Fatalf("focused list selection = %#v, %v", item, ok)
	}
}

func TestAWSLoginCommandUsesSelectedProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    []string
	}{
		{name: "named profile", profile: "production", want: []string{"aws", "sso", "login", "--profile", "production"}},
		{name: "default profile", want: []string{"aws", "sso", "login"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := awsSSOLoginCommand(tt.profile)
			if !reflect.DeepEqual(cmd.Args, tt.want) {
				t.Errorf("command args = %q, want %q", cmd.Args, tt.want)
			}
		})
	}
}

func TestSSOLoginShortcutAvailableForSelectedExpiredAWSProfile(t *testing.T) {
	appCfg := &config.AppConfig{
		Pipelines: []config.PipelineEntry{
			{Name: "pipeline", Provider: config.ProviderGitHub},
		},
	}
	m := New(appCfg, config.Config{})

	appCfg.Pipelines[0].Provider = config.ProviderAWS
	appCfg.Pipelines[0].AWSProfile = "production"
	listView := m.stack[0].(list.Model)
	listView.Items[0].Entry = appCfg.Pipelines[0]
	m.stack[0] = listView
	m.awsCreds = map[string]credstatus.Status{
		"production": {Err: errors.New("SSO token has expired")},
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if cmd == nil {
		t.Fatal("expected SSO login command for selected expired AWS profile")
	}
	if m.statusMsg != "Opening AWS SSO login for profile \"production\"…" {
		t.Errorf("status message = %q", m.statusMsg)
	}
}

func TestSSOLoginShortcutIgnoredWithoutExpiredSSOCredentials(t *testing.T) {
	appCfg := &config.AppConfig{
		Pipelines: []config.PipelineEntry{
			{Name: "pipeline", Provider: config.ProviderGitHub},
		},
	}
	m := New(appCfg, config.Config{})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if cmd != nil {
		t.Fatal("expected no command when selected pipeline has no expired AWS SSO credentials")
	}
}
