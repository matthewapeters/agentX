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

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"

	"agentx/internal/state"
	"agentx/internal/surfaces/scrollutil"
)

// defaultMaxBody is the fallback body-row cap before a widget scrolls in place.
const defaultMaxBody = 20

type entryKind int

const (
	kindUser entryKind = iota
	kindClassification
	kindTaskDiag
	kindAssistant
	kindThinking
	kindToolCall
	kindToolResult
	kindSystem
	kindError
	kindLaunch
	kindPlan
	// kindApprovalRequest is a lightweight scrollback record of an interactive
	// decision the runtime asked for (tool approval, verb continuation, or any
	// future kind) — the same generic {prompt, options} event that also drives
	// the swapped-in approval widget in the input panel.
	kindApprovalRequest
	// kindApprovalDecision is the post-decision audit line: the prompt and the
	// chosen option's label as text. Fixed gray, not part of context.
	kindApprovalDecision
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
	// ordinal is the source event's durable identity; toggleable marks a user,
	// agent, or (flat, untagged) tool-call/tool-result conversation element that
	// the context surface can enable/disable; disabled means it is withheld from
	// the agent's upcoming context. Pinning a tool element is this same toggle.
	ordinal    uint64
	toggleable bool
	disabled   bool
	// markdown renders the body with markdown emphasis as terminal SGR. Set for
	// model-authored assistant bodies; left off for user prompts and tool output so
	// their text stays literal. While streaming, the lightweight per-line scanner
	// styles it live; once finalized, the native renderer may upgrade it (ADR 0007).
	markdown bool
	// finalized marks an assistant widget whose complete answer has landed, so the
	// native renderer (when enabled) may replace the streaming scanner render.
	finalized bool
	// renderedBody caches the native render of body at renderedWidth. It is
	// invalidated (cleared) when the body changes or the target width does, so a
	// resize re-renders at the new width. See ADR 0007's width contract.
	renderedBody  string
	renderedWidth int
	// nested indents the whole box under its parent (a plan step's 🔧 call and
	// 📋 result render as children of the 🗺 plan widget — ADR 0009 §9c).
	nested bool
	// planRoot identifies the plan tree this widget renders (kindPlan only) —
	// renderPlanWidget looks it up in Model.plans and draws it recursively at view
	// time (ADR 0009 §9c redesign: nested Step/Task boxes, not a flat text body).
	planRoot string
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
	mdMode          string // finalized-assistant renderer: "native" or "" (scanner) — ADR 0007

	plans map[string]*planState // live plan widgets by root task id (ADR 0009 §9c)
	// spinIndex routes a spinner.TickMsg to the plan node it belongs to — each
	// currently-running node owns an independent spinner.Model (see applyNodeEvent),
	// keyed by its own globally-unique spinner ID.
	spinIndex map[int]*planNode
}

// SetCollapseByDefault makes newly added elements start collapsed — the context
// surface's navigable-summary mode. The chat window leaves it off.
func (m *Model) SetCollapseByDefault(on bool) { m.collapseAll = on }

// SetShowToggleState makes toggleable elements render an enabled checkbox ([x]/[ ])
// left of their emoji — the context surface's management cue. The chat window,
// which does not toggle, leaves it off.
func (m *Model) SetShowToggleState(on bool) { m.showToggleState = on }

// SetMarkdownRenderer selects how a finalized assistant answer is rendered: "native"
// (scanner prose + GFM tables drawn directly with lipgloss/table — bordered,
// zebra-striped — and chroma-highlighted fenced code) or anything else for the plain
// scanner. The streaming path always uses the scanner regardless. Switching modes
// invalidates any cached render. See ADR 0007.
func (m *Model) SetMarkdownRenderer(mode string) {
	next := ""
	if strings.EqualFold(strings.TrimSpace(mode), "native") {
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
// state when it is a user/agent/tool-call/tool-result conversation element the
// context surface can toggle; ok is false otherwise (e.g. thinking/classification
// elements, or none selected).
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

// SelectedToolEvent returns the selected element's ordinal when it is a flat
// (untagged) tool_result widget — the class of element the context surface's Pin
// affordance (PD-CTX-AF-012, docs/ux/03_PANEL_DETAILS.md PD-WM) can act on. A
// tool_call widget, a plan-tagged tool_result folded into a Task node, or any
// other selection is not pinnable; ok is false for those.
func (m *Model) SelectedToolEvent() (ordinal uint64, ok bool) {
	if m.selected < 0 || m.selected >= len(m.widgets) {
		return 0, false
	}
	w := m.widgets[m.selected]
	if w.kind != kindToolResult || w.ordinal == 0 {
		return 0, false
	}
	return w.ordinal, true
}

// New returns an empty output panel backed by a viewport.
func New() *Model {
	vp := viewport.New()
	vp.FillHeight = true
	return &Model{vp: vp, maxBody: defaultMaxBody, selected: -1, active: defaultActive, inactive: defaultInactive,
		plans: map[string]*planState{}, spinIndex: map[int]*planNode{}}
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

// TotalContentLines returns the transcript's total rendered line count — the
// measure the chat surface's pinned banner (internal/surfaces/banner) uses to
// decide whether the applied content exceeds one screenful and should
// collapse. See docs/ux/06_OUTPUT_WIDGET.md ("Logo banner").
func (m *Model) TotalContentLines() int { return m.vp.TotalLineCount() }

// Update forwards scrolling messages (mouse wheel) to the transcript viewport, and
// routes a plan node's independent spinner tick (ADR 0009 §9c redesign — each
// concurrently-running node animates on its own, not in lockstep). A tick whose ID
// doesn't match any currently-running node (already completed, or never ours) is a
// harmless no-op — its Tick chain simply ends there.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		// msg.ID <= 0 never belongs to a real node spinner (startSpin's spinner.New()
		// always yields a positive, globally-unique ID) — defensive, since chat.go's
		// own forwarding already filters this, but Update has no other caller
		// guarantee to lean on.
		if msg.ID <= 0 {
			return nil
		}
		n, ok := m.spinIndex[msg.ID]
		if !ok || n.status != "running" || n.spin == nil {
			return nil
		}
		var cmd tea.Cmd
		*n.spin, cmd = n.spin.Update(msg)
		m.refresh(false) // a tick must not yank scroll position the way a new event does
		return cmd
	default:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return cmd
	}
}

// Apply folds a bus event into the panel as a widget and selects it. The returned
// tea.Cmd starts a plan node's spinner ticking on dispatch (ADR 0009 §9c redesign);
// every other path returns nil. client.SurfaceModel's Apply(ev) has no return value —
// callers outside the bundled chat surface (e.g. the context surface, which holds its
// own internal output.Model) may simply discard it, losing only the live spinner
// animation, not correctness.
func (m *Model) Apply(ev state.Event) tea.Cmd {
	if ev.EventType == "ERROR" {
		m.add(&widget{kind: kindError, title: "⚠ error", body: eventText(ev), previewWhenCollapsed: true})
		return nil
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
	case state.ContentTaskDiagnostic:
		// The task classifier's per-turn trace: three stage scores + outcome/reason.
		// A multi-line body, so it renders as a titled box (not the flat one-liner),
		// collapsed by default with a preview to keep the log unobtrusive.
		m.add(&widget{kind: kindTaskDiag, title: "⚙ task classify", body: eventText(ev), collapsible: true, collapsed: true, previewWhenCollapsed: true})
	case state.ContentAgentDelta:
		// Live streaming chunk (chat window only): coalesce into an in-progress
		// assistant widget for the typing effect. The complete agent_response
		// finalizes it and stamps its identity.
		m.appendAssistant(eventText(ev))
		return nil
	case state.ContentAgentResponse:
		m.finalizeAssistant(eventText(ev), ev.Ordinal, ev.Enabled)
		return nil
	case state.ContentThinking:
		m.appendThinking(eventText(ev))
		return nil
	case state.ContentToolCall:
		// A call tagged with a plan step's task id folds into that Task node's own
		// fields (ADR 0009 §9c redesign) — no separate widget. An untagged call
		// (single_tool cycle) renders flat as before, and — like a user/agent
		// element — carries an enabled checkbox: the context surface can pin it so
		// it folds into subsequent turns' assembled context (PD-CTX-AF-011).
		if tid := taskIDOf(ev); tid != "" && m.planFor(tid) != nil {
			m.applyTaskToolEvent(ev, false)
			return nil
		}
		m.add(&widget{kind: kindToolCall, title: "🔧 " + ev.ToolName, body: eventText(ev), collapsible: true, previewWhenCollapsed: true,
			ordinal: ev.Ordinal, toggleable: true, disabled: !ev.Enabled})
	case state.ContentToolResult:
		if tid := taskIDOf(ev); tid != "" && m.planFor(tid) != nil {
			m.applyTaskToolEvent(ev, true)
			return nil
		}
		m.add(&widget{kind: kindToolResult, title: "📋 result", body: eventText(ev), collapsible: true, collapsed: true,
			ordinal: ev.Ordinal, toggleable: true, disabled: !ev.Enabled})
	case state.ContentTaskPlan:
		// A plan gets ONE live widget, created at the "started" snapshot — before any
		// execution — and finalized by the "ended" snapshot (ADR 0009 §9c).
		m.applyPlanEvent(ev)
		return nil
	case state.ContentTaskNode:
		// Per-node delta: mutate the plan widget in place (dispatch/decompose/complete).
		return m.applyNodeEvent(ev)
	case state.ContentSystemPrompt:
		m.add(&widget{kind: kindSystem, title: "📜 system prompt", body: eventText(ev), previewWhenCollapsed: true})
	case state.ContentApprovalRequest:
		// A lightweight scrollback record of what was asked — the same event also
		// drives the swapped-in approval widget in the input panel (chat.go).
		p, _ := ev.Payload.(map[string]any)
		m.add(&widget{kind: kindApprovalRequest, title: "❓ approval needed",
			body: str(p["prompt"]), collapsible: true, previewWhenCollapsed: true})
	case state.ContentApprovalDecision:
		// The post-decision audit line: fixed gray, not toggleable (never part of
		// context — see grayLine/renderWidget's kindApprovalDecision dispatch).
		p, _ := ev.Payload.(map[string]any)
		body := "AgentX asked: " + str(p["prompt"]) + " → You selected: " + str(p["chosen_label"])
		m.add(&widget{kind: kindApprovalDecision, body: body, toggleable: false})
	default:
		return nil
	}
	return nil
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
		w.renderedBody, w.renderedWidth = "", 0 // body changed: drop any stale native render
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
	m.selected = scrollutil.ClampInt(m.selected+delta, 0, len(m.widgets)-1)
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
			lines[i] += scrollutil.ScrollbarCell(i, offset, total, track)
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

// render builds every widget block and the flattened transcript.
func (m *Model) render() blocks {
	var b blocks
	row := 0
	for i, w := range m.widgets {
		lines := m.renderWidget(w, i == m.selected)
		b.starts = append(b.starts, row)
		b.totals = append(b.totals, len(lines))
		b.lines = append(b.lines, lines...)
		row += len(lines)
	}
	return b
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
	// A plan renders as a recursive nested-box DAG, not a flat text body — drawn at
	// view time (not baked into w.body at event time) so a resize is correct for
	// free (ADR 0009 §9c redesign).
	if w.kind == kindPlan {
		return m.renderPlanWidget(w, selected)
	}
	// Classification is always a single line of metadata, so it renders flat (no
	// box): "⚙ classification · <intent → route>", tinted by selection like a border.
	if w.kind == kindClassification {
		return []string{m.flatLine(w, selected)}
	}
	// The approval-decision audit line renders flat and a fixed gray — unlike
	// flatLine's selection-tinted active/inactive, this is never part of context
	// and never draws attention, so its color doesn't change with focus/selection.
	if w.kind == kindApprovalDecision {
		return []string{grayLine(w, m.contentWidth())}
	}

	// A nested widget (a plan step's tool call/result) indents two columns under its
	// parent box, narrowing its own frame to match (ADR 0009 §9c).
	const nestIndent = "  "
	indentW := 0
	if w.nested {
		indentW = len(nestIndent)
	}
	innerW := m.contentWidth() - 2 - indentW
	if innerW < 1 {
		return []string{scrollutil.TruncateWord(w.title, m.contentWidth())}
	}

	// Style once (markdown → SGR) before wrapping, so the ANSI-aware wrap/pad math
	// measures true display width and the markers never count toward it. A finalized
	// assistant body upgrades to the native renderer; the streaming path (not yet
	// finalized) always uses the lightweight scanner. The native renderer is rendered
	// to innerW-1, reserving the scrollbar gutter. See ADR 0007.
	body := w.body
	if w.markdown {
		switch {
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
			rows = []string{scrollutil.PadTo(collapsedPreview(body, innerW), innerW)}
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
	lines := m.boxify(title, rows, innerW, selected)
	if w.nested {
		for i := range lines {
			lines[i] = nestIndent + lines[i]
		}
	}
	return lines
}

// flatLine renders a borderless one-row widget ("<title> · <body>"), tinted by the
// selection state the way a box border would be. Used for classification, whose
// payload is always a single line of metadata.
func (m *Model) flatLine(w *widget, selected bool) string {
	line := w.title
	if b := oneLine(w.body); b != "" {
		line += " · " + b
	}
	line = scrollutil.TruncateWord(line, m.contentWidth())
	code := m.inactive
	if selected && m.focused {
		code = m.active
	}
	if code == "" {
		return line
	}
	return "\x1b[" + code + "m" + line + "\x1b[0m"
}

// grayLine renders a borderless one-row widget in a fixed gray (SGR 90) —
// unlike flatLine, never tinted by selection/focus — matching the muted
// styling input.go's dimHint uses for non-content affordances. Used for the
// approval-decision audit line, which should never draw attention.
func grayLine(w *widget, width int) string {
	return "\x1b[90m" + scrollutil.TruncateWord(oneLine(w.body), width) + "\x1b[0m"
}

// collapsedPreview returns the first wrapped body line, marked with an ellipsis when
// more content follows.
func collapsedPreview(body string, innerW int) string {
	lines := scrollutil.WrapLines(body, innerW)
	if len(lines) == 0 {
		return ""
	}
	if len(lines) == 1 {
		return lines[0]
	}
	return scrollutil.TruncateWord(lines[0], max(innerW-1, 0)) + "…"
}

// renderBody wraps and windows a widget body, adding a proportional scrollbar
// column when the body exceeds the cap.
func (m *Model) renderBody(w *widget, body string, innerW int) []string {
	lines := scrollutil.WrapLines(body, innerW)
	if len(lines) <= m.maxBody {
		w.offset = 0
		out := make([]string, len(lines))
		for i, l := range lines {
			out[i] = scrollutil.PadTo(l, innerW)
		}
		return out
	}

	// Over the cap: reserve a scrollbar column and window the body.
	bodyW := innerW - 1
	lines = scrollutil.WrapLines(body, bodyW)
	total := len(lines)
	maxOffset := total - m.maxBody
	if w.followTail {
		// Streaming / bottom-pinned: track the growing tail so new text stays visible.
		w.offset = maxOffset
	} else {
		w.offset = scrollutil.ClampInt(w.offset, 0, maxOffset)
		// Scrolled to the bottom edge: re-attach so further growth keeps following.
		w.followTail = w.offset >= maxOffset
	}
	window := lines[w.offset : w.offset+m.maxBody]

	out := make([]string, m.maxBody)
	for i, l := range window {
		out[i] = scrollutil.PadTo(l, bodyW) + scrollutil.ScrollbarCell(i, w.offset, total, m.maxBody)
	}
	return out
}

// boxify frames content rows (each already padded to innerW) in a box border;
// the selected widget gets a heavy border. Borders are colored: the selected
// widget uses the active color while the panel is focused, every other widget
// (and the selected one when the panel is unfocused) uses the inactive color.
func (m *Model) boxify(title string, rows []string, innerW int, selected bool) []string {
	code := m.inactive
	if selected && m.focused {
		code = m.active
	}
	return m.boxifyStyled(title, rows, innerW, code, selected)
}

// boxifyStyled is boxify's underlying primitive with an explicit border color
// (empty = undyed) and heaviness, independent of the panel's selected/inactive
// two-way logic — the plan widget's nested Step/Task boxes use this directly so
// color can carry kind/running-state information instead (ADR 0009 §9c redesign).
func (m *Model) boxifyStyled(title string, rows []string, innerW int, colorCode string, heavy bool) []string {
	tl, tr, bl, br, h, v := "┌", "┐", "└", "┘", "─", "│"
	if heavy {
		tl, tr, bl, br, h, v = "┏", "┓", "┗", "┛", "━", "┃"
	}
	paint := func(s string) string {
		if colorCode == "" {
			return s
		}
		return "\x1b[" + colorCode + "m" + s + "\x1b[0m"
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
	t := scrollutil.TruncateWord(title, innerW-lead)
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
// width, cached on the widget (keyed by width, so a resize re-renders). Callers pass innerW-1
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
