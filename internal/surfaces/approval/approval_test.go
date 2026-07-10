package approval

import (
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
	m.Set("run rm -rf /tmp/x?", testOptions())
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.Selected().Decision; got != "deny" {
		t.Fatalf("expected cursor at 'deny' after two downs, got %q", got)
	}
	m.Set("a different prompt", testOptions())
	if got := m.Selected().Decision; got != "session" {
		t.Fatalf("Set did not reset cursor: got %q, want %q", got, "session")
	}
}

func TestCursorClampsAtBoundaries(t *testing.T) {
	m := New()
	m.Set("prompt", testOptions())

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
	m.Set("prompt", testOptions())
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
	m.Set("prompt", testOptions())
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
	m.Set("prompt", nil)
	if action := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); action != ActionNone {
		t.Fatalf("enter with no options: got action %v, want ActionNone", action)
	}
}

func TestDesiredHeightScalesWithOptionCount(t *testing.T) {
	m := New()
	m.SetSize(80, 0)
	m.Set("short", testOptions()[:1])
	h1 := m.DesiredHeight()
	m.Set("short", testOptions())
	h3 := m.DesiredHeight()
	if h3 <= h1 {
		t.Fatalf("DesiredHeight did not grow with more options: h1=%d h3=%d", h1, h3)
	}
	if h3-h1 != 2 {
		t.Fatalf("expected DesiredHeight to grow by 2 rows for 2 extra options, got delta %d", h3-h1)
	}
}
