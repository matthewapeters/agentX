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
	title       string // emoji + type label, rendered in the top border
	body        string // content, shown inside the box
	collapsible bool
	collapsed   bool
	// previewWhenCollapsed shows the first body line while collapsed (narrative
	// boxes). When false, a collapsed box shows only its titled border (noise boxes
	// such as thinking / tool result).
	previewWhenCollapsed bool
	offset               int // inner scroll offset (wrapped body-line index)
	// followTail keeps the body window pinned to the growing tail while the widget
	// streams (agent_delta / thinking chunks), so incoming text stays visible
	// without a manual scroll. A manual scroll up detaches it; scrolling back to the
	// bottom re-attaches. See nits.md #2 / UC-WIDGET-STREAM-FOLLOW.
	followTail bool
	// ordinal is the source event's durable identity; toggleable marks a user or
	// agent conversation element that the context surface can enable/disable;
	// disabled means it is withheld from the agent's upcoming context.
	ordinal    uint64
	toggleable bool
	disabled   bool
	// markdown renders the body with Tier-1 markdown emphasis (**bold**, `code`,
	// #/##/### headers) as terminal SGR. Set for model-authored assistant bodies;
	// left off for user prompts and tool output so their text stays literal.
	markdown bool
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

	launchItems   []launchItem    // attach commands behind the launch-info widget
	launchSession string          // this session's name, for the manual-launch footer
	copied        string          // name of the last surface whose command was copied
	connected     map[string]bool // launch kinds with a live attached surface (SS-4)

	collapseAll     bool // context surface: every element starts collapsed (summary)
	showToggleState bool // context surface: show an enabled checkbox on toggleables
}

// SetCollapseByDefault makes newly added elements start collapsed — the context
// surface's navigable-summary mode. The chat window leaves it off.
func (m *Model) SetCollapseByDefault(on bool) { m.collapseAll = on }

// SetShowToggleState makes toggleable elements render an enabled checkbox ([x]/[ ])
// left of their emoji — the context surface's management cue. The chat window,
// which does not toggle, leaves it off.
func (m *Model) SetShowToggleState(on bool) { m.showToggleState = on }

// SelectedToggleable returns the selected element's ordinal and current enabled
// state when it is a user/agent conversation element the context surface can
// toggle; ok is false otherwise (e.g. thinking/tool elements, or none selected).
func (m *Model) SelectedToggleable() (ordinal uint64, enabled, ok bool) {
	if m.selected < 0 || m.selected >= len(m.widgets) {
		return 0, false, false
	}
	w := m.widgets[m.selected]
	if !w.toggleable || w.ordinal == 0 {
		return 0, false, false
	}
	return w.ordinal, !w.disabled, true
}

// SetSelectedEnabled flips the selected element's local disabled state (optimistic
// UI after a toggle POST). It is a no-op when the selection is not toggleable.
func (m *Model) SetSelectedEnabled(enabled bool) {
	if m.selected < 0 || m.selected >= len(m.widgets) {
		return
	}
	w := m.widgets[m.selected]
	if !w.toggleable {
		return
	}
	w.disabled = !enabled
	m.refresh(false)
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
func (m *Model) SetLaunchInfo(header, session string, names, commands []string) {
	n := min(len(names), len(commands))
	m.launchItems = make([]launchItem, n)
	for i := range n {
		m.launchItems[i] = launchItem{name: names[i], command: commands[i]}
	}
	m.launchSession = session
	m.copied = ""
	w := &widget{
		kind:        kindLaunch,
		title:       header,
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
// surfaces, each `<digit> <status> <name>` (🟢 attached / 🔴 not), plus a copy hint
// and a confirmation after a copy. Commands (and thus the token) are never included.
func (m *Model) launchBody() string {
	lines := []string{fmt.Sprintf("press 1-%d to copy a launch command (clipboard support varies):", len(m.launchItems)), ""}
	for i, it := range m.launchItems {
		status := "🔴"
		if m.connected[it.name] {
			status = "🟢"
		}
		lines = append(lines, fmt.Sprintf("  %d  %s %s", i+1, status, it.name))
	}
	// Manual fallback: a user whose terminal drops OSC 52 can type this in another
	// pane, substituting a name above. It names the session so it stays correct when
	// more than one agentx session is running (SS-5).
	manual := "agentx surface launch <name>"
	if m.launchSession != "" {
		manual += " --session " + m.launchSession
	}
	lines = append(lines, "", "or run in another pane:  "+manual)
	if m.copied != "" {
		lines = append(lines, "", "✓ copied "+m.copied+" — paste in another terminal")
	}
	return strings.Join(lines, "\n")
}

// SetConnected updates which launch kinds currently have a live attached surface and
// re-renders the launch-info widget's status emojis (SS-4). It is a no-op when the
// connected set is unchanged or no launch-info widget is installed.
func (m *Model) SetConnected(kinds []string) {
	if sameStringSet(m.connected, kinds) {
		return
	}
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	m.connected = set
	if w := m.launchWidget(); w != nil {
		w.body = m.launchBody()
		m.refresh(false)
	}
}

// launchWidget returns the installed launch-info widget, or nil.
func (m *Model) launchWidget() *widget {
	for _, w := range m.widgets {
		if w.kind == kindLaunch {
			return w
		}
	}
	return nil
}

// sameStringSet reports whether set has exactly the kinds in the slice.
func sameStringSet(set map[string]bool, kinds []string) bool {
	if len(set) != len(kinds) {
		return false
	}
	for _, k := range kinds {
		if !set[k] {
			return false
		}
	}
	return true
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

// SetSize sets the panel's render dimensions and reflows content. The rightmost
// column is reserved as a transcript-scrollbar gutter, so content renders one column
// narrower than the panel.
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 0)
	m.height = max(height, 0)
	m.vp.SetWidth(m.contentWidth())
	m.vp.SetHeight(m.height)
	m.refresh(false)
}

// contentWidth is the panel width minus the transcript-scrollbar gutter column.
func (m *Model) contentWidth() int { return max(m.width-1, 0) }

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
		m.add(&widget{kind: kindError, title: "⚠ error", body: eventText(ev), previewWhenCollapsed: true})
		return
	}
	switch ev.ContentType {
	case state.ContentUserPrompt:
		m.add(&widget{kind: kindUser, title: "👤 You", body: eventText(ev), previewWhenCollapsed: true,
			ordinal: ev.Ordinal, toggleable: true, disabled: !ev.Enabled})
	case state.ContentClassification:
		// Plain gear (no VS16): its display width is a deterministic 1 cell, so the
		// titled border's right corner stays aligned on terminals that render the
		// emoji-presentation gear as a single column.
		m.add(&widget{kind: kindClassification, title: "⚙ classification", body: eventText(ev), previewWhenCollapsed: true})
	case state.ContentAgentDelta:
		// Live streaming chunk (chat window only): coalesce into an in-progress
		// assistant widget for the typing effect. The complete agent_response
		// finalizes it and stamps its identity.
		m.appendAssistant(eventText(ev))
		return
	case state.ContentAgentResponse:
		m.finalizeAssistant(eventText(ev), ev.Ordinal, ev.Enabled)
		return
	case state.ContentThinking:
		m.appendThinking(eventText(ev))
		return
	case state.ContentToolCall:
		m.add(&widget{kind: kindToolCall, title: "🔧 " + ev.ToolName, body: eventText(ev), collapsible: true, previewWhenCollapsed: true})
	case state.ContentToolResult:
		m.add(&widget{kind: kindToolResult, title: "📋 result", body: eventText(ev), collapsible: true, collapsed: true})
	case state.ContentSystemPrompt:
		m.add(&widget{kind: kindSystem, title: "📜 system prompt", body: eventText(ev), previewWhenCollapsed: true})
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
	// Collapse-all surfaces (the context navigable summary) start every element
	// collapsed regardless of kind.
	if m.collapseAll && w.collapsible {
		w.collapsed = true
	}
	m.widgets = append(m.widgets, w)
	m.selected = len(m.widgets) - 1
	m.refresh(true)
}

// appendAssistant streams a delta into a single in-progress assistant widget (its
// body). Used for the chat window's live agent_delta chunks only.
func (m *Model) appendAssistant(text string) {
	if n := len(m.widgets); n > 0 && m.widgets[n-1].kind == kindAssistant {
		m.widgets[n-1].body += text
		m.widgets[n-1].followTail = true // stream: keep the window on the newest text
		m.refresh(true)
		return
	}
	m.add(&widget{kind: kindAssistant, title: "🤖 AgentX", body: text, previewWhenCollapsed: true, toggleable: true, markdown: true})
}

// finalizeAssistant records the complete agent response as one conversation
// element. In the chat window it finalizes the widget the deltas built (stamping
// its identity); in the context surface, which sees no deltas, it adds the element.
func (m *Model) finalizeAssistant(text string, ordinal uint64, enabled bool) {
	if n := len(m.widgets); n > 0 && m.widgets[n-1].kind == kindAssistant {
		w := m.widgets[n-1]
		w.body = text
		w.ordinal = ordinal
		w.toggleable = true
		w.disabled = !enabled
		m.refresh(true)
		return
	}
	m.add(&widget{kind: kindAssistant, title: "🤖 AgentX", body: text, previewWhenCollapsed: true,
		ordinal: ordinal, toggleable: true, disabled: !enabled, markdown: true})
}

// appendThinking streams reasoning text into a single collapsed thinking widget
// (collapsed by default per the canonical output spec), creating it on first use.
func (m *Model) appendThinking(text string) {
	if n := len(m.widgets); n > 0 && m.widgets[n-1].kind == kindThinking {
		m.widgets[n-1].body += text
		m.widgets[n-1].followTail = true // stream: keep the window on the newest text
		m.refresh(true)
		return
	}
	m.add(&widget{kind: kindThinking, title: "💭 thinking", body: text, collapsible: true, collapsed: true})
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
	w := m.widgets[m.selected]
	w.offset += n
	if n < 0 {
		w.followTail = false // scrolling up detaches from the streaming tail
	}
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
	// Append the transcript scrollbar in the reserved right gutter: a thumb sized
	// and positioned by the viewport's scroll offset over the whole transcript, so
	// the user sees where the visible window sits within the whole. When everything
	// fits, the gutter is blank.
	lines := strings.Split(m.vp.View(), "\n")
	total := m.vp.TotalLineCount()
	track := len(lines)
	offset := m.vp.YOffset()
	for i := range lines {
		if total > track {
			lines[i] += scrollbarCell(i, offset, total, track)
		} else {
			lines[i] += " "
		}
	}
	return strings.Join(lines, "\n")
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
	cw := m.contentWidth()
	if cw <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if ansi.StringWidth(l) > cw {
			l = ansi.Truncate(l, cw, "")
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

// renderWidget renders one widget to its boxed lines: the kind label lives in the
// top border, and the inner rows are content. A collapsed box shows its first body
// line (narrative kinds) or nothing (noise kinds); an expanded box shows the capped,
// scrollable body.
func (m *Model) renderWidget(w *widget, selected bool) []string {
	// Classification is always a single line of metadata, so it renders flat (no
	// box): "⚙ classification · <intent → route>", tinted by selection like a border.
	if w.kind == kindClassification {
		return []string{m.flatLine(w, selected)}
	}

	innerW := m.contentWidth() - 2
	if innerW < 1 {
		return []string{truncateWord(w.title, m.contentWidth())}
	}

	// Style once (markdown → SGR) before wrapping, so the ANSI-aware wrap/pad math
	// measures true display width and the markers never count toward it.
	body := w.body
	if w.markdown {
		body = styleMarkdown(w.body)
	}

	var rows []string
	switch {
	case w.collapsed:
		if w.previewWhenCollapsed && w.body != "" {
			rows = []string{padTo(collapsedPreview(body, innerW), innerW)}
		}
	case w.body != "":
		rows = m.renderBody(w, body, innerW)
	}
	// On the context surface, toggleable elements carry an enabled checkbox to the
	// left of the emoji (☑ analogue): [x] enabled (in context) / [ ] disabled. This
	// is orthogonal to the selection border, so navigation and context-membership
	// read independently. The chat window leaves toggle state off.
	title := w.title
	if m.showToggleState && w.toggleable {
		box := "[x] "
		if w.disabled {
			box = "[ ] "
		}
		title = box + w.title
	}
	return m.boxify(title, rows, innerW, selected)
}

// flatLine renders a borderless one-row widget ("<title> · <body>"), tinted by the
// selection state the way a box border would be. Used for classification, whose
// payload is always a single line of metadata.
func (m *Model) flatLine(w *widget, selected bool) string {
	line := w.title
	if b := oneLine(w.body); b != "" {
		line += " · " + b
	}
	line = truncateWord(line, m.contentWidth())
	code := m.inactive
	if selected && m.focused {
		code = m.active
	}
	if code == "" {
		return line
	}
	return "\x1b[" + code + "m" + line + "\x1b[0m"
}

// collapsedPreview returns the first wrapped body line, marked with an ellipsis when
// more content follows.
func collapsedPreview(body string, innerW int) string {
	lines := wrapLines(body, innerW)
	if len(lines) == 0 {
		return ""
	}
	if len(lines) == 1 {
		return lines[0]
	}
	return truncateWord(lines[0], max(innerW-1, 0)) + "…"
}

// renderBody wraps and windows a widget body, adding a proportional scrollbar
// column when the body exceeds the cap.
func (m *Model) renderBody(w *widget, body string, innerW int) []string {
	lines := wrapLines(body, innerW)
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
	lines = wrapLines(body, bodyW)
	total := len(lines)
	maxOffset := total - m.maxBody
	if w.followTail {
		// Streaming / bottom-pinned: track the growing tail so new text stays visible.
		w.offset = maxOffset
	} else {
		w.offset = clampInt(w.offset, 0, maxOffset)
		// Scrolled to the bottom edge: re-attach so further growth keeps following.
		w.followTail = w.offset >= maxOffset
	}
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
func (m *Model) boxify(title string, rows []string, innerW int, selected bool) []string {
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
	out := make([]string, 0, len(rows)+2)
	out = append(out, m.topBorder(title, innerW, tl, tr, h, paint))
	for _, r := range rows {
		out = append(out, paint(v)+r+paint(v))
	}
	out = append(out, paint(bl+strings.Repeat(h, innerW)+br))
	return out
}

// topBorder draws the top border with the kind label embedded as `┌─ title ──┐`.
// The border glyphs take the focus color; the title keeps the default text color so
// it stays legible. A title that does not fit (very narrow box) is dropped.
func (m *Model) topBorder(title string, innerW int, tl, tr, h string, paint func(string) string) string {
	const lead = 3 // h + space before the title + space after it
	if title == "" || innerW-lead <= 0 {
		return paint(tl + strings.Repeat(h, innerW) + tr)
	}
	t := truncateWord(title, innerW-lead)
	fill := innerW - lead - ansi.StringWidth(t)
	if fill < 0 {
		fill = 0
	}
	return paint(tl+h+" ") + t + paint(" "+strings.Repeat(h, fill)+tr)
}

// --- text helpers ---

// oneLine returns the first line of s.
func oneLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
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

// Tier-1 markdown emphasis rendered as terminal SGR — enough to make LLM markdown
// read richly without pulling in a heavyweight renderer. Scope: inline **bold** and
// `code`, plus level 1..3 ATX headers (# / ## / ###). Source markers are consumed.
// These constants are the seed of a future emphasis/header theme (nits.md #6).
const (
	sgrReset = "\x1b[0m"
	sgrBold  = "\x1b[1m"
	sgrCode  = "\x1b[7m"   // reverse video: a theme-neutral inline-code cue
	sgrH1    = "\x1b[1;4m" // bold + underline
	sgrH2    = "\x1b[1m"   // bold
	sgrH3    = "\x1b[4m"   // underline
)

// styleMarkdown applies Tier-1 markdown emphasis line by line, preserving newlines so
// the existing ANSI-aware wrap/pad math still measures display width correctly.
func styleMarkdown(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = styleLine(line)
	}
	return strings.Join(lines, "\n")
}

// styleLine styles one source line: an ATX header (#{1,3} then a space) styles the
// whole line by level; any other line gets inline emphasis only.
func styleLine(line string) string {
	n := 0
	for n < len(line) && n < 3 && line[n] == '#' {
		n++
	}
	if n > 0 && n < len(line) && line[n] == ' ' {
		open := sgrH3
		switch n {
		case 1:
			open = sgrH1
		case 2:
			open = sgrH2
		}
		return open + styleInline(strings.TrimLeft(line[n+1:], " ")) + sgrReset
	}
	return styleInline(line)
}

// styleInline consumes **bold** and `code` spans left to right, emitting SGR and
// dropping the markers. Whichever delimiter opens first wins, so **stars** inside a
// `code` span stay literal. Unpaired or empty delimiters are left as written, so a
// mid-stream "**Age" renders plainly until its closing marker arrives.
func styleInline(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "**"):
			if end := strings.Index(s[i+2:], "**"); end > 0 {
				b.WriteString(sgrBold + s[i+2:i+2+end] + sgrReset)
				i += 2 + end + 2
				continue
			}
		case s[i] == '`':
			if end := strings.IndexByte(s[i+1:], '`'); end > 0 {
				b.WriteString(sgrCode + s[i+1:i+1+end] + sgrReset)
				i += 1 + end + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
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
