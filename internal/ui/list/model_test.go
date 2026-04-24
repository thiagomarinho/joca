package list

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thiagomarinho/joca/internal/config"
)

func makeTestModel(names ...string) Model {
	items := make([]PipelineItem, len(names))
	for i, n := range names {
		items[i] = PipelineItem{Entry: config.PipelineEntry{Name: n}}
	}
	return New(items)
}

func sendKeyMsg(m Model, key string) (Model, MoveItemMsg, bool) {
	updated, cmd := m.Update(keyMsgFromString(key))
	listModel, _ := updated.(Model)
	if cmd == nil {
		return listModel, MoveItemMsg{}, false
	}
	result := cmd()
	move, ok := result.(MoveItemMsg)
	return listModel, move, ok
}

// keyMsgFromString creates a tea.KeyMsg that will match msg.String() == key.
// For "shift+up"/"shift+down" we use the KeyShiftUp/KeyShiftDown types.
func keyMsgFromString(key string) tea.KeyMsg {
	switch key {
	case "shift+up":
		return tea.KeyMsg{Type: tea.KeyShiftUp}
	case "shift+down":
		return tea.KeyMsg{Type: tea.KeyShiftDown}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

func setCursor(m Model, pos int) Model {
	for i := 0; i < pos; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m, _ = updated.(Model)
	}
	return m
}

func TestMove_up(t *testing.T) {
	m := makeTestModel("a", "b", "c")
	m = setCursor(m, 1) // cursor at index 1

	m2, move, ok := sendKeyMsg(m, "shift+up")
	if !ok {
		t.Fatal("expected MoveItemMsg, got none")
	}
	if move.From != 1 || move.To != 0 {
		t.Errorf("expected From=1 To=0, got From=%d To=%d", move.From, move.To)
	}
	if m2.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m2.cursor)
	}
	if m2.Items[0].Entry.Name != "b" || m2.Items[1].Entry.Name != "a" {
		t.Errorf("expected items [b a c], got [%s %s %s]",
			m2.Items[0].Entry.Name, m2.Items[1].Entry.Name, m2.Items[2].Entry.Name)
	}
}

func TestMove_down(t *testing.T) {
	m := makeTestModel("a", "b", "c")
	m = setCursor(m, 1) // cursor at index 1

	m2, move, ok := sendKeyMsg(m, "shift+down")
	if !ok {
		t.Fatal("expected MoveItemMsg, got none")
	}
	if move.From != 1 || move.To != 2 {
		t.Errorf("expected From=1 To=2, got From=%d To=%d", move.From, move.To)
	}
	if m2.cursor != 2 {
		t.Errorf("expected cursor=2, got %d", m2.cursor)
	}
	if m2.Items[1].Entry.Name != "c" || m2.Items[2].Entry.Name != "b" {
		t.Errorf("expected items [a c b], got [%s %s %s]",
			m2.Items[0].Entry.Name, m2.Items[1].Entry.Name, m2.Items[2].Entry.Name)
	}
}

func TestMove_atTop_noOp(t *testing.T) {
	m := makeTestModel("a", "b", "c")
	// cursor is at 0 by default

	m2, _, ok := sendKeyMsg(m, "shift+up")
	if ok {
		t.Fatal("expected no MoveItemMsg at top")
	}
	if m2.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m2.cursor)
	}
	if m2.Items[0].Entry.Name != "a" {
		t.Errorf("expected items unchanged, first item is %s", m2.Items[0].Entry.Name)
	}
}

func TestMove_atBottom_noOp(t *testing.T) {
	m := makeTestModel("a", "b", "c")
	m = setCursor(m, 2) // cursor at last index

	m2, _, ok := sendKeyMsg(m, "shift+down")
	if ok {
		t.Fatal("expected no MoveItemMsg at bottom")
	}
	if m2.cursor != 2 {
		t.Errorf("expected cursor=2, got %d", m2.cursor)
	}
	if m2.Items[2].Entry.Name != "c" {
		t.Errorf("expected items unchanged, last item is %s", m2.Items[2].Entry.Name)
	}
}

// --- HidePaused tests ---

func makeModelWithPaused(specs ...struct {
	name   string
	paused bool
}) Model {
	items := make([]PipelineItem, len(specs))
	for i, s := range specs {
		items[i] = PipelineItem{
			Entry:  config.PipelineEntry{Name: s.name},
			Paused: s.paused,
		}
	}
	return New(items)
}

func sendKey(m Model, key string) Model {
	updated, _ := m.Update(keyMsgFromString(key))
	return updated.(Model)
}

func TestHidePaused_togglesField(t *testing.T) {
	m := makeTestModel("a", "b", "c")
	if m.HidePaused {
		t.Fatal("expected HidePaused=false by default")
	}
	m = sendKey(m, "h")
	if !m.HidePaused {
		t.Error("expected HidePaused=true after h")
	}
	m = sendKey(m, "h")
	if m.HidePaused {
		t.Error("expected HidePaused=false after second h")
	}
}

func TestHidePaused_visibleIdxFilters(t *testing.T) {
	m := makeModelWithPaused(
		struct {
			name   string
			paused bool
		}{"a", false},
		struct {
			name   string
			paused bool
		}{"b", true},
		struct {
			name   string
			paused bool
		}{"c", false},
	)
	m.HidePaused = true
	vi := m.visibleIdx()
	if len(vi) != 2 {
		t.Fatalf("expected 2 visible items, got %d", len(vi))
	}
	if vi[0] != 0 || vi[1] != 2 {
		t.Errorf("expected indices [0 2], got %v", vi)
	}
}

func TestHidePaused_clampsCursorOnToggle(t *testing.T) {
	m := makeModelWithPaused(
		struct {
			name   string
			paused bool
		}{"a", false},
		struct {
			name   string
			paused bool
		}{"b", false},
		struct {
			name   string
			paused bool
		}{"c", true},
	)
	// Move cursor to item c (index 2)
	m = setCursor(m, 2)
	if m.cursor != 2 {
		t.Fatalf("expected cursor=2, got %d", m.cursor)
	}
	// Hide paused — c disappears, cursor must clamp to 1 (last visible)
	m = sendKey(m, "h")
	if m.cursor != 1 {
		t.Errorf("expected cursor clamped to 1, got %d", m.cursor)
	}
}

func TestHidePaused_navigationStaysWithinVisible(t *testing.T) {
	m := makeModelWithPaused(
		struct {
			name   string
			paused bool
		}{"a", false},
		struct {
			name   string
			paused bool
		}{"b", true},
		struct {
			name   string
			paused bool
		}{"c", false},
	)
	m.HidePaused = true
	// cursor=0 → visible item 0 (Items index 0 = "a")
	// down → cursor=1 → visible item 1 (Items index 2 = "c")
	m = sendKey(m, "j")
	if m.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m.cursor)
	}
	// down again — already at last visible, no change
	m = sendKey(m, "j")
	if m.cursor != 1 {
		t.Errorf("expected cursor still 1, got %d", m.cursor)
	}
}

func TestHidePaused_selectedReturnsVisibleItem(t *testing.T) {
	m := makeModelWithPaused(
		struct {
			name   string
			paused bool
		}{"a", false},
		struct {
			name   string
			paused bool
		}{"b", true},
		struct {
			name   string
			paused bool
		}{"c", false},
	)
	m.HidePaused = true
	m = sendKey(m, "j") // cursor → visible index 1 (Items[2] = "c")

	item, ok := m.Selected()
	if !ok {
		t.Fatal("expected Selected to return true")
	}
	if item.Entry.Name != "c" {
		t.Errorf("expected selected item 'c', got '%s'", item.Entry.Name)
	}
}

func TestHidePaused_allPausedEmptyVisible(t *testing.T) {
	m := makeModelWithPaused(
		struct {
			name   string
			paused bool
		}{"a", true},
		struct {
			name   string
			paused bool
		}{"b", true},
	)
	m.HidePaused = true
	vi := m.visibleIdx()
	if len(vi) != 0 {
		t.Errorf("expected 0 visible items, got %d", len(vi))
	}
	_, ok := m.Selected()
	if ok {
		t.Error("expected Selected to return false when no visible items")
	}
}
