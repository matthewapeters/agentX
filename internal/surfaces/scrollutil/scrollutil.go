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

// WrapLines word-wraps s to w columns, preserving existing newlines.
func WrapLines(s string, w int) []string {
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
