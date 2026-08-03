package scrollutil

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// GIVEN a line containing a tab character, at a width where ansi.StringWidth
// (before this fix, the only measure WrapLines' caller ever consulted) says
// it fits exactly
// WHEN WrapLines wraps it
// THEN the result's row count matches what lipgloss's own Render() at the
// same width actually produces — the exact mismatch that let an untracked
// extra row past every row-count budget built on ansi.StringWidth alone
// (docs/architecture/behavior/scrollutil_tab_width_disagreement.feature.md).
func TestWrapLinesTabWidthMatchesLipgloss(t *testing.T) {
	s := "a\tb"
	w := ansi.StringWidth(s) // width ansi.StringWidth reports BEFORE expansion
	if w != 2 {
		t.Fatalf("test assumption violated: ansi.StringWidth(%q) = %d, want 2", s, w)
	}

	got := WrapLines(s, w)
	lipglossRows := strings.Split(lipgloss.NewStyle().Width(w).Render(s), "\n")
	if len(got) != len(lipglossRows) {
		t.Errorf("WrapLines(%q, %d) produced %d rows, lipgloss.Render at the same width produced %d — still disagree",
			s, w, len(got), len(lipglossRows))
	}
}

// GIVEN a string with no tab characters at all
// WHEN WrapLines wraps it
// THEN the result is unchanged from before this fix — expandTabs is a no-op
// whenever there's nothing to expand.
func TestWrapLinesNoTabsUnaffected(t *testing.T) {
	s := "plain text with no tabs at all"
	got := WrapLines(s, 80)
	if len(got) != 1 || got[0] != s {
		t.Errorf("WrapLines(%q, 80) = %v, want unchanged [%q]", s, got, s)
	}
}

// GIVEN a title string containing a tab, narrow enough to need truncation
// WHEN TruncateWord truncates it
// THEN the tab is expanded first — TruncateWord independently measures
// width via ansi.StringWidth/ansi.Wrap, so it needs the same fix as
// WrapLines, not just a caller that happens to route through WrapLines
// first.
func TestTruncateWordExpandsTabs(t *testing.T) {
	s := "a\tb\tc\td\te\tf\tg"
	got := TruncateWord(s, 5)
	if strings.Contains(got, "\t") {
		t.Errorf("TruncateWord(%q, 5) = %q, want tabs expanded before truncation", s, got)
	}
}

// GIVEN a string containing a tab, padded to a target width
// WHEN PadTo pads it
// THEN the tab is expanded first, so the returned string's actual display
// width (post-expansion) is exactly the target, not off by whatever a raw
// tab would have measured as under a different renderer.
func TestPadToExpandsTabs(t *testing.T) {
	got := PadTo("a\tb", 10)
	if strings.Contains(got, "\t") {
		t.Errorf("PadTo(%q, 10) = %q, want tabs expanded", "a\tb", got)
	}
	if w := ansi.StringWidth(got); w != 10 {
		t.Errorf("PadTo(%q, 10) display width = %d, want 10", "a\tb", w)
	}
}
