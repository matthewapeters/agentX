package chat

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"agentx/internal/state"
	"agentx/internal/surfaces/approval"
	"agentx/internal/surfaces/banner"
	"agentx/internal/surfaces/input"
	"agentx/internal/surfaces/output"
)

// hintHeight is the single context-sensitive hint/command row beneath the status
// bar. The input panel's height is dynamic (it grows with wrapped content up to its
// max), so the output panel takes whatever height remains.
const hintHeight = 1

// flashDuration is how long the input border stays highlighted after a history
// navigation hits a boundary (PD-02-AF-015).
const flashDuration = 150 * time.Millisecond

// focus identifies which panel currently receives navigation/edit keys.
type focus int

const (
	// focusInput is the default: keystrokes edit the prompt.
	focusInput focus = iota
	// focusOutput: keystrokes navigate and scroll the output widgets.
	focusOutput
)

// defaultActive and defaultInactive are the built-in panel-border SGR colors
// used until SetTheme overrides them (cyan / dark gray, matching config).
const (
	defaultActive   = "38;5;6"
	defaultInactive = "38;5;240"
)

// defaultAttention is the input-panel border color while an interactive decision
// is awaiting the user (bold yellow) — distinct from active/inactive/flash so a
// pending approval visually stands out from ordinary focus state.
const defaultAttention = "1;33"

// approvalTitleBase is the border title shown while the swapped-in approval
// widget occupies the input panel's slot — the same wording regardless of what
// decision is being asked for (tool approval, verb continuation, or any future
// kind), per the "consistent behavior" requirement.
const approvalTitleBase = "AgentX Needs Your Input"

// approvalPanelTitle appends a "(1 of N)" queue-depth suffix when more than
// one decision is pending — always position 1, since decisionGate only ever
// shows the front of its FIFO queue. Unchanged for the common single-pending
// case, so this adds no clutter when there's nothing to disambiguate.
func approvalPanelTitle(pending int) string {
	if pending <= 1 {
		return approvalTitleBase
	}
	return fmt.Sprintf("%s (1 of %d)", approvalTitleBase, pending)
}

// decodeInt reads an int payload field that may arrive as a native Go int
// (in-process chat surface) or a JSON-decoded float64 (a remote surface
// attached over transport) — the same dual-mode robustness
// state.DecodeApprovalOptions already demonstrates for the sibling options
// field on the same event.
func decodeInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}

// ProcessingStateMsg delivers a processing-state update to the chat surface.
type ProcessingStateMsg state.ProcessingState

// EventMsg delivers a bus event to the chat surface for rendering.
type EventMsg state.Event

// flashOffMsg ends an input-border flash; scheduled by flashCmd.
type flashOffMsg struct{}

// Bridge connects the chat surface to the runtime: a Submit function for prompts,
// a Stop function that interrupts the in-flight prompt, and the channels the
// surface listens on for events and processing-state. A zero Bridge (nil fields)
// leaves the surface in local-echo mode for unit tests.
type Bridge struct {
	Submit func(text string)
	Stop   func()
	// Approve sends a decision string back to whichever interactive-decision
	// request is currently shown (tool approval, verb continuation, or any future
	// kind) — the runtime's shared decision gate resolves it generically.
	Approve    func(decision string)
	Events     <-chan state.Event
	Processing <-chan state.ProcessingState
	// Connected returns the surface kinds currently attached (SS-4). When set, the
	// surface polls it to drive the launch-info status emojis. Nil disables polling.
	Connected func() []string
}

// Model is the chat surface Bubble Tea model. It composes an output panel and an
// input panel separated by a status bar that doubles as the processing-state
// indicator.
type Model struct {
	width        int
	height       int
	banner       *banner.Model
	output       *output.Model
	input        *input.Model
	// approval is a third panel inserted between output and input while
	// proc.State == StateAwaitingInput: a generic prompt + navigable option
	// list, populated from the runtime's approval_request events. Input stays
	// visible (inert) rather than being replaced, so there is no swap and no
	// frame where a bottom panel goes missing.
	approval     *approval.Model
	// pending is the decision queue depth at the last APPROVAL_REQUEST event,
	// including the one currently shown — always >= 1 while proc.State is
	// StateAwaitingInput, meaningless otherwise. Drives the approval panel
	// title's "(1 of N)" suffix when > 1, so a queued-behind-this-one decision
	// is never a surprise (docs/architecture/behavior/
	// chat_pending_approval_count.feature.md).
	pending      int
	proc         state.ProcessingState
	spinner      spinner.Model
	focus        focus
	chordPending bool // an ESC was pressed; the next key resolves the chord
	flashing     bool // the input border is flashing a history boundary
	active       string
	inactive     string
	attention    string
	bridge       *Bridge
}

// New returns an unwired chat surface model (input focused, idle status). Submit
// echoes locally; used by unit tests.
func New() Model {
	out := output.New()
	out.SetTheme(defaultActive, defaultInactive)
	out.SetFocus(false)
	return Model{
		banner:    banner.New(),
		output:    out,
		input:     input.New(),
		approval:  approval.New(),
		proc:      state.ProcessingState{State: state.StateIdle, Phase: state.PhaseNone},
		spinner:   spinner.New(spinner.WithSpinner(spinner.Dot)),
		focus:     focusInput,
		active:    defaultActive,
		inactive:  defaultInactive,
		attention: defaultAttention,
	}
}

// NewWithBridge returns a chat surface wired to the runtime: prompts are routed
// through bridge.Submit and events/processing-state are consumed from its
// channels.
func NewWithBridge(b Bridge) Model {
	m := New()
	m.bridge = &b
	return m
}

// Init implements tea.Model: a wired surface starts listening to its runtime.
func (m Model) Init() tea.Cmd {
	if m.bridge == nil {
		return nil
	}
	cmds := []tea.Cmd{m.listenEvents(), m.listenProcessing()}
	if c := m.pollConnections(); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

// connPollInterval is how often the surface refreshes peer-connection status.
const connPollInterval = time.Second

// connTickMsg requests a connection-status refresh.
type connTickMsg struct{}

// pollConnections schedules the next connection-status refresh, or nil when the
// bridge does not expose connection status (transport disabled / unit tests).
func (m Model) pollConnections() tea.Cmd {
	if m.bridge == nil || m.bridge.Connected == nil {
		return nil
	}
	return tea.Tick(connPollInterval, func(time.Time) tea.Msg { return connTickMsg{} })
}

// SetMaxWidgetLines configures the output widgets' body-row cap.
func (m Model) SetMaxWidgetLines(n int) { m.output.SetMaxBody(n) }

// SetMaxInputLines caps how tall the input panel grows before it scrolls.
func (m Model) SetMaxInputLines(n int) { m.input.SetMaxHeight(n) }

// SetMarkdownRenderer selects how assistant markdown is styled in the output panel:
// "native" for the full finalize-time render (tables + highlighted code), anything
// else for the lightweight per-line scanner (ADR 0007).
func (m Model) SetMarkdownRenderer(mode string) { m.output.SetMarkdownRenderer(mode) }

// SetLaunchInfo installs the collapsed launch-info widget (attach commands for peer
// surfaces) as the first output widget. names label the surfaces; commands are the
// matching attach commands, reachable only via digit-copy; session names this
// session for the manual-launch footer. See docs/ux/06_OUTPUT_WIDGET.md.
func (m Model) SetLaunchInfo(header, session string, names, commands []string) {
	m.output.SetLaunchInfo(header, session, names, commands)
}

// SetTheme sets the focus-border SGR colors for the panels and output widgets.
// Empty values keep the built-in defaults.
func (m *Model) SetTheme(active, inactive string) {
	if active != "" {
		m.active = active
	}
	if inactive != "" {
		m.inactive = inactive
	}
	m.output.SetTheme(m.active, m.inactive)
}

// setFocus moves focus between the input and output panels, keeping the panels'
// own focus flags in sync so borders and key routing agree.
func (m *Model) setFocus(f focus) {
	m.focus = f
	if f == focusOutput {
		m.input.Blur()
		m.output.SetFocus(true)
	} else {
		m.input.Focus()
		m.output.SetFocus(false)
	}
}

// Output returns the output panel.
func (m Model) Output() *output.Model { return m.output }

// Input returns the input panel.
func (m Model) Input() *input.Model { return m.input }

// Banner returns the pinned logo banner.
func (m Model) Banner() *banner.Model { return m.banner }

// Update implements tea.Model: it handles resize and the quit key, and routes
// other keys to the focused panel (panel key handling lands in CHT-B3).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()
	case ProcessingStateMsg:
		prev := m.proc.State
		m.proc = state.ProcessingState(msg)
		m.input.SetStreaming(m.proc.State == state.StateWorking)
		m.banner.SetLabel(bannerLabel(m.proc))
		cmds := []tea.Cmd{m.listenProcessing()}
		// Start the spinner and the banner's rainbow-wave animation when work
		// begins; each sustains itself via its own TickMsg loop.
		if m.proc.State == state.StateWorking && prev != state.StateWorking {
			cmds = append(cmds, m.spinner.Tick)
			if c := m.banner.SetAnimating(true); c != nil {
				cmds = append(cmds, c)
			}
		}
		// Cancel a pending chord and revert the banner to its static coloring
		// once work ends.
		if m.proc.State != state.StateWorking {
			m.chordPending = false
			m.banner.SetAnimating(false)
		}
		// The approval widget's height differs from the free-text input's; resize
		// the input slot whenever a decision starts or stops being awaited.
		if (m.proc.State == state.StateAwaitingInput) != (prev == state.StateAwaitingInput) {
			m.relayout()
		}
		return m, tea.Batch(cmds...)
	case spinner.TickMsg:
		// spinner.TickMsg carries a globally-unique ID (one process-wide counter, per
		// bubbles) — a tick that isn't chat's own status spinner belongs to one of the
		// output panel's independent per-plan-node spinners (ADR 0009 §9c redesign);
		// forward it there instead of silently swallowing it. The ID>0 guard matters:
		// a real spinner.Tick() always carries a positive ID, but a hand-built
		// spinner.TickMsg{} (id 0, e.g. a test fixture) is not owned by anything —
		// forwarding it anyway reproduced a genuine hang under the full godog suite's
		// concurrent scenario execution (isolated single-scenario runs never hit it),
		// so a zero ID is treated as un-owned and left for chat's own spinner branch
		// below rather than routed anywhere.
		if msg.ID > 0 && msg.ID != m.spinner.ID() {
			return m, m.output.Update(msg)
		}
		if m.proc.State != state.StateWorking {
			return m, nil // stop ticking once work finishes
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case banner.TickMsg:
		return m, m.banner.Tick(msg)
	case EventMsg:
		ev := state.Event(msg)
		if ev.ContentType == state.ContentApprovalRequest {
			if p, ok := ev.Payload.(map[string]any); ok {
				prompt, _ := p["prompt"].(string)
				m.approval.Set(prompt, state.DecodeApprovalOptions(p["options"]))
				m.pending = decodeInt(p["pending"])
			}
		}
		cmd := m.output.Apply(ev)
		// New content may cross the banner's collapse threshold, so re-derive
		// layout (also covers the approval-request resize above).
		m.relayout()
		return m, tea.Batch(cmd, m.listenEvents())
	case connTickMsg:
		if m.bridge != nil && m.bridge.Connected != nil {
			m.output.SetConnected(m.bridge.Connected())
		}
		return m, m.pollConnections()
	case flashOffMsg:
		m.flashing = false
		return m, nil
	case tea.KeyboardEnhancementsMsg:
		// The terminal answered the enhancement query: if it disambiguates modified
		// keys, Shift+Enter will reach us distinctly, so advertise it as the
		// soft-newline. Otherwise the terminal-agnostic Alt+Enter default stands.
		// (Terminals without support never send this message.)
		if msg.SupportsKeyDisambiguation() {
			m.input.SetNewlineKey("shift+enter")
		}
		return m, nil
	case tea.MouseWheelMsg:
		// Forwarded to the output viewport; only delivered when the program
		// enables the mouse (off by default to preserve native text selection).
		return m, m.output.Update(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.PasteMsg:
		// Bracketed-paste content arrives as its own message type, distinct from
		// KeyPressMsg, so it needs its own route to the input panel. Only live
		// when the input actually owns the keyboard (same condition the border
		// highlight at m.frame(m.input...) uses): not while an approval decision
		// has swapped in its own widget, and not while output has focus.
		if m.focus == focusInput && m.proc.State != state.StateAwaitingInput {
			m.input.Paste(msg.Content)
		}
		return m, nil
	}
	return m, nil
}

// handleKey routes a key press: global quit first, then a pending ESC chord,
// then the focus-aware navigation/edit keys. ESC is a leader key — ESC,q quits;
// ESC,↑ focuses the output; ESC,↓ focuses the input; ESC,ESC interrupts an
// in-flight response.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	// Resolve a pending ESC chord with the follow-up key.
	if m.chordPending {
		m.chordPending = false
		switch key {
		case "q":
			return m, tea.Quit
		case "up":
			m.setFocus(focusOutput)
			return m, nil
		case "down":
			m.setFocus(focusInput)
			return m, nil
		case "esc", "escape":
			// ESC,ESC interrupts while working; idle, it clears an active
			// history seed; otherwise it cancels the chord.
			if m.proc.State == state.StateWorking {
				return m, m.stopCmd()
			}
			if m.focus == focusInput && m.input.Seeded() {
				m.input.ClearSeed()
			}
			return m, nil
		default:
			return m, nil // unknown chord: cancel
		}
	}

	// PgUp/PgDn always jump focus into the output and move the selection.
	switch key {
	case "pgup":
		m.setFocus(focusOutput)
		m.output.SelectUp()
		return m, nil
	case "pgdown":
		m.setFocus(focusOutput)
		m.output.SelectDown()
		return m, nil
	}

	// ESC opens a chord in any state.
	if key == "esc" || key == "escape" {
		m.chordPending = true
		return m, nil
	}

	// Awaiting an interactive decision: the swapped-in approval widget owns the
	// key — navigate its option list and confirm with Enter. One code path
	// regardless of which kind of decision is being asked for (tool approval,
	// verb continuation, or any future kind): the widget renders whatever
	// prompt/options it was handed and just echoes back the chosen Decision.
	if m.proc.State == state.StateAwaitingInput {
		if m.approval.Update(msg) == approval.ActionConfirm {
			return m, m.approveCmd(m.approval.Selected().Decision)
		}
		return m, nil
	}

	// While working, the input is disabled; only the chord above acts.
	if m.proc.State == state.StateWorking {
		return m, nil
	}

	// Output focus: navigate and scroll the selected widget.
	if m.focus == focusOutput {
		switch key {
		case "up", "k":
			m.output.ScrollSelected(-1)
		case "down", "j":
			m.output.ScrollSelected(1)
		case "ctrl+o", "enter":
			m.output.ToggleSelected()
		default:
			// A digit copies the matching attach command from the selected
			// launch-info widget to the clipboard (no-op for any other widget).
			if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
				if cmd, ok := m.output.CopyCommand(int(key[0] - '0')); ok {
					return m, cmd
				}
			}
		}
		return m, nil
	}

	// Input focus: edit the prompt or navigate prompt history. The edit may change
	// the wrapped input height, so relayout after it settles (input grows, output
	// shrinks, and vice versa).
	action := m.input.Update(msg)
	var cmd tea.Cmd
	switch action {
	case input.ActionSubmit:
		text := m.input.Value()
		m.input.Reset()
		m.relayout()
		if m.bridge != nil && m.bridge.Submit != nil {
			// Route to the runtime; the user prompt and response arrive as
			// EventMsgs and render through the output panel.
			return m, m.submitCmd(text)
		}
		// Unwired: echo locally so the surface is usable in isolation.
		cmd = m.output.Apply(state.Event{
			ContentType: state.ContentUserPrompt,
			Payload:     map[string]any{"text": text},
		})
	case input.ActionHistoryBoundary:
		// History navigation hit an edge; flash the input border.
		m.flashing = true
		return m, m.flashCmd()
	}
	m.relayout()
	return m, cmd
}

// flashCmd schedules the end of an input-border flash.
func (m Model) flashCmd() tea.Cmd {
	return tea.Tick(flashDuration, func(time.Time) tea.Msg { return flashOffMsg{} })
}

// approveCmd sends a decision string to the runtime, resolving whichever
// interactive-decision request is currently shown (the widget's Selected().Decision).
func (m Model) approveCmd(decision string) tea.Cmd {
	if m.bridge == nil || m.bridge.Approve == nil {
		return nil
	}
	approve := m.bridge.Approve
	return func() tea.Msg {
		approve(decision)
		return nil
	}
}

// stopCmd asks the runtime to interrupt the in-flight prompt.
func (m Model) stopCmd() tea.Cmd {
	if m.bridge == nil || m.bridge.Stop == nil {
		return nil
	}
	stop := m.bridge.Stop
	return func() tea.Msg {
		stop()
		return nil
	}
}

// submitCmd returns a command that hands the prompt to the runtime.
func (m Model) submitCmd(text string) tea.Cmd {
	submit := m.bridge.Submit
	return func() tea.Msg {
		submit(text)
		return nil
	}
}

// listenEvents returns a command that waits for the next runtime event.
func (m Model) listenEvents() tea.Cmd {
	if m.bridge == nil || m.bridge.Events == nil {
		return nil
	}
	ch := m.bridge.Events
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return EventMsg(ev)
	}
}

// listenProcessing returns a command that waits for the next processing-state.
func (m Model) listenProcessing() tea.Cmd {
	if m.bridge == nil || m.bridge.Processing == nil {
		return nil
	}
	ch := m.bridge.Processing
	return func() tea.Msg {
		ps, ok := <-ch
		if !ok {
			return nil
		}
		return ProcessingStateMsg(ps)
	}
}

// relayout sizes the panels to the current terminal. Each panel is wrapped in a
// focus border (2 rows, 2 columns); a status row and a hint row sit between the
// bordered output (which fills the rest) and the bordered, fixed-height input.
// While awaiting an interactive decision, the approval widget is inserted as a
// THIRD region between output and input — input stays visible (inert) rather
// than being swapped out, so there is never a frame where neither the output
// nor a bottom panel is rendered. The pinned logo banner is a FOURTH,
// borderless region above the output panel (docs/ux/06_OUTPUT_WIDGET.md "Logo
// banner") — carved out of the same height budget, never part of the output
// viewport's scrollable content. relayout also re-checks the banner's
// content-based collapse trigger on every call, since both a resize and new
// output content can cross it.
func (m *Model) relayout() {
	innerW := m.width - 2
	if innerW < 0 {
		innerW = 0
	}
	// Set the input width first so its desired (wrapped) height reflects this
	// width, then give the input that height and the output the remaining
	// space. This runs on every content change, so the input grows and the
	// output shrinks as text wraps.
	m.input.SetSize(innerW, 0)
	inputH := m.input.DesiredHeight()

	awaiting := m.proc.State == state.StateAwaitingInput
	var approvalH int
	if awaiting {
		m.approval.SetSize(innerW, 0)
		approvalH = m.approval.DesiredHeight()
	}
	// Chrome: output border (2) + status (1) + hint (1) + input border (2), plus
	// the approval panel's own border (2) when it's showing.
	chrome := 2 + 1 + hintHeight + 2
	if awaiting {
		chrome += 2
	}

	m.banner.SetWidth(innerW)
	// The collapse trigger is measured against the output height available
	// under the FULL-size banner, a fixed budget, so evaluating it doesn't
	// move the goalposts once the banner has already collapsed.
	budget := m.height - chrome - inputH - approvalH - m.banner.FullHeight()
	if budget < 0 {
		budget = 0
	}
	m.banner.MaybeCollapse(m.output.TotalContentLines(), budget)

	outputHeight := m.height - chrome - inputH - approvalH - m.banner.Height()
	if outputHeight < 0 {
		outputHeight = 0
	}
	m.output.SetSize(innerW, outputHeight)
	if awaiting {
		m.approval.SetSize(innerW, approvalH)
	}
	m.input.SetSize(innerW, inputH)
}

// View implements tea.Model: the pinned logo banner, a focus-bordered output
// panel, a status bar (processing-state indicator), a context-sensitive hint
// row, an attention-bordered approval panel (only while a decision is
// awaited), and a focus-bordered input panel — input is always rendered, even
// while an approval is pending (inert, since the approval widget captures the
// keys).
func (m Model) View() tea.View {
	rows := make([]string, 0, m.height)
	if bv := m.banner.View(); bv != "" {
		rows = append(rows, strings.Split(bv, "\n")...)
	}
	rows = append(rows, m.frame(m.output.View(), m.focus == focusOutput, false, "")...)
	rows = append(rows, statusBar(m.proc, m.spinner.View(), m.width))
	rows = append(rows, m.hintStrip())
	if m.proc.State == state.StateAwaitingInput {
		rows = append(rows, m.frame(m.approval.View(), true, false, approvalPanelTitle(m.pending))...)
	}
	rows = append(rows, m.frame(m.input.View(), m.focus == focusInput && m.proc.State != state.StateAwaitingInput, m.flashing, "")...)
	rows = clampRowWidth(rows, m.width)
	v := tea.NewView(strings.Join(rows, "\n"))
	v.AltScreen = true
	return v
}

// clampRowWidth is the last-resort safety net against a row wider than the
// terminal: the height budget in relayout is only self-consistent if every
// row is also <= width, and nothing upstream (panel borders, per-widget
// wrapping, the status bar, the hint strip) is otherwise guaranteed to hold
// that invariant across every code path that can produce a row. A row wider
// than width gets soft-wrapped by the terminal itself into an extra physical
// row bubbletea's own height accounting never learns about, pushing every
// row below it — including the input and approval panels — off the visible
// viewport (docs/architecture/behavior/chat_width_overflow_clamp.feature.md).
// A no-op for any row that already fits, so this changes nothing in the
// common case; ansi.StringWidth/ansi.Truncate give the same display-width
// (not byte-length) measurement and ANSI-safe truncation already trusted
// elsewhere in this codebase (internal/surfaces/scrollutil).
func clampRowWidth(rows []string, width int) []string {
	if width <= 0 {
		return rows
	}
	for i, r := range rows {
		if ansi.StringWidth(r) > width {
			rows[i] = ansi.Truncate(r, width, "")
		}
	}
	return rows
}

// frame wraps a panel's rendered content in a box border colored by focus: the
// active panel uses a bold active color, the inactive panel the inactive color. A
// non-empty title switches to the attention color (regardless of active/flash) and
// embeds the title in the top border, drawing attention to an interactive decision
// that needs the user's input.
func (m Model) frame(content string, active, flash bool, title string) []string {
	innerW := m.width - 2
	if innerW < 0 {
		innerW = 0
	}
	code := m.inactive
	if active {
		code = "1;" + m.active
	}
	// A history-boundary flash inverts the border in the active color.
	if flash {
		code = "7;" + m.active
	}
	if title != "" {
		code = m.attention
	}
	paint := func(s string) string {
		if code == "" {
			return s
		}
		return "\x1b[" + code + "m" + s + "\x1b[0m"
	}
	out := []string{paint(titledTopBorder(title, innerW))}
	for _, line := range strings.Split(content, "\n") {
		out = append(out, paint("│")+padCells(line, innerW)+paint("│"))
	}
	out = append(out, paint("└"+strings.Repeat("─", innerW)+"┘"))
	return out
}

// titledTopBorder draws the top border, embedding title as `┌─ title ──...──┐`
// when non-empty (a title that doesn't fit a very narrow panel is dropped).
func titledTopBorder(title string, innerW int) string {
	const lead = 3 // "─ " before the title + " " after it
	if title == "" || innerW-lead <= 0 {
		return "┌" + strings.Repeat("─", innerW) + "┐"
	}
	t := truncateTitle(title, innerW-lead)
	fill := max(innerW-lead-len([]rune(t)), 0)
	return "┌─ " + t + " " + strings.Repeat("─", fill) + "┐"
}

// truncateTitle fits s to w runes, marking truncation with an ellipsis.
func truncateTitle(s string, w int) string {
	r := []rune(s)
	if len(r) <= w || w <= 0 {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// hintStrip renders the single context-sensitive hint row: interrupt guidance
// while working, the ESC-chord menu when a chord is pending, output-navigation
// keys when the output is focused, and the ESC discoverability hint otherwise.
func (m Model) hintStrip() string {
	var text string
	switch {
	case m.proc.State == state.StateAwaitingInput:
		text = "↑/↓ (or j/k) navigate · enter confirm"
	case m.proc.State == state.StateWorking:
		if m.chordPending {
			text = "esc again to confirm interrupt"
		} else {
			text = "esc → interrupt"
		}
	case m.chordPending:
		text = "q quit · ↑ output · ↓ input · esc cancel"
	case m.focus == focusOutput:
		text = "j/k scroll · pgup/pgdn select · ^o expand · esc → input"
	default:
		text = "↑/↓ history · esc → options"
	}
	return padLine(text, m.width)
}

// padCells clips or right-pads s to exactly w display columns, ignoring ANSI
// styling when measuring (used to fit panel content inside the focus border).
func padCells(s string, w int) string {
	width := ansi.StringWidth(s)
	switch {
	case width == w:
		return s
	case width > w:
		return ansi.Truncate(s, w, "")
	default:
		return s + strings.Repeat(" ", w-width)
	}
}

// padLine clips or right-pads s to exactly width display columns. Was
// previously rune-counted, not display-width-measured — the same class of
// bug padCells (above) already avoided. Rune count and display width only
// coincide for plain ASCII; any wide/multi-byte character, or any ANSI
// styling code (which contributes runes but zero display width), made this
// undercount or overcount the real terminal width, and a naive []rune slice
// truncation could cut a multi-byte ANSI escape sequence mid-sequence,
// producing a malformed code the terminal could render as literal garbage
// — genuinely wider on screen than the function ever accounted for. See
// docs/architecture/behavior/chat_width_overflow_clamp.feature.md.
func padLine(s string, width int) string {
	if width <= 0 {
		return s
	}
	w := ansi.StringWidth(s)
	switch {
	case w == width:
		return s
	case w > width:
		return ansi.Truncate(s, width, "")
	default:
		return s + strings.Repeat(" ", width-w)
	}
}

// bannerLabel maps the current processing state to the text the pinned
// banner's collapsed row shows after "AgentX - " (docs/ux/06_OUTPUT_WIDGET.md
// "Logo banner"). Awaiting an interactive decision takes priority over phase
// (the phase the run paused in isn't what the user needs to know right now);
// within StateWorking, phase distinguishes what kind of work is happening.
func bannerLabel(ps state.ProcessingState) string {
	switch ps.State {
	case state.StateWorking:
		switch ps.Phase {
		case state.PhaseThinking, state.PhaseClassify:
			return "Thinking"
		case state.PhaseTool:
			return "Working"
		case state.PhaseRespond:
			return "Responding"
		case state.PhasePlanning:
			return "Planning"
		default:
			return "Working"
		}
	case state.StateAwaitingInput:
		return "Needs Input"
	default:
		return "Your Local Agent"
	}
}

// statusBar renders the processing-state indicator as a single full-width row
// that also separates the output and input panels. While working it shows the
// animated spinner frame in place of the static marker.
func statusBar(ps state.ProcessingState, spin string, width int) string {
	marker := "○"
	switch ps.State {
	case state.StateWorking:
		if marker = strings.TrimSpace(spin); marker == "" {
			marker = "●"
		}
	case state.StateIdle:
		marker = "○"
	default:
		marker = "●"
	}
	label := string(ps.State)
	if ps.Phase != "" && ps.Phase != state.PhaseNone {
		label += " · " + string(ps.Phase)
	}
	text := fmt.Sprintf("%s %s ", marker, label)
	if width <= 0 {
		return text
	}
	// Display-width measured, not rune-counted — see padLine's doc comment for
	// why that distinction matters (a styled spinner frame's ANSI codes are
	// runes but contribute zero display width).
	w := ansi.StringWidth(text)
	if w >= width {
		return ansi.Truncate(text, width, "")
	}
	return text + strings.Repeat("─", width-w)
}
