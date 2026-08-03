package chat

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"agentx/internal/state"
)

// GIVEN a chat surface fed a long unbroken token (no spaces) wider than any
// panel's wrap width — the adversarial case a real session hit — across a
// range of window sizes
// WHEN View() renders
// THEN every resulting row's display width is <= the window width, so no row
// can force the terminal to insert an extra physical row that pushes the
// input/approval panels off the visible viewport.
func TestViewNeverExceedsWidth(t *testing.T) {
	longToken := strings.Repeat("x", 500)

	for _, size := range []struct{ w, h int }{
		{80, 24}, {120, 40}, {40, 15}, {200, 50},
	} {
		m := New()
		m.width = size.w
		m.height = size.h
		m.relayout()

		m.output.Apply(state.Event{
			EventType:   "AGENT_RESPONSE",
			ContentType: state.ContentAgentResponse,
			Payload:     map[string]any{"text": "prefix " + longToken + " suffix"},
			Enabled:     true,
		})
		m.relayout()

		view := m.View()
		for i, row := range strings.Split(view.Content, "\n") {
			if w := ansi.StringWidth(row); w > size.w {
				t.Errorf("size %dx%d: row %d width = %d, want <= %d: %q", size.w, size.h, i, w, size.w, row)
			}
		}
	}
}

// GIVEN content that already fits well within the window
// WHEN View() renders
// THEN the injected text survives intact — proving the clamp is a no-op in
// the common path (nothing gets truncated) rather than just safe in the
// adversarial one.
func TestViewUnchangedWhenContentFits(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.relayout()

	const want = "a short reply that easily fits"
	m.output.Apply(state.Event{
		EventType:   "AGENT_RESPONSE",
		ContentType: state.ContentAgentResponse,
		Payload:     map[string]any{"text": want},
		Enabled:     true,
	})
	m.relayout()

	if view := m.View(); !strings.Contains(view.Content, want) {
		t.Errorf("View() = %q, want it to contain unmodified text %q", view.Content, want)
	}
}

// GIVEN a string containing ANSI styling codes (runes that contribute zero
// display width) — the case padLine's rune-counting predecessor got wrong,
// found by inspection: it measured len([]rune(s)), not display width, unlike
// its sibling padCells right next to it in this file
// WHEN padLine pads/truncates it to a target width
// THEN the result's actual display width (ansi.StringWidth) is exactly the
// target — not the target rune count, which would systematically undercount
// visible content whenever styling codes are present.
func TestPadLineMeasuresDisplayWidthNotRuneCount(t *testing.T) {
	styled := "\x1b[38;5;6m" + "hi" + "\x1b[0m" // 2 visible cells, many more runes
	const width = 10

	got := padLine(styled, width)
	if w := ansi.StringWidth(got); w != width {
		t.Errorf("padLine(%q, %d) display width = %d, want %d", styled, width, w, width)
	}

	long := "\x1b[38;5;6m" + strings.Repeat("x", 20) + "\x1b[0m"
	got = padLine(long, width)
	if w := ansi.StringWidth(got); w != width {
		t.Errorf("padLine(long styled, %d) display width = %d, want %d", width, w, width)
	}
}

// GIVEN a styled spinner frame reaches statusBar (the same class of input
// that exposed padLine's bug)
// WHEN statusBar renders
// THEN the result's display width is exactly the requested width, never more
// — proving the fix without depending on whether the live spinner style
// happens to emit ANSI codes today.
func TestStatusBarMeasuresDisplayWidthNotRuneCount(t *testing.T) {
	styled := "\x1b[38;5;6m⠋\x1b[0m"
	got := statusBar(state.ProcessingState{State: state.StateWorking}, styled, 40)
	if w := ansi.StringWidth(got); w != 40 {
		t.Errorf("statusBar display width = %d, want 40: %q", w, got)
	}
}
