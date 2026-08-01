package list

import (
	"strings"
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

func TestCopyUsesSelectedVisiblePipeline(t *testing.T) {
	m := makeModelWithPaused(
		struct {
			name   string
			paused bool
		}{"hidden", true},
		struct {
			name   string
			paused bool
		}{"visible", false},
	)
	m.HidePaused = true

	_, cmd := m.Update(keyMsgFromString("c"))
	if cmd == nil {
		t.Fatal("expected copy command")
	}
	msg, ok := cmd().(OpenCopyMsg)
	if !ok {
		t.Fatalf("expected OpenCopyMsg, got %T", cmd())
	}
	if msg.Entry.Name != "visible" {
		t.Errorf("copy entry = %q, want %q", msg.Entry.Name, "visible")
	}
}

func TestDeleteUsesSelectedVisiblePipeline(t *testing.T) {
	m := makeModelWithPaused(
		struct {
			name   string
			paused bool
		}{"hidden", true},
		struct {
			name   string
			paused bool
		}{"visible", false},
	)
	m.HidePaused = true

	_, cmd := m.Update(keyMsgFromString("d"))
	if cmd == nil {
		t.Fatal("expected delete command")
	}
	msg, ok := cmd().(OpenDeleteMsg)
	if !ok {
		t.Fatalf("expected OpenDeleteMsg, got %T", cmd())
	}
	if msg.Entry.Name != "visible" {
		t.Errorf("delete entry = %q, want %q", msg.Entry.Name, "visible")
	}
}

// --- Fuzzy search tests ---

// typeSearch sends "/" to enter search mode, then types the given query one
// rune at a time.
func typeSearch(m Model, query string) Model {
	m = sendKey(m, "/")
	for _, r := range query {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	return m
}

func TestFuzzyMatch_basicSubsequence(t *testing.T) {
	cases := []struct {
		query, target string
		want          bool
	}{
		{"abc", "abcdef", true},
		{"ace", "abcde", true},
		{"aec", "abcde", false}, // wrong order
		{"ABC", "abcde", true},  // case-insensitive
		{"", "anything", true},  // empty query matches everything
		{"abc", "", false},      // non-empty query, empty target
		{"deploy", "deploy-prod", true},
		{"dprd", "deploy-prod", true},
		{"xyz", "abcdef", false},
	}
	for _, tc := range cases {
		got := fuzzyMatch(tc.query, tc.target)
		if got != tc.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tc.query, tc.target, got, tc.want)
		}
	}
}

func TestSearch_entersSearchMode(t *testing.T) {
	m := makeTestModel("alpha", "beta", "gamma")
	if m.IsSearching() {
		t.Fatal("expected not searching initially")
	}
	m = sendKey(m, "/")
	if !m.IsSearching() {
		t.Error("expected IsSearching=true after /")
	}
}

func TestSearch_typingFiltersVisibleItems(t *testing.T) {
	m := makeTestModel("deploy-prod", "deploy-staging", "build-prod")
	m = typeSearch(m, "dep")

	vi := m.visibleIdx()
	if len(vi) != 2 {
		t.Fatalf("expected 2 matches for 'dep', got %d", len(vi))
	}
	if m.Items[vi[0]].Entry.Name != "deploy-prod" || m.Items[vi[1]].Entry.Name != "deploy-staging" {
		t.Errorf("unexpected matched items: %v", vi)
	}
}

func TestSearch_enterConfirmsAndKeepsFilter(t *testing.T) {
	m := makeTestModel("foo", "bar", "foobar")
	m = typeSearch(m, "foo")
	m = sendKey(m, "enter")

	if m.IsSearching() {
		t.Error("expected IsSearching=false after enter")
	}
	if m.query != "foo" {
		t.Errorf("expected query to remain 'foo', got %q", m.query)
	}
	vi := m.visibleIdx()
	if len(vi) != 2 {
		t.Errorf("expected 2 items still filtered, got %d", len(vi))
	}
}

func TestSearch_escClearsQueryAndExitsMode(t *testing.T) {
	m := makeTestModel("foo", "bar")
	m = typeSearch(m, "foo")
	m = sendKey(m, "esc")

	if m.IsSearching() {
		t.Error("expected not searching after esc")
	}
	if m.query != "" {
		t.Errorf("expected empty query after esc, got %q", m.query)
	}
	if len(m.visibleIdx()) != 2 {
		t.Error("expected all items visible after clearing query")
	}
}

func TestSearch_escWhenNotSearchingClearsActiveFilter(t *testing.T) {
	m := makeTestModel("foo", "bar", "baz")
	m = typeSearch(m, "ba")
	m = sendKey(m, "enter") // confirm filter, exit search mode

	// Now pressing esc clears the residual filter
	m = sendKey(m, "esc")
	if m.query != "" {
		t.Errorf("expected query cleared by esc, got %q", m.query)
	}
	if len(m.visibleIdx()) != 3 {
		t.Error("expected all 3 items visible after esc clear")
	}
}

func TestSearch_backspaceRemovesChar(t *testing.T) {
	m := makeTestModel("foo", "bar")
	m = typeSearch(m, "fo")
	m = sendKey(m, "backspace")

	if m.query != "f" {
		t.Errorf("expected query 'f' after backspace, got %q", m.query)
	}
}

func TestSearch_selectedReturnsFilteredItem(t *testing.T) {
	m := makeTestModel("alpha", "beta", "gamma")
	m = typeSearch(m, "bet")

	item, ok := m.Selected()
	if !ok {
		t.Fatal("expected Selected to return true")
	}
	if item.Entry.Name != "beta" {
		t.Errorf("expected 'beta', got %q", item.Entry.Name)
	}
}

func TestSearch_resetsCursorOnEachChar(t *testing.T) {
	m := makeTestModel("alpha", "beta", "gamma")
	m = sendKey(m, "/")
	// Navigate down first
	m = sendKey(m, "j")
	if m.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m.cursor)
	}
	// Typing a char should reset cursor to 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	if m.cursor != 0 {
		t.Errorf("expected cursor reset to 0 after typing, got %d", m.cursor)
	}
}

func TestViewportKeepsCursorVisibleAtTopAndBottom(t *testing.T) {
	m := makeTestModel(
		"pipeline-0", "pipeline-1", "pipeline-2", "pipeline-3", "pipeline-4",
		"pipeline-5", "pipeline-6", "pipeline-7", "pipeline-8", "pipeline-9",
	)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 13})
	m = updated.(Model)

	m = sendKey(m, "end")
	view := m.View()
	if !strings.Contains(view, "pipeline-9") || strings.Contains(view, "pipeline-0") {
		t.Errorf("viewport did not scroll to bottom:\n%s", view)
	}
	if item, ok := m.Selected(); !ok || item.Entry.Name != "pipeline-9" {
		t.Fatalf("selected item at bottom = %#v, %v", item, ok)
	}

	m = sendKey(m, "home")
	view = m.View()
	if !strings.Contains(view, "pipeline-0") || strings.Contains(view, "pipeline-9") {
		t.Errorf("viewport did not return to top:\n%s", view)
	}
}

func TestPageNavigationMovesByVisiblePage(t *testing.T) {
	m := makeTestModel("0", "1", "2", "3", "4", "5", "6")
	updated, _ := m.Update(tea.WindowSizeMsg{Height: 13}) // three pipeline rows
	m = updated.(Model)

	m = sendKey(m, "pgdown")
	if m.cursor != 3 {
		t.Errorf("cursor after page down = %d, want 3", m.cursor)
	}
	m = sendKey(m, "pgup")
	if m.cursor != 0 {
		t.Errorf("cursor after page up = %d, want 0", m.cursor)
	}
}

func TestPositionUsesFilteredList(t *testing.T) {
	m := makeTestModel("alpha", "beta", "gamma", "delta")
	m = typeSearch(m, "ta")
	m = sendKey(m, "down")

	position, total := m.Position()
	if position != 2 || total != 2 {
		t.Errorf("position = %d/%d, want 2/2", position, total)
	}
}

func TestFocusMessageAndStateUseSelectedPipeline(t *testing.T) {
	m := makeModelWithPaused(
		struct {
			name   string
			paused bool
		}{"paused", true},
		struct {
			name   string
			paused bool
		}{"active", false},
	)

	_, cmd := m.Update(keyMsgFromString("f"))
	msg, ok := cmd().(FocusMsg)
	if !ok || msg.Index != 0 {
		t.Fatalf("focus message = %#v, %v", msg, ok)
	}

	m = m.SetFocused(msg.Index)
	if !m.HidePaused {
		t.Error("expected focus to hide paused pipelines")
	}
	if m.Items[0].Paused || !m.Items[1].Paused {
		t.Errorf("focus pause states = [%v %v]", m.Items[0].Paused, m.Items[1].Paused)
	}
	item, ok := m.Selected()
	if !ok || item.Entry.Name != "paused" {
		t.Fatalf("focused selection = %#v, %v", item, ok)
	}
	if visible := m.visibleIdx(); len(visible) != 1 || visible[0] != msg.Index {
		t.Errorf("visible indexes after focus = %v, want [%d]", visible, msg.Index)
	}
}
