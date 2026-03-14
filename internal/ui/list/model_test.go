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
