package approval

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"agentx/internal/state"
)

func testOptions() []state.ApprovalOption {
	return []state.ApprovalOption{
		{Label: "Approve for this session", Decision: "session"},
		{Label: "Approve for all sessions", Decision: "global"},
		{Label: "Deny", Decision: "deny"},
	}
}

func key(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Text: s, Code: rune(s[0])} }

func TestSetResetsCursorToFirstOption(t *testing.T) {
	m := New()
	m.Set("run rm -rf /tmp/x?", testOptions(), nil)
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.Selected().Decision; got != "deny" {
		t.Fatalf("expected cursor at 'deny' after two downs, got %q", got)
	}
	m.Set("a different prompt", testOptions(), nil)
	if got := m.Selected().Decision; got != "session" {
		t.Fatalf("Set did not reset cursor: got %q, want %q", got, "session")
	}
}

func TestCursorClampsAtBoundaries(t *testing.T) {
	m := New()
	m.Set("prompt", testOptions(), nil)

	// Up at the top stays put.
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := m.Selected().Decision; got != "session" {
		t.Fatalf("up at top: got %q, want %q", got, "session")
	}

	// Down past the bottom stays at the last option.
	for range 10 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if got := m.Selected().Decision; got != "deny" {
		t.Fatalf("down past bottom: got %q, want %q", got, "deny")
	}
}

func TestVimStyleNavigation(t *testing.T) {
	m := New()
	m.Set("prompt", testOptions(), nil)
	m.Update(key("j"))
	if got := m.Selected().Decision; got != "global" {
		t.Fatalf("j (down): got %q, want %q", got, "global")
	}
	m.Update(key("j"))
	if got := m.Selected().Decision; got != "deny" {
		t.Fatalf("j (down) x2: got %q, want %q", got, "deny")
	}
	m.Update(key("k"))
	if got := m.Selected().Decision; got != "global" {
		t.Fatalf("k (up): got %q, want %q", got, "global")
	}
}

func TestUpdateOnlyConfirmsOnEnter(t *testing.T) {
	m := New()
	m.Set("prompt", testOptions(), nil)
	for _, k := range []tea.KeyPressMsg{{Code: tea.KeyUp}, {Code: tea.KeyDown}, key("j"), key("k"), {Code: tea.KeyEsc}} {
		if action := m.Update(k); action != ActionNone {
			t.Fatalf("key %v: got action %v, want ActionNone", k, action)
		}
	}
	if action := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); action != ActionConfirm {
		t.Fatalf("enter: got action %v, want ActionConfirm", action)
	}
}

func TestUpdateNoOptionsNeverConfirms(t *testing.T) {
	m := New()
	m.Set("prompt", nil, nil)
	if action := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); action != ActionNone {
		t.Fatalf("enter with no options: got action %v, want ActionNone", action)
	}
}

func TestDesiredHeightScalesWithOptionCount(t *testing.T) {
	m := New()
	m.SetSize(80, 0)
	m.Set("short", testOptions()[:1], nil)
	h1 := m.DesiredHeight()
	m.Set("short", testOptions(), nil)
	h3 := m.DesiredHeight()
	if h3 <= h1 {
		t.Fatalf("DesiredHeight did not grow with more options: h1=%d h3=%d", h1, h3)
	}
	if h3-h1 != 2 {
		t.Fatalf("expected DesiredHeight to grow by 2 rows for 2 extra options, got delta %d", h3-h1)
	}
}

// GIVEN a request with nothing queued behind it (nil or empty queued)
// WHEN the panel renders
// THEN no queued-preview section appears — View()/DesiredHeight() are
// byte-for-byte/exactly what they were before this feature existed.
func TestQueuedEmptyRendersNothing(t *testing.T) {
	m := New()
	m.SetSize(80, 0)
	m.Set("prompt", testOptions(), nil)
	withoutQueued := m.View()
	heightWithout := m.DesiredHeight()

	m.Set("prompt", testOptions(), []string{})
	if got := m.View(); got != withoutQueued {
		t.Errorf("View() with empty queued = %q, want unchanged %q", got, withoutQueued)
	}
	if got := m.DesiredHeight(); got != heightWithout {
		t.Errorf("DesiredHeight() with empty queued = %d, want unchanged %d", got, heightWithout)
	}
}

// GIVEN one or more queued prompts
// WHEN the panel renders
// THEN a "Also waiting:" section lists them, and DesiredHeight grows to
// cover it.
func TestQueuedNonEmptyRendersPreview(t *testing.T) {
	m := New()
	m.SetSize(80, 0)
	m.Set("prompt", testOptions(), []string{"run http_get?", "run read_file?"})

	view := m.View()
	if !strings.Contains(view, "Also waiting:") {
		t.Errorf("View() = %q, want it to contain %q", view, "Also waiting:")
	}
	if !strings.Contains(view, "run http_get?") || !strings.Contains(view, "run read_file?") {
		t.Errorf("View() = %q, want both queued prompts listed", view)
	}
	if got, want := m.DesiredHeight(), len(m.promptLines())+len(testOptions())+3; got != want {
		t.Errorf("DesiredHeight() = %d, want %d (prompt + options + header + 2 queued rows)", got, want)
	}
}

// GIVEN more queued prompts than maxQueuedPreview
// WHEN the panel renders
// THEN only the first maxQueuedPreview are listed individually, followed by
// a "+N more" summary row — the panel's height stays bounded regardless of
// how deep the queue gets.
func TestQueuedCapsPreviewWithMoreSummary(t *testing.T) {
	m := New()
	m.SetSize(80, 0)
	queued := make([]string, maxQueuedPreview+3)
	for i := range queued {
		queued[i] = fmt.Sprintf("call %d", i)
	}
	m.Set("prompt", testOptions(), queued)

	view := m.View()
	if !strings.Contains(view, "… and 3 more") {
		t.Errorf("View() = %q, want a \"… and 3 more\" summary row", view)
	}
	for i := maxQueuedPreview; i < len(queued); i++ {
		if strings.Contains(view, queued[i]) {
			t.Errorf("View() unexpectedly lists %q individually beyond the cap", queued[i])
		}
	}
	lines := m.queuedLines()
	// header + maxQueuedPreview individual rows + 1 summary row
	if len(lines) != maxQueuedPreview+2 {
		t.Errorf("queuedLines() has %d rows, want %d", len(lines), maxQueuedPreview+2)
	}
}

// oversizedPrompt returns a prompt that wraps to well over maxPromptRows
// rows at a narrow width, for tests exercising the scroll/window path.
func oversizedPrompt() string {
	words := make([]string, 200)
	for i := range words {
		words[i] = "word"
	}
	return strings.Join(words, " ")
}

// GIVEN a prompt long enough to wrap past maxPromptRows at the panel's
// current width
// WHEN promptLines renders it
// THEN it is windowed to exactly maxPromptRows rows, each carrying a
// scrollbar cell in its last column — never unbounded, regardless of how
// long the prompt is or how narrow the panel is, and never silently
// dropping content the way a truncation marker would (docs/architecture/
// behavior/approval_prompt_length_bound.feature.md).
func TestPromptLinesWindowsAtMaxPromptRows(t *testing.T) {
	m := New()
	m.SetSize(10, 0) // narrow, so a modest prompt still wraps to many rows
	m.Set(oversizedPrompt(), testOptions(), nil)

	lines := m.promptLines()
	if len(lines) != maxPromptRows {
		t.Fatalf("promptLines() has %d rows, want %d", len(lines), maxPromptRows)
	}
	for i, l := range lines {
		if l == "" {
			t.Errorf("promptLines()[%d] is empty, want a scrollbar cell at minimum", i)
		}
	}
}

// GIVEN an oversized prompt, freshly Set (scrolled to the top)
// WHEN PgDn pages down and PgUp pages back
// THEN the visible window's content actually changes on PgDn (proving
// ScrollPrompt/Update wire into promptLines, not just adjust a field
// nothing reads) and returns to the original top window on PgUp.
func TestScrollPromptPagesThroughOversizedPrompt(t *testing.T) {
	m := New()
	m.SetSize(10, 0)
	m.Set(oversizedPrompt(), testOptions(), nil)

	top := strings.Join(m.promptLines(), "\n")
	if m.Update(tea.KeyPressMsg{Text: "pgdown", Code: tea.KeyPgDown}) != ActionNone {
		t.Fatal("pgdown returned a non-ActionNone Action")
	}
	scrolled := strings.Join(m.promptLines(), "\n")
	if scrolled == top {
		t.Fatal("promptLines() unchanged after pgdown, want the visible window to advance")
	}

	if m.Update(tea.KeyPressMsg{Text: "pgup", Code: tea.KeyPgUp}) != ActionNone {
		t.Fatal("pgup returned a non-ActionNone Action")
	}
	if got := strings.Join(m.promptLines(), "\n"); got != top {
		t.Errorf("promptLines() after pgup = %q, want back to the original top window %q", got, top)
	}
}

// GIVEN an oversized prompt scrolled past its top
// WHEN PgDn is pressed far more times than there is content
// THEN the scroll offset clamps at the bottom instead of running past it —
// mirrors output.Model.ScrollSelected's clamping contract.
func TestScrollPromptClampsAtBottom(t *testing.T) {
	m := New()
	m.SetSize(10, 0)
	m.Set(oversizedPrompt(), testOptions(), nil)

	for range 50 {
		m.Update(tea.KeyPressMsg{Text: "pgdown", Code: tea.KeyPgDown})
	}
	bottom := strings.Join(m.promptLines(), "\n")
	m.Update(tea.KeyPressMsg{Text: "pgdown", Code: tea.KeyPgDown})
	if got := strings.Join(m.promptLines(), "\n"); got != bottom {
		t.Errorf("promptLines() changed after scrolling past the bottom, want it to stay clamped")
	}
}

// GIVEN a widget with no options yet (an edge case, but PgUp/PgDn must not
// depend on options being present)
// WHEN PgDn is pressed
// THEN it still scrolls the prompt rather than being swallowed by the
// len(m.options) == 0 guard that gates option-cursor movement.
func TestScrollPromptWorksWithNoOptions(t *testing.T) {
	m := New()
	m.SetSize(10, 0)
	m.Set(oversizedPrompt(), nil, nil)

	top := strings.Join(m.promptLines(), "\n")
	m.Update(tea.KeyPressMsg{Text: "pgdown", Code: tea.KeyPgDown})
	if got := strings.Join(m.promptLines(), "\n"); got == top {
		t.Error("promptLines() unchanged after pgdown with no options set, want it to still scroll")
	}
}

// GIVEN a short prompt that fits well within maxPromptRows
// WHEN promptLines renders it
// THEN the cap is a no-op — unchanged from before this fix.
func TestPromptLinesShortPromptUnaffected(t *testing.T) {
	m := New()
	m.SetSize(80, 0)
	m.Set("a short prompt", testOptions(), nil)

	lines := m.promptLines()
	if len(lines) != 1 || lines[0] != "a short prompt" {
		t.Errorf("promptLines() = %v, want [%q] unchanged", lines, "a short prompt")
	}
}
