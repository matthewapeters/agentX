// Package scrollutil holds the small, surface-agnostic text/scroll primitives
// shared by TUI surfaces that host a scrollable viewport of capped, wrappable
// content — currently the output panel (internal/surfaces/output, PD-01) and the
// working-memory editor (internal/surfaces/workmemory, PD-WM). Extracted so the
// wrap/cap/scrollbar math has one source of truth instead of drifting copies.
package scrollutil

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// ScrollbarCell returns the scrollbar glyph for visible row i (thumb vs track),
// sized proportionally to the visible fraction of the content.
func ScrollbarCell(i, offset, total, track int) string {
	thumb := track * track / total
	if thumb < 1 {
		thumb = 1
	}
	span := total - track // max offset
	top := 0
	if span > 0 {
		top = (track - thumb) * offset / span
	}
	if i >= top && i < top+thumb {
		return "█"
	}
	return "░"
}

// tabWidth is the fixed number of spaces a tab expands to before any width
// measurement or wrapping in this package — not an attempt at true
// tab-stop semantics (which depend on cursor column), just enough to
// guarantee ansi.StringWidth and lipgloss's own internal measurement
// always agree on a given string's width. ansi.StringWidth counts a raw
// tab as a single column, but lipgloss.Style.Render (what
// viewport.Model.View ultimately calls) expands it wider when actually
// rendering — a line ansi.StringWidth measured as fitting exactly at a
// panel's width can still get soft-wrapped by lipgloss into an extra row
// nothing in a caller's row-count budget ever accounted for. Plain spaces
// have unambiguous width everywhere, so expanding tabs to them here — the
// one shared entry point every wrap/measure call in this package goes
// through — removes the disagreement at its source instead of trying to
// reconcile two width algorithms that disagree specifically about tabs
// (docs/architecture/behavior/scrollutil_tab_width_disagreement.feature.md).
const tabWidth = 4

// expandTabs replaces every tab in s with tabWidth spaces. A cheap no-op
// (one Contains scan) when s has no tabs, so it costs nothing in the
// overwhelmingly common case.
func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabWidth))
}

// WrapLines word-wraps s to w columns, preserving existing newlines.
func WrapLines(s string, w int) []string {
	s = expandTabs(s)
	if w <= 0 {
		return strings.Split(s, "\n")
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		out = append(out, strings.Split(ansi.Wrap(line, w, " -"), "\n")...)
	}
	return out
}

// TruncateWord fits s to w display columns, breaking at a word boundary and
// marking truncation with an ellipsis.
func TruncateWord(s string, w int) string {
	s = expandTabs(s)
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	first := oneLine(ansi.Wrap(s, w, " -"))
	if ansi.StringWidth(first) <= w {
		return first
	}
	return ansi.Truncate(first, w, "…")
}

// oneLine returns the first line of s.
func oneLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// PadTo right-pads (or clips) s to exactly w display columns.
func PadTo(s string, w int) string {
	s = expandTabs(s)
	width := ansi.StringWidth(s)
	if width == w {
		return s
	}
	if width > w {
		return ansi.Truncate(s, w, "")
	}
	return s + strings.Repeat(" ", w-width)
}

// ClampInt clamps v to [lo, hi].
func ClampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
