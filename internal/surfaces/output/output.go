// Package output is the chat surface's output panel: a vertical stack of
// collapsible widgets (one per conversation event) hosted in a scrollable
// viewport. Each widget is an IBM-style box with an always-visible, word-break
// truncated header; a body that collapses/expands; a configurable height cap with
// in-place scrolling and a proportional scrollbar; and a selection cursor that
// drives collapse and inner scroll.
//
// Source contract: docs/ux/06_OUTPUT_WIDGET.md (re-authors PD-01/PD-09 for the
// TUI). Backlog task: CHT-D1.
package output

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"agentx/internal/state"
)

// defaultMaxBody is the fallback body-row cap before a widget scrolls in place.
const defaultMaxBody = 20

type entryKind int

const (
	kindUser entryKind = iota
	kindClassification
	kindAssistant
	kindThinking
	kindToolCall
	kindToolResult
	kindSystem
	kindError
	kindLaunch
)

// launchItem pairs a launchable surface kind with its full attach command. The
// name is shown in the launch-info widget; the command (which carries the attach
// token) is never rendered — it is only ever placed on the clipboard.
type launchItem struct {
	name    string
	command string
}

// widget is one renderable output entry.
type widget struct {
	kind        entryKind
	header      string // emoji + label, always shown (one line)
	body        string // optional detail, collapsible
	collapsible bool
	collapsed   bool
	offset      int // inner scroll offset (wrapped body-line index)
}

// defaultActive and defaultInactive are the built-in border SGR colors used
// until SetTheme overrides them (cyan / dark gray, matching config defaults).
const (
	defaultActive   = "38;5;6"
	defaultInactive = "38;5;240"
)

// Model is the output panel state.
type Model struct {
	vp       viewport.Model
	width    int
	height   int
	maxBody  int
	banner   string // optional logo banner pinned above all widgets (bootstrap)
	widgets  []*widget
	selected int    // index of the selected widget, or -1 when empty
	focused  bool   // whether the output panel currently holds focus
	active   string // SGR color for the selected widget when focused
	inactive string // SGR color for unselected widgets / unfocused panel

	launchItems []launchItem // attach commands behind the launch-info widget
	copied      string       // name of the last surface whose command was copied
}

// New returns an empty output panel backed by a viewport.
func New() *Model {
	vp := viewport.New()
	vp.FillHeight = true
	return &Model{vp: vp, maxBody: defaultMaxBody, selected: -1, active: defaultActive, inactive: defaultInactive}
}

// SetTheme sets the border SGR colors for selected (active) and other (inactive)
// widgets. Empty values keep the current colors.
func (m *Model) SetTheme(active, inactive string) {
	if active != "" {
		m.active = active
	}
	if inactive != "" {
		m.inactive = inactive
	}
	m.refresh(false)
}

// SetFocus marks the panel focused or not; the selected widget only renders in
// the active color while the panel has focus.
func (m *Model) SetFocus(focused bool) {
	m.focused = focused
	m.refresh(false)
}

// Focused reports whether the output panel holds focus.
func (m *Model) Focused() bool { return m.focused }

// SetBanner sets a pre-rendered (ANSI-colored) logo banner pinned above all
// widgets as the first transcript element. It is a bootstrap-time "running"
// signal; see docs/ux/06_OUTPUT_WIDGET.md ("Logo banner"). An empty string clears
// it. The banner is rendered verbatim, clipped per line to the panel width.
func (m *Model) SetBanner(s string) {
	m.banner = s
	m.refresh(false)
}

// SetLaunchInfo installs a collapsed launch-info widget as the first widget of the
// transcript (after the banner, before any event). It is surface-local — never a
// session event — so it is not persisted and never appears on attached peer
// surfaces. names label the launchable surfaces (shown when expanded); commands are
// the matching full attach commands, revealed only on the clipboard via CopyCommand
// — never rendered, so the attach token stays off-screen. See
// docs/ux/06_OUTPUT_WIDGET.md (Launch-info widget).
func (m *Model) SetLaunchInfo(header string, names, commands []string) {
	n := min(len(names), len(commands))
	m.launchItems = make([]launchItem, n)
	for i := range n {
		m.launchItems[i] = launchItem{name: names[i], command: commands[i]}
	}
	m.copied = ""
	w := &widget{
		kind:        kindLaunch,
		header:      header,
		body:        m.launchBody(),
		collapsible: true,
		collapsed:   true,
	}
	m.widgets = append([]*widget{w}, m.widgets...)
	if m.selected >= 0 {
		m.selected++ // keep the selection pointing at the same widget
	}
	m.refresh(true)
}

// launchBody renders the launch-info widget body: an expand-time list of numbered
// surface names plus a copy hint, and a confirmation after a copy. Commands (and
// thus the token) are never included.
func (m *Model) launchBody() string {
	lines := []string{fmt.Sprintf("press 1-%d to copy a launch command to the clipboard:", len(m.launchItems)), ""}
	for i, it := range m.launchItems {
		lines = append(lines, fmt.Sprintf("  %d  %s", i+1, it.name))
	}
	if m.copied != "" {
		lines = append(lines, "", "✓ copied "+m.copied+" — paste in another terminal")
	}
	return strings.Join(lines, "\n")
}

// launchSelected reports whether the launch-info widget is the current selection.
func (m *Model) launchSelected() bool {
	return m.selected >= 0 && m.selected < len(m.widgets) && m.widgets[m.selected].kind == kindLaunch
}

// CopyCommand returns a command that copies the i-th (1-based) attach command to the
// system clipboard via OSC 52, when the launch-info widget is selected. ok is false
// when the launch widget isn't selected or i is out of range. The copied surface is
// confirmed in the widget body; the command itself is never displayed.
func (m *Model) CopyCommand(i int) (tea.Cmd, bool) {
	if !m.launchSelected() || i < 1 || i > len(m.launchItems) {
		return nil, false
	}
	item := m.launchItems[i-1]
	m.copied = item.name
	m.widgets[m.selected].body = m.launchBody()
	m.refresh(false)
	return tea.SetClipboard(item.command), true
}

// SetMaxBody sets the per-widget body-row cap (max_widget_lines).
func (m *Model) SetMaxBody(n int) {
	if n > 0 {
		m.maxBody = n
	}
	m.refresh(false)
}

// SetSize sets the panel's render dimensions and reflows content.
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 0)
	m.height = max(height, 0)
	m.vp.SetWidth(m.width)
	m.vp.SetHeight(m.height)
	m.refresh(false)
}

// Height returns the panel's row count.
func (m *Model) Height() int { return m.height }

// Update forwards scrolling messages (mouse wheel) to the transcript viewport.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return cmd
}

// Apply folds a bus event into the panel as a widget and selects it.
func (m *Model) Apply(ev state.Event) {
	if ev.EventType == "ERROR" {
		m.add(&widget{kind: kindError, header: "⚠ " + oneLine(eventText(ev)), body: detail(eventText(ev))})
		return
	}
	switch ev.ContentType {
	case state.ContentUserPrompt:
		m.add(&widget{kind: kindUser, header: "👤 You", body: eventText(ev)})
	case state.ContentClassification:
		m.add(&widget{kind: kindClassification, header: "⚙️ " + oneLine(eventText(ev))})
	case state.ContentAgentResponse:
		m.appendAssistant(eventText(ev))
		return
	case state.ContentThinking:
		m.appendThinking(eventText(ev))
		return
	case state.ContentToolCall:
		m.add(&widget{kind: kindToolCall, header: "🔧 " + ev.ToolName, body: eventText(ev), collapsible: true})
	case state.ContentToolResult:
		m.add(&widget{kind: kindToolResult, header: "📋 result", body: eventText(ev), collapsible: true, collapsed: true})
	case state.ContentSystemPrompt:
		m.add(&widget{kind: kindSystem, header: "📜 " + oneLine(eventText(ev)), body: detail(eventText(ev))})
	default:
		return
	}
}

// add appends a widget, makes it the selection, and pins the view to the bottom.
// Every widget that carries a body is collapsible, so Enter toggles expand/
// collapse uniformly across user, thinking, tool, and assistant widgets; the
// body is capped at max_widget_lines (SetMaxBody) regardless of kind.
func (m *Model) add(w *widget) {
	if w.body != "" {
		w.collapsible = true
	}
	m.widgets = append(m.widgets, w)
	m.selected = len(m.widgets) - 1
	m.refresh(true)
}

// appendAssistant streams text into a single assistant widget (its body).
func (m *Model) appendAssistant(text string) {
	if n := len(m.widgets); n > 0 && m.widgets[n-1].kind == kindAssistant {
		m.widgets[n-1].body += text
		m.refresh(true)
		return
	}
	m.add(&widget{kind: kindAssistant, header: "🤖 AgentX", body: text})
}

// appendThinking streams reasoning text into a single collapsed thinking widget
// (collapsed by default per the canonical output spec), creating it on first use.
func (m *Model) appendThinking(text string) {
	if n := len(m.widgets); n > 0 && m.widgets[n-1].kind == kindThinking {
		m.widgets[n-1].body += text
		m.refresh(true)
		return
	}
	m.add(&widget{kind: kindThinking, header: "💭 thinking", body: text, collapsible: true, collapsed: true})
}

// AssistantEntries returns the number of distinct assistant widgets.
func (m *Model) AssistantEntries() int {
	n := 0
	for _, w := range m.widgets {
		if w.kind == kindAssistant {
			n++
		}
	}
	return n
}

// ToggleCollapse flips the collapsed state of the i-th widget if collapsible.
func (m *Model) ToggleCollapse(i int) {
	if i < 0 || i >= len(m.widgets) || !m.widgets[i].collapsible {
		return
	}
	m.widgets[i].collapsed = !m.widgets[i].collapsed
	m.refresh(false)
}

// SelectUp moves the selection to the previous widget.
func (m *Model) SelectUp() { m.moveSelection(-1) }

// SelectDown moves the selection to the next widget.
func (m *Model) SelectDown() { m.moveSelection(1) }

func (m *Model) moveSelection(delta int) {
	if len(m.widgets) == 0 {
		return
	}
	m.selected = clampInt(m.selected+delta, 0, len(m.widgets)-1)
	m.refresh(false)
	m.scrollSelectedIntoView()
}

// ToggleSelected flips the collapsed state of the selected widget.
func (m *Model) ToggleSelected() {
	if m.selected >= 0 {
		m.ToggleCollapse(m.selected)
		m.scrollSelectedIntoView()
	}
}

// ScrollSelected scrolls the selected widget's body by n rows (positive = down).
func (m *Model) ScrollSelected(n int) {
	if m.selected < 0 {
		return
	}
	m.widgets[m.selected].offset += n
	m.refresh(false) // refresh clamps the offset against the body length
}

// ScrollUp/ScrollDown/PageUp/PageDown scroll the transcript as a whole.
func (m *Model) ScrollUp(n int)   { m.vp.ScrollUp(n) }
func (m *Model) ScrollDown(n int) { m.vp.ScrollDown(n) }
func (m *Model) PageUp()          { m.vp.ScrollUp(m.height) }
func (m *Model) PageDown()        { m.vp.ScrollDown(m.height) }

// View renders the transcript viewport.
func (m *Model) View() string {
	if m.height == 0 {
		return ""
	}
	return m.vp.View()
}

// blocks holds, per widget, its rendered lines and starting row in the transcript.
type blocks struct {
	lines  []string
	starts []int
	totals []int
}

// render builds every widget block and the flattened transcript. The logo
// banner, when set, is rendered first (above all widgets) and is not selectable;
// widget start rows are offset past it so selection/scroll math stays correct.
func (m *Model) render() blocks {
	var b blocks
	row := 0
	if m.banner != "" {
		bl := m.bannerLines()
		b.lines = append(b.lines, bl...)
		row += len(bl)
	}
	for i, w := range m.widgets {
		lines := m.renderWidget(w, i == m.selected)
		b.starts = append(b.starts, row)
		b.totals = append(b.totals, len(lines))
		b.lines = append(b.lines, lines...)
		row += len(lines)
	}
	return b
}

// bannerLines splits the banner and clips each line (ANSI-aware) to the panel
// width so embedded color is preserved without soft-wrapping the art.
func (m *Model) bannerLines() []string {
	lines := strings.Split(m.banner, "\n")
	if m.width <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if ansi.StringWidth(l) > m.width {
			l = ansi.Truncate(l, m.width, "")
		}
		out[i] = l
	}
	return out
}

// refresh re-renders the widgets into the viewport, optionally pinning the bottom.
func (m *Model) refresh(pinBottom bool) {
	atBottom := m.vp.AtBottom()
	b := m.render()
	m.vp.SetContent(strings.Join(b.lines, "\n"))
	if pinBottom || atBottom {
		m.vp.GotoBottom()
	}
}

// scrollSelectedIntoView scrolls the transcript so the selected widget is visible.
func (m *Model) scrollSelectedIntoView() {
	if m.selected < 0 {
		return
	}
	b := m.render()
	if m.selected >= len(b.starts) {
		return
	}
	top := b.starts[m.selected]
	bottom := top + b.totals[m.selected] - 1
	y := m.vp.YOffset()
	switch {
	case top < y:
		m.vp.SetYOffset(top)
	case bottom >= y+m.height:
		m.vp.SetYOffset(bottom - m.height + 1)
	}
}

// renderWidget renders one widget to its boxed lines.
func (m *Model) renderWidget(w *widget, selected bool) []string {
	innerW := m.width - 2
	if innerW < 1 {
		return []string{truncateWord(w.header, max(m.width, 0))}
	}

	rows := []string{padTo(truncateWord(w.header, innerW), innerW)}

	if !w.collapsed && w.body != "" {
		rows = append(rows, m.renderBody(w, innerW)...)
	}
	return m.boxify(rows, innerW, selected)
}

// renderBody wraps and windows a widget body, adding a proportional scrollbar
// column when the body exceeds the cap.
func (m *Model) renderBody(w *widget, innerW int) []string {
	lines := wrapLines(w.body, innerW)
	if len(lines) <= m.maxBody {
		w.offset = 0
		out := make([]string, len(lines))
		for i, l := range lines {
			out[i] = padTo(l, innerW)
		}
		return out
	}

	// Over the cap: reserve a scrollbar column and window the body.
	bodyW := innerW - 1
	lines = wrapLines(w.body, bodyW)
	total := len(lines)
	w.offset = clampInt(w.offset, 0, total-m.maxBody)
	window := lines[w.offset : w.offset+m.maxBody]

	out := make([]string, m.maxBody)
	for i, l := range window {
		out[i] = padTo(l, bodyW) + scrollbarCell(i, w.offset, total, m.maxBody)
	}
	return out
}

// scrollbarCell returns the scrollbar glyph for visible row i (thumb vs track),
// sized proportionally to the visible fraction of the content.
func scrollbarCell(i, offset, total, track int) string {
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

// boxify frames content rows (each already padded to innerW) in a box border;
// the selected widget gets a heavy border. Borders are colored: the selected
// widget uses the active color while the panel is focused, every other widget
// (and the selected one when the panel is unfocused) uses the inactive color.
func (m *Model) boxify(rows []string, innerW int, selected bool) []string {
	tl, tr, bl, br, h, v := "┌", "┐", "└", "┘", "─", "│"
	if selected {
		tl, tr, bl, br, h, v = "┏", "┓", "┗", "┛", "━", "┃"
	}
	code := m.inactive
	if selected && m.focused {
		code = m.active
	}
	paint := func(s string) string {
		if code == "" {
			return s
		}
		return "\x1b[" + code + "m" + s + "\x1b[0m"
	}
	bar := strings.Repeat(h, innerW)
	out := make([]string, 0, len(rows)+2)
	out = append(out, paint(tl+bar+tr))
	for _, r := range rows {
		out = append(out, paint(v)+r+paint(v))
	}
	out = append(out, paint(bl+bar+br))
	return out
}

// --- text helpers ---

// oneLine returns the first line of s.
func oneLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// detail returns s when it spans multiple lines (so it is worth expanding),
// otherwise "" (the header already shows it).
func detail(s string) string {
	if strings.Contains(s, "\n") {
		return s
	}
	return ""
}

// truncateWord fits s to w display columns, breaking at a word boundary and
// marking truncation with an ellipsis.
func truncateWord(s string, w int) string {
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

// wrapLines word-wraps s to w columns, preserving existing newlines.
func wrapLines(s string, w int) []string {
	if w <= 0 {
		return strings.Split(s, "\n")
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		out = append(out, strings.Split(ansi.Wrap(line, w, " -"), "\n")...)
	}
	return out
}

// padTo right-pads (or clips) s to exactly w display columns.
func padTo(s string, w int) string {
	width := ansi.StringWidth(s)
	if width == w {
		return s
	}
	if width > w {
		return ansi.Truncate(s, w, "")
	}
	return s + strings.Repeat(" ", w-width)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func eventText(ev state.Event) string {
	switch p := ev.Payload.(type) {
	case map[string]any:
		if t, ok := p["text"].(string); ok {
			return t
		}
	case string:
		return p
	}
	return fmt.Sprintf("%v", ev.Payload)
}
