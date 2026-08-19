package ui

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
	"github.com/thiagomarinho/joca/internal/credstatus"
	"github.com/thiagomarinho/joca/internal/ui/detail"
	"github.com/thiagomarinho/joca/internal/ui/list"
	"github.com/thiagomarinho/joca/internal/ui/logsearch"
	"github.com/thiagomarinho/joca/internal/ui/savedsearches"
	"github.com/thiagomarinho/joca/internal/ui/settings"
)

func TestSavedLogSearchIsAddedOrReplacedByNameAndScope(t *testing.T) {
	appCfg := &config.AppConfig{SavedLogSearches: []config.SavedLogSearch{{
		Name: "errors", Pipeline: "deploy", Expression: "old", Executions: 10,
	}}}
	m := New(appCfg, config.Config{ConfigFile: filepath.Join(t.TempDir(), "config.yaml")})

	updated := config.SavedLogSearch{Name: "errors", Pipeline: "deploy", Expression: "new", Executions: 20}
	_, cmd := m.Update(logsearch.SaveMsg{Search: updated})
	if cmd == nil {
		t.Fatal("expected saved-search persistence command")
	}
	if len(appCfg.SavedLogSearches) != 1 || appCfg.SavedLogSearches[0].Expression != "new" {
		t.Fatalf("saved searches = %#v", appCfg.SavedLogSearches)
	}

	global := config.SavedLogSearch{Name: "errors", Expression: "global", Executions: 5}
	m.Update(logsearch.SaveMsg{Search: global})
	if len(appCfg.SavedLogSearches) != 2 {
		t.Fatalf("global search should use a separate scope: %#v", appCfg.SavedLogSearches)
	}
}

func TestSavedLogSearchRunsForCurrentPipeline(t *testing.T) {
	search := config.SavedLogSearch{Name: "errors", Expression: "ERROR", Executions: 5}
	appCfg := &config.AppConfig{Pipelines: []config.PipelineEntry{{
		Name: "deploy", Provider: config.ProviderAWS, PipelineName: "deploy", AWSRegion: "us-east-1",
	}}}
	m := New(appCfg, config.Config{})
	m.Update(detail.OpenSavedLogSearchesMsg{Item: list.PipelineItem{Entry: appCfg.Pipelines[0]}})

	_, cmd := m.Update(savedsearches.RunMsg{Pipeline: "deploy", Search: search})
	if cmd == nil {
		t.Fatal("expected saved search to start immediately")
	}
	if _, ok := m.stack[len(m.stack)-1].(logsearch.Model); !ok {
		t.Fatalf("active model = %T, want logsearch.Model", m.stack[len(m.stack)-1])
	}
}

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

func TestPipelineDetailDoesNotShowListScreenFooter(t *testing.T) {
	appCfg := &config.AppConfig{
		Pipelines: []config.PipelineEntry{{
			Name: "pipeline", Provider: config.ProviderAWS,
		}},
	}
	m := New(appCfg, config.Config{})
	item := m.stack[0].(list.Model).Items[0]
	m.Update(list.OpenDetailMsg{Item: item})

	view := m.View()
	if count := strings.Count(view, "/: search"); count != 1 {
		t.Fatalf("detail search shortcut count = %d, want 1:\n%s", count, view)
	}
	if strings.Contains(view, "space: pause") {
		t.Fatal("pipeline list footer should not appear on the detail screen")
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

func TestSettingsSaveUpdatesRuntimeAndConfiguration(t *testing.T) {
	appCfg := &config.AppConfig{RefreshInterval: "30s"}
	m := New(appCfg, config.Config{ConfigFile: filepath.Join(t.TempDir(), "config.yaml")})
	m.stack = append(m.stack, settings.New(*appCfg))

	_, cmd := m.Update(settings.SavedMsg{
		RefreshInterval:   "2m",
		DefaultAWSProfile: "production",
		DefaultAWSRegion:  "ca-central-1",
		LogEditor:         "code-insiders --wait",
	})
	if cmd == nil {
		t.Fatal("expected persistence and timer commands")
	}
	if len(m.stack) != 1 {
		t.Errorf("settings page was not closed; stack length = %d", len(m.stack))
	}
	if m.refreshInterval != 2*time.Minute {
		t.Errorf("runtime refresh interval = %v, want 2m", m.refreshInterval)
	}
	if appCfg.DefaultAWSProfile != "production" || appCfg.DefaultAWSRegion != "ca-central-1" {
		t.Errorf("AWS defaults were not updated: %#v", appCfg)
	}
	if appCfg.LogEditor != "code-insiders --wait" {
		t.Errorf("log editor was not updated: %q", appCfg.LogEditor)
	}

	_, staleCmd := m.Update(tickMsg{generation: 0})
	if staleCmd != nil {
		t.Error("expected stale refresh timer to be ignored")
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

func TestSSOLoginShortcutRemainsAvailableAfterSelectingAnotherPipeline(t *testing.T) {
	appCfg := &config.AppConfig{
		Pipelines: []config.PipelineEntry{
			{Name: "aws", Provider: config.ProviderAWS, AWSProfile: "production"},
			{Name: "github", Provider: config.ProviderGitHub},
		},
	}
	m := New(appCfg, config.Config{})
	m.awsCreds = map[string]credstatus.Status{
		"production": {Err: errors.New("SSO token has expired")},
	}

	// Navigate away from the AWS row. The login action must still be rendered
	// and use the expired profile.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if view := m.View(); !strings.Contains(view, "l: SSO login") {
		t.Fatal("expected SSO login option after selecting a non-AWS pipeline")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if cmd == nil {
		t.Fatal("expected SSO login command after selecting a non-AWS pipeline")
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
