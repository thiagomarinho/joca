package settings

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
)

func TestNewPrefillsConfiguration(t *testing.T) {
	m := New(config.AppConfig{
		RefreshInterval:   "45s",
		DefaultAWSProfile: "production",
		DefaultAWSRegion:  "ca-central-1",
		LogEditor:         "code-insiders --wait",
	})
	if m.values[fieldRefreshInterval] != "45s" || m.values[fieldDefaultAWSProfile] != "production" || m.values[fieldDefaultAWSRegion] != "ca-central-1" || m.values[fieldLogEditor] != "code-insiders --wait" {
		t.Errorf("settings were not prefilled: %#v", m.values)
	}
}

func TestSaveRejectsRefreshIntervalBelowMinimum(t *testing.T) {
	m := New(config.AppConfig{RefreshInterval: "9s"})
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected interval below minimum to prevent saving")
	}
	if err := updated.(Model).err; !strings.Contains(err, "at least 10s") {
		t.Fatalf("validation error = %q, want minimum interval message", err)
	}
}

func TestSaveEmitsValidatedSettings(t *testing.T) {
	m := New(config.AppConfig{RefreshInterval: "2m", DefaultAWSProfile: "prod", DefaultAWSRegion: "us-east-1", LogEditor: "zed --wait"})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected save command")
	}
	msg := cmd().(SavedMsg)
	if msg.RefreshInterval != "2m" || msg.DefaultAWSProfile != "prod" || msg.DefaultAWSRegion != "us-east-1" || msg.LogEditor != "zed --wait" {
		t.Errorf("saved settings = %#v", msg)
	}
}

func TestSaveRejectsInvalidRefreshInterval(t *testing.T) {
	m := New(config.AppConfig{RefreshInterval: "not-a-duration"})
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected invalid interval to prevent saving")
	}
	if updated.(Model).err == "" {
		t.Fatal("expected validation error")
	}
}
