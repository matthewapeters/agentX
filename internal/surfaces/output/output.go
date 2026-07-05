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
	glamour "charm.land/glamour/v2"
	glansi "charm.land/glamour/v2/ansi"
	glstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
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
	// markdown renders the body with markdown emphasis as terminal SGR. Set for
	// model-authored assistant bodies; left off for user prompts and tool output so
	// their text stays literal. While streaming, the lightweight per-line scanner
	// styles it live; once finalized, the glamour renderer may upgrade it (ADR 0007).
	markdown bool
	// finalized marks an assistant widget whose complete answer has landed, so the
	// glamour renderer (when enabled) may replace the streaming scanner render.
	finalized bool
	// renderedBody caches the glamour render of body at renderedWidth. It is
	// invalidated (cleared) when the body changes or the target width does, so a
	// resize re-renders at the new width. See ADR 0007's width contract.
	renderedBody  string
	renderedWidth int
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
	mdMode          string // finalized-assistant renderer: "glamour", "native", or "" (scanner) — ADR 0007
}

// SetCollapseByDefault makes newly added elements start collapsed — the context
// surface's navigable-summary mode. The chat window leaves it off.
func (m *Model) SetCollapseByDefault(on bool) { m.collapseAll = on }

// SetShowToggleState makes toggleable elements render an enabled checkbox ([x]/[ ])
// left of their emoji — the context surface's management cue. The chat window,
// which does not toggle, leaves it off.
func (m *Model) SetShowToggleState(on bool) { m.showToggleState = on }

// SetMarkdownRenderer selects how a finalized assistant answer is rendered: "glamour"
// (full glamour document), "native" (scanner prose + GFM tables drawn directly with
// lipgloss/table — bordered, zebra-striped), or anything else for the plain scanner.
// The streaming path always uses the scanner regardless. Switching modes invalidates
// any cached render. See ADR 0007.
func (m *Model) SetMarkdownRenderer(mode string) {
	next := ""
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "glamour":
		next = "glamour"
	case "native":
		next = "native"
	}
	if next == m.mdMode {
		return
	}
	m.mdMode = next
	for _, w := range m.widgets {
		w.renderedBody, w.renderedWidth = "", 0
	}
	m.refresh(false)
}

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
		w.finalized = true
		w.renderedBody, w.renderedWidth = "", 0 // body changed: drop any stale glamour render
		m.refresh(true)
		return
	}
	m.add(&widget{kind: kindAssistant, title: "🤖 AgentX", body: text, previewWhenCollapsed: true,
		ordinal: ordinal, toggleable: true, disabled: !enabled, markdown: true, finalized: true})
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
	// measures true display width and the markers never count toward it. A finalized
	// assistant body upgrades to the configured renderer; the streaming path (not yet
	// finalized) always uses the lightweight scanner. Both the glamour and native
	// renderers are rendered to innerW-1, reserving the scrollbar gutter. See ADR 0007.
	body := w.body
	if w.markdown {
		switch {
		case w.finalized && m.mdMode == "glamour":
			body = m.glamourBody(w, innerW-1)
		case w.finalized && m.mdMode == "native":
			body = m.nativeBody(w, innerW-1)
		default:
			body = styleMarkdown(w.body)
		}
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
	sgrQuote = "\x1b[2m"   // dim: a recessed blockquote

	bulletGlyph = "•" // normalized unordered-list marker (-, *, + all fold to this)
	quoteGlyph  = "▎" // blockquote gutter marker
)

// glamourStyle is the dark glamour style with the document margin zeroed and its
// block prefix/suffix newlines dropped, so a rendered block fills the widget's inner
// width exactly (no left indent the box did not budget for) and carries no leading or
// trailing blank line that would waste a capped row. Built once.
var glamourStyle = func() glansi.StyleConfig {
	s := glstyles.DarkStyleConfig
	zero := uint(0)
	s.Document.Margin = &zero
	s.Document.BlockPrefix = ""
	s.Document.BlockSuffix = ""
	return s
}()

// glamourBody returns the glamour render of the widget's markdown body wrapped to
// width, cached on the widget. The cache is keyed by width so a resize (which changes
// the reserved width) re-renders; callers pass innerW-1 to reserve the vertical
// scrollbar gutter unconditionally, guaranteeing tables never need a horizontal
// scroll (ADR 0007). A render error falls back to the scanner styling.
func (m *Model) glamourBody(w *widget, width int) string {
	if width < 1 {
		return styleMarkdown(w.body)
	}
	if w.renderedWidth == width && w.renderedBody != "" {
		return w.renderedBody
	}
	w.renderedBody = renderGlamour(w.body, width)
	w.renderedWidth = width
	return w.renderedBody
}

// renderGlamour renders markdown source to ANSI at the given wrap width using the
// margin-zeroed dark style, trimming the outer blank lines. On any renderer error it
// degrades to the lightweight scanner so the body is never lost.
func renderGlamour(src string, width int) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(glamourStyle),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	)
	if err != nil {
		return styleMarkdown(src)
	}
	out, err := r.Render(src)
	if err != nil {
		return styleMarkdown(src)
	}
	return strings.Trim(out, "\n")
}

// Native-table styling: theme-neutral dark SGR indices. The zebra pair is a near-black
// and a dark-gray so body rows — and their wrapped continuation lines — stay visually
// distinct; the header gets a slightly lighter band. See ADR 0007 (drop-glamour path).
const (
	nativeTableBorder   = "240" // dark gray column/row rules
	nativeTableHeaderBg = "238"
	nativeZebraA        = "236" // dark gray
	nativeZebraB        = "233" // near black
)

// nativeBody returns the "native" render of the widget's markdown body wrapped to
// width, cached on the widget (keyed by width, like glamourBody). Callers pass innerW-1
// to reserve the scrollbar gutter (ADR 0007).
func (m *Model) nativeBody(w *widget, width int) string {
	if width < 1 {
		return styleMarkdown(w.body)
	}
	if w.renderedWidth == width && w.renderedBody != "" {
		return w.renderedBody
	}
	w.renderedBody = renderNative(w.body, width)
	w.renderedWidth = width
	return w.renderedBody
}

// renderNative styles prose with the per-line scanner and renders the two blocks the
// scanner cannot: GFM tables (drawn directly with lipgloss/table — bordered,
// zebra-striped) and fenced code (chroma syntax highlighting). This closes the gap
// with glamour without its full document renderer (ADR 0007). A table block is a
// pipe-delimited header row followed by a delimiter row; a code block is a ``` fence.
// Everything else falls through to styleLine.
func renderNative(src string, width int) string {
	lines := strings.Split(src, "\n")
	var out []string
	for i := 0; i < len(lines); {
		// Fenced code block: ```lang ... ``` → chroma-highlighted, wrapped to width.
		if lang, ok := fenceInfo(lines[i]); ok {
			var code []string
			j := i + 1
			for j < len(lines) && !isFence(lines[j]) {
				code = append(code, lines[j])
				j++
			}
			out = append(out, renderCodeBlock(strings.Join(code, "\n"), lang, width))
			if j < len(lines) { // consume the closing fence
				j++
			}
			i = j
			continue
		}
		// GFM table block.
		if i+1 < len(lines) && strings.Contains(lines[i], "|") && isTableDelimiter(lines[i+1]) {
			header := splitTableCells(lines[i])
			aligns := tableAligns(lines[i+1], len(header))
			var rows [][]string
			j := i + 2
			for j < len(lines) && strings.Contains(lines[j], "|") && strings.TrimSpace(lines[j]) != "" {
				rows = append(rows, padCells(splitTableCells(lines[j]), len(header)))
				j++
			}
			out = append(out, renderMarkdownTable(header, aligns, rows, width))
			i = j
			continue
		}
		out = append(out, styleLine(lines[i]))
		i++
	}
	return strings.Join(out, "\n")
}

// nativeCodeStyle is the chroma style used for fenced code blocks in the native
// renderer — a dark palette that reads on the panel's dark default.
const nativeCodeStyle = "catppuccin-mocha"

// fenceInfo reports whether line opens a fenced code block (``` optionally followed by
// an info string), returning the language token (the first word of the info string).
func fenceInfo(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "```") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(t, "```")), true
}

// isFence reports whether line is a ``` fence (used to find a block's closing fence).
func isFence(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "```")
}

// renderCodeBlock syntax-highlights code with chroma and hard-wraps each highlighted
// line to width so a long line never forces a horizontal scroll (ADR 0007's width
// contract). chroma colors the trailing newline of each line, which would bleed the
// code color into the panel's padding and border, so every emitted line is closed with
// an SGR reset. An empty or unhighlightable block degrades to the raw code.
func renderCodeBlock(code, lang string, width int) string {
	// Expand tabs to spaces first: a tab measures as ~0 cells to the ANSI width math
	// but the terminal expands it, which would drift the padding and right border.
	code = strings.ReplaceAll(code, "\t", "    ")
	highlighted := chromaHighlight(code, lang)
	var out []string
	for _, ln := range strings.Split(strings.TrimRight(highlighted, "\n"), "\n") {
		if width > 0 && ansi.StringWidth(ln) > width {
			ln = ansi.Hardwrap(ln, width, false)
		}
		for _, seg := range strings.Split(ln, "\n") {
			out = append(out, seg+sgrReset)
		}
	}
	return strings.Join(out, "\n")
}

// chromaHighlight tokenizes code (by language name, else auto-detected, else plain
// text) and formats it to 256-color ANSI. Any failure returns the code unstyled so a
// code block is never lost.
func chromaHighlight(code, lang string) string {
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	style := styles.Get(nativeCodeStyle)
	if style == nil {
		style = styles.Fallback
	}
	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Fallback
	}
	it, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}
	var b strings.Builder
	if err := formatter.Format(&b, style, it); err != nil {
		return code
	}
	return b.String()
}

// isTableDelimiter reports whether line is a GFM table delimiter row: every
// pipe-separated cell is dashes with an optional leading/trailing colon (alignment).
func isTableDelimiter(line string) bool {
	cells := splitTableCells(line)
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		c = strings.TrimSpace(c)
		c = strings.TrimPrefix(c, ":")
		c = strings.TrimSuffix(c, ":")
		if c == "" {
			return false
		}
		for _, r := range c {
			if r != '-' {
				return false
			}
		}
	}
	return true
}

// splitTableCells splits a pipe-delimited row into trimmed cells, dropping the optional
// outer pipes.
func splitTableCells(line string) []string {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	parts := strings.Split(s, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// tableAligns derives per-column alignment (0 left, 1 center, 2 right) from a delimiter
// row: a leading colon means left-anchored, both colons center, a trailing colon right.
func tableAligns(delim string, n int) []int {
	cells := splitTableCells(delim)
	a := make([]int, n)
	for i := 0; i < n; i++ {
		if i >= len(cells) {
			continue
		}
		c := strings.TrimSpace(cells[i])
		left, right := strings.HasPrefix(c, ":"), strings.HasSuffix(c, ":")
		switch {
		case left && right:
			a[i] = 1
		case right:
			a[i] = 2
		}
	}
	return a
}

// padCells normalizes a row to exactly n cells (truncating extras, padding shortfalls).
func padCells(cells []string, n int) []string {
	if len(cells) >= n {
		return cells[:n]
	}
	out := make([]string, n)
	copy(out, cells)
	return out
}

// renderMarkdownTable draws a GFM table with lipgloss/table: column + inter-row rules,
// a header band, and zebra body rows, bounded to width (wrapping long cells). This is
// what glamour's table renderer cannot express (it discards the row index and never
// enables BorderRow) — the affordance the drop-glamour path buys.
func renderMarkdownTable(header []string, aligns []int, rows [][]string, width int) string {
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(nativeTableBorder))
	t := table.New().
		Width(width).
		Wrap(true).
		Border(lipgloss.NormalBorder()).
		BorderStyle(borderStyle).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderRow(true).
		Headers(header...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			st := lipgloss.NewStyle().Padding(0, 1)
			if col >= 0 && col < len(aligns) {
				switch aligns[col] {
				case 1:
					st = st.Align(lipgloss.Center)
				case 2:
					st = st.Align(lipgloss.Right)
				}
			}
			if row == table.HeaderRow {
				return st.Bold(true).Background(lipgloss.Color(nativeTableHeaderBg))
			}
			if row%2 == 0 {
				return st.Background(lipgloss.Color(nativeZebraA))
			}
			return st.Background(lipgloss.Color(nativeZebraB))
		})
	return strings.TrimRight(t.String(), "\n")
}

// styleMarkdown applies Tier-1 markdown emphasis line by line, preserving newlines so
// the existing ANSI-aware wrap/pad math still measures display width correctly.
func styleMarkdown(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = styleLine(line)
	}
	return strings.Join(lines, "\n")
}

// styleLine styles one source line. An ATX header (#{1,3} then a space) styles the
// whole line by level. Otherwise, leading block markers are recognized (Tier 2):
// blockquotes (> ), unordered lists (-/*/+ ), and ordered lists (\d+. ). Their source
// markers are replaced with styled glyphs and the remainder gets inline emphasis. Any
// leading indentation is preserved so nested items keep their shape.
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

	indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
	rest := line[len(indent):]

	switch {
	// Blockquote: "> text" → a dim, gutter-marked line.
	case strings.HasPrefix(rest, "> "):
		return indent + sgrQuote + quoteGlyph + " " + styleInline(rest[2:]) + sgrReset
	// Unordered list: "-"/"*"/"+" then a space → a normalized bullet glyph.
	case len(rest) >= 2 && (rest[0] == '-' || rest[0] == '*' || rest[0] == '+') && rest[1] == ' ':
		return indent + sgrBold + bulletGlyph + sgrReset + " " + styleInline(rest[2:])
	}
	// Ordered list: "12. text" → keep the number, embolden the marker.
	if m := orderedMarker(rest); m > 0 {
		return indent + sgrBold + rest[:m] + sgrReset + " " + styleInline(rest[m+1:])
	}
	return styleInline(line)
}

// orderedMarker returns the length of a leading ordered-list marker ("12.") when s
// begins with one or more digits, a dot, and a trailing space; otherwise 0. The space
// at index m separates the marker from the content that starts at m+1.
func orderedMarker(s string) int {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(s) && s[i] == '.' && s[i+1] == ' ' {
		return i + 1
	}
	return 0
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
