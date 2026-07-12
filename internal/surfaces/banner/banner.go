// Package banner is the chat surface's pinned logo banner: a fixed screen
// region above the output viewport, carved out of the chat surface's height
// budget the same way the input panel is (internal/surfaces/chat's
// relayout) — never part of the output viewport's scrollable content, so it
// never scrolls with the transcript.
//
// The banner starts full-size (the application logo). The first time the
// applied transcript content would exceed one screenful under that full-size
// budget, it collapses to a single row reading "AgentX - <label>" — a
// one-way, sticky transition for the rest of the session (MaybeCollapse). The
// label tracks what the agent is currently doing (SetLabel) — the caller
// (internal/surfaces/chat) maps state.RunState/state.Phase to text; this
// package only renders whatever label it's given, synthesizing the
// collapsed row's cells (and their grayscale gradient) from that text at
// call time, so the row is never a fixed length.
//
// While the run is state.StateWorking, whichever grid is active animates a
// left-to-right traveling rainbow hue, modulated by each cell's originally
// authored grayscale luminance, so the banner's existing shading survives
// and only its hue moves; otherwise it renders that same static grayscale
// unchanged. See docs/ux/06_OUTPUT_WIDGET.md ("Logo banner").
package banner

import (
	"math"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	colorful "github.com/lucasb-eyer/go-colorful"
)

// Cell is one glyph of a banner grid: the rune to draw and the xterm-256
// palette index it was authored with (-1 when no color was set). The
// generated grid (logo_generated.go, produced by cmd/logogen from
// logo/agentx.logo — see logo/README.md) supplies the full-size banner's
// data; the collapsed row is synthesized at runtime instead (buildLabelGrid),
// since its text varies with the agent's current activity.
type Cell struct {
	Rune  rune
	Color int16
}

// collapsedFirst and collapsedLast bound the left-to-right gradient the
// collapsed row's text is colored with — the same values logo/coloriz.py
// uses to author the full-size banner's grayscale ramp, so the collapsed
// row's static (non-animated) look matches it.
const (
	collapsedFirst = 255
	collapsedLast  = 232
)

// defaultLabel seeds the banner before the first SetLabel call — matching
// the chat surface's initial state.StateIdle.
const defaultLabel = "Your Local Agent"

// frameInterval bounds the animation to a modest, non-real-time-high frame
// rate (~10/sec) so a long-running agent task doesn't impose continuous
// high-frequency rendering.
const frameInterval = 100 * time.Millisecond

// huePerColumn and huePerFrame control the traveling wave: degrees of hue
// shift per column (the rainbow's width across the banner) and per animation
// frame (how fast it visibly sweeps left to right as frame increases).
const (
	huePerColumn = 6.0
	huePerFrame  = 3.0
)

// grayLo and grayHi bound the xterm-256 grayscale ramp this repo's banner
// assets are authored in (see logo/coloriz.py); a color index in that range
// maps linearly to a 0..1 luminance. A color outside it (not produced by the
// current asset pipeline, but defensively handled) falls back to mid gray.
const (
	grayLo = 232
	grayHi = 255
)

// TickMsg advances the rainbow-wave animation by one frame.
type TickMsg struct{}

// Model is the pinned banner's state.
type Model struct {
	full, collapsed [][]Cell
	label           string
	width           int
	collapsedSticky bool
	animating       bool
	frame           int
}

// New returns a banner seeded with the compiled-in logo grid and the default
// (idle) label. Leading and trailing blank rows are trimmed from the
// full-size grid: they were spacing for the banner's old placement as the
// first line of scrollable content, and aren't worth permanently occupying a
// fixed chrome region for.
func New() *Model {
	m := &Model{full: trimBlankRows(LogoGrid), label: defaultLabel}
	m.collapsed = buildLabelGrid(m.collapsedText())
	return m
}

// SetLabel sets the text shown after "AgentX - " in the collapsed row (e.g.
// "Working", "Thinking", "Your Local Agent") — the caller maps
// state.RunState/state.Phase to this text; see internal/surfaces/chat. A
// no-op if the label is unchanged, since it's paid for by re-synthesizing
// the collapsed row's cells.
func (m *Model) SetLabel(label string) {
	if label == m.label {
		return
	}
	m.label = label
	m.collapsed = buildLabelGrid(m.collapsedText())
}

func (m *Model) collapsedText() string { return "AgentX - " + m.label }

// SetWidth sets the panel width lines are clipped to.
func (m *Model) SetWidth(w int) { m.width = max(w, 0) }

// Height returns the banner's current fixed row count: the full grid's row
// count, or the collapsed grid's (today, 1) once collapsed.
func (m *Model) Height() int {
	if m.collapsedSticky {
		return len(m.collapsed)
	}
	return len(m.full)
}

// FullHeight returns the full-size grid's row count regardless of the
// current collapse state — the fixed budget MaybeCollapse measures against,
// so the trigger point doesn't move once evaluated.
func (m *Model) FullHeight() int { return len(m.full) }

// Collapsed reports whether the banner has collapsed to the wordmark.
func (m *Model) Collapsed() bool { return m.collapsedSticky }

// MaybeCollapse trips the one-way collapse the first time contentLines (the
// output viewport's total applied content) exceeds budgetLines (the output
// viewport height available under the full-size banner). A no-op once
// already collapsed, and a no-op while at or under budget.
func (m *Model) MaybeCollapse(contentLines, budgetLines int) {
	if m.collapsedSticky {
		return
	}
	if contentLines > budgetLines {
		m.collapsedSticky = true
	}
}

// SetAnimating turns the rainbow-wave animation on or off, matching the run
// entering/leaving state.StateWorking. Turning it on (off -> on) returns the
// tea.Cmd that starts the tick loop; any other call (already in that state,
// or turning it off) returns nil.
func (m *Model) SetAnimating(on bool) tea.Cmd {
	if on == m.animating {
		return nil
	}
	m.animating = on
	if !on {
		return nil
	}
	return tick()
}

// Animating reports whether the rainbow-wave animation is currently active.
func (m *Model) Animating() bool { return m.animating }

func tick() tea.Cmd {
	return tea.Tick(frameInterval, func(time.Time) tea.Msg { return TickMsg{} })
}

// Tick advances the animation by one frame and returns the next tick's cmd,
// or nil once animation has been turned off (ending the tick chain rather
// than ticking forever in the background).
func (m *Model) Tick(TickMsg) tea.Cmd {
	if !m.animating {
		return nil
	}
	m.frame++
	return tick()
}

func (m *Model) active() [][]Cell {
	if m.collapsedSticky {
		return m.collapsed
	}
	return m.full
}

// View renders the active grid, clipped (ANSI-aware) to the panel width.
func (m *Model) View() string {
	grid := m.active()
	lines := make([]string, len(grid))
	for i, row := range grid {
		lines[i] = m.renderRow(row)
	}
	if m.width > 0 {
		for i, l := range lines {
			if ansi.StringWidth(l) > m.width {
				lines[i] = ansi.Truncate(l, m.width, "")
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderRow(row []Cell) string {
	var b strings.Builder
	for col, c := range row {
		switch {
		case m.animating:
			writeTrueColor(&b, rainbowCell(c, col, m.frame))
		case c.Color >= 0:
			b.WriteString("\x1b[38;5;")
			b.WriteString(strconv.Itoa(int(c.Color)))
			b.WriteByte('m')
		}
		b.WriteRune(c.Rune)
	}
	if len(row) > 0 {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// rainbowCell computes the animated color for one cell: a hue that travels
// left to right as frame advances (a traveling wave, C(col, frame) = f(col -
// v*frame), moves in +col direction over time), at fixed high saturation,
// with value taken from the cell's original grayscale luminance so the
// banner's existing shading/shape survives the recolor.
func rainbowCell(c Cell, col, frame int) colorful.Color {
	hue := math.Mod(float64(col)*huePerColumn-float64(frame)*huePerFrame, 360)
	if hue < 0 {
		hue += 360
	}
	return colorful.Hsv(hue, 0.9, luminance(c.Color))
}

func luminance(colorIdx int16) float64 {
	if colorIdx < grayLo || colorIdx > grayHi {
		return 0.5
	}
	return float64(colorIdx-grayLo) / float64(grayHi-grayLo)
}

func writeTrueColor(b *strings.Builder, c colorful.Color) {
	r, g, bl := c.RGB255()
	b.WriteString("\x1b[38;2;")
	b.WriteString(strconv.Itoa(int(r)))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(int(g)))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(int(bl)))
	b.WriteByte('m')
}

// buildLabelGrid synthesizes a one-row grid from text, colored with the same
// left-to-right grayscale gradient logo/coloriz.py authors the full-size
// banner with (linearly interpolated across the text's rune count), so the
// collapsed row's static look matches the full banner's.
func buildLabelGrid(text string) [][]Cell {
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return [][]Cell{{}}
	}
	step := float64(collapsedFirst-collapsedLast) / float64(n)
	row := make([]Cell, n)
	for i, r := range runes {
		row[i] = Cell{Rune: r, Color: int16(float64(collapsedFirst) - float64(i)*step)}
	}
	return [][]Cell{row}
}

// trimBlankRows drops leading and trailing zero-length rows (blank lines in
// the authored source).
func trimBlankRows(grid [][]Cell) [][]Cell {
	start, end := 0, len(grid)
	for start < end && len(grid[start]) == 0 {
		start++
	}
	for end > start && len(grid[end-1]) == 0 {
		end--
	}
	return grid[start:end]
}
