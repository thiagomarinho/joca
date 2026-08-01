package addautomation

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func pipelineNames(count int) []string {
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("pipeline-%d", i)
	}
	return names
}

func sendKey(m Model, key string) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(Model)
}

func TestWatchPipelineViewportKeepsEndCursorVisible(t *testing.T) {
	m := New(pipelineNames(10), nil, true)
	updated, _ := m.Update(tea.WindowSizeMsg{Height: 15}) // three pipeline rows
	m = updated.(Model)
	m = sendKey(m, "end")

	view := m.View()
	if !strings.Contains(view, "pipeline-9") || strings.Contains(view, "pipeline-0") {
		t.Errorf("watch viewport did not scroll to end:\n%s", view)
	}
	if !strings.Contains(view, "[10/10]") {
		t.Errorf("watch viewport missing position counter:\n%s", view)
	}
}

func TestTriggerPipelineSupportsPageNavigationAndSelection(t *testing.T) {
	m := New(pipelineNames(10), nil, true)
	m.height = 15 // three pipeline rows
	m.step = stepTriggerPipeline
	m = sendKey(m, "pgdown")
	if m.triggerCursor != 3 {
		t.Fatalf("trigger cursor after page down = %d, want 3", m.triggerCursor)
	}

	m = sendKey(m, " ")
	if !m.triggerSelected[3] {
		t.Error("expected pipeline 3 to be selected")
	}
	view := m.View()
	if !strings.Contains(view, "[x] pipeline-3") {
		t.Errorf("selected trigger missing from viewport:\n%s", view)
	}
}

func TestPipelineViewportRespondsToHomeAndPageKeys(t *testing.T) {
	m := New(pipelineNames(10), nil, true)
	m.height = 15
	m = sendKey(m, "end")
	m = sendKey(m, "pgup")
	if m.watchCursor != 6 {
		t.Errorf("watch cursor after end and page up = %d, want 6", m.watchCursor)
	}
	m = sendKey(m, "home")
	if m.watchCursor != 0 || m.viewportStart != 0 {
		t.Errorf("home left cursor/start at %d/%d", m.watchCursor, m.viewportStart)
	}
}
