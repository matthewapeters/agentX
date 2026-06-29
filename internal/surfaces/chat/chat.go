package chat

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"agentx/internal/state"
	"agentx/internal/surfaces/input"
	"agentx/internal/surfaces/output"
)

// inputHeight is the fixed row count reserved for the input panel; hintHeight is
// the single context-sensitive hint/command row beneath the status bar.
const (
	inputHeight = 3
	hintHeight  = 1
)

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
	Submit     func(text string)
	Stop       func()
	Approve    func(decision string)
	Events     <-chan state.Event
	Processing <-chan state.ProcessingState
}

// Model is the chat surface Bubble Tea model. It composes an output panel and an
// input panel separated by a status bar that doubles as the processing-state
// indicator.
type Model struct {
	width        int
	height       int
	output       *output.Model
	input        *input.Model
	proc         state.ProcessingState
	spinner      spinner.Model
	focus        focus
	chordPending bool // an ESC was pressed; the next key resolves the chord
	flashing     bool // the input border is flashing a history boundary
	active       string
	inactive     string
	bridge       *Bridge
}

// New returns an unwired chat surface model (input focused, idle status). Submit
// echoes locally; used by unit tests.
func New() Model {
	out := output.New()
	out.SetTheme(defaultActive, defaultInactive)
	out.SetFocus(false)
	return Model{
		output:   out,
		input:    input.New(),
		proc:     state.ProcessingState{State: state.StateIdle, Phase: state.PhaseNone},
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot)),
		focus:    focusInput,
		active:   defaultActive,
		inactive: defaultInactive,
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
	return tea.Batch(m.listenEvents(), m.listenProcessing())
}

// SetMaxWidgetLines configures the output widgets' body-row cap.
func (m Model) SetMaxWidgetLines(n int) { m.output.SetMaxBody(n) }

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
		cmds := []tea.Cmd{m.listenProcessing()}
		// Start the spinner when work begins; the TickMsg loop sustains it.
		if m.proc.State == state.StateWorking && prev != state.StateWorking {
			cmds = append(cmds, m.spinner.Tick)
		}
		// Cancel a pending chord once work ends.
		if m.proc.State != state.StateWorking {
			m.chordPending = false
		}
		return m, tea.Batch(cmds...)
	case spinner.TickMsg:
		if m.proc.State != state.StateWorking {
			return m, nil // stop ticking once work finishes
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case EventMsg:
		m.output.Apply(state.Event(msg))
		return m, m.listenEvents()
	case flashOffMsg:
		m.flashing = false
		return m, nil
	case tea.MouseWheelMsg:
		// Forwarded to the output viewport; only delivered when the program
		// enables the mouse (off by default to preserve native text selection).
		return m, m.output.Update(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
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

	// Awaiting a tool-approval decision: a/g/d resolve it; other keys are ignored.
	if m.proc.State == state.StateAwaitingInput {
		switch key {
		case "a":
			return m, m.approveCmd("session")
		case "g":
			return m, m.approveCmd("global")
		case "d":
			return m, m.approveCmd("deny")
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
		}
		return m, nil
	}

	// Input focus: edit the prompt or navigate prompt history.
	switch m.input.Update(msg) {
	case input.ActionSubmit:
		text := m.input.Value()
		m.input.Reset()
		if m.bridge != nil && m.bridge.Submit != nil {
			// Route to the runtime; the user prompt and response arrive as
			// EventMsgs and render through the output panel.
			return m, m.submitCmd(text)
		}
		// Unwired: echo locally so the surface is usable in isolation.
		m.output.Apply(state.Event{
			ContentType: state.ContentUserPrompt,
			Payload:     map[string]any{"text": text},
		})
	case input.ActionHistoryBoundary:
		// History navigation hit an edge; flash the input border.
		m.flashing = true
		return m, m.flashCmd()
	}
	return m, nil
}

// flashCmd schedules the end of an input-border flash.
func (m Model) flashCmd() tea.Cmd {
	return tea.Tick(flashDuration, func(time.Time) tea.Msg { return flashOffMsg{} })
}

// approveCmd sends a tool-approval decision ("session"/"global"/"deny") to the
// runtime, resolving the awaiting_input pause.
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
func (m *Model) relayout() {
	innerW := m.width - 2
	if innerW < 0 {
		innerW = 0
	}
	// Chrome: output border (2) + status (1) + hint (1) + input border (2).
	outputHeight := m.height - 2 - 1 - hintHeight - 2 - inputHeight
	if outputHeight < 0 {
		outputHeight = 0
	}
	m.output.SetSize(innerW, outputHeight)
	m.input.SetSize(innerW, inputHeight)
}

// View implements tea.Model: a focus-bordered output panel, a status bar
// (processing-state indicator), a context-sensitive hint row, and a
// focus-bordered input panel.
func (m Model) View() tea.View {
	rows := make([]string, 0, m.height)
	rows = append(rows, m.frame(m.output.View(), m.focus == focusOutput, false)...)
	rows = append(rows, statusBar(m.proc, m.spinner.View(), m.width))
	rows = append(rows, m.hintStrip())
	rows = append(rows, m.frame(m.input.View(), m.focus == focusInput, m.flashing)...)
	v := tea.NewView(strings.Join(rows, "\n"))
	v.AltScreen = true
	return v
}

// frame wraps a panel's rendered content in a box border colored by focus: the
// active panel uses a bold active color, the inactive panel the inactive color.
func (m Model) frame(content string, active, flash bool) []string {
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
	paint := func(s string) string {
		if code == "" {
			return s
		}
		return "\x1b[" + code + "m" + s + "\x1b[0m"
	}
	bar := strings.Repeat("─", innerW)
	out := []string{paint("┌" + bar + "┐")}
	for _, line := range strings.Split(content, "\n") {
		out = append(out, paint("│")+padCells(line, innerW)+paint("│"))
	}
	out = append(out, paint("└"+bar+"┘"))
	return out
}

// hintStrip renders the single context-sensitive hint row: interrupt guidance
// while working, the ESC-chord menu when a chord is pending, output-navigation
// keys when the output is focused, and the ESC discoverability hint otherwise.
func (m Model) hintStrip() string {
	var text string
	switch {
	case m.proc.State == state.StateAwaitingInput:
		text = "approve: a session · g global · d deny"
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

// padLine clips or right-pads s to exactly width display columns.
func padLine(s string, width int) string {
	if width <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) >= width {
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len(r))
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
	r := []rune(text)
	if len(r) >= width {
		return string(r[:width])
	}
	return text + strings.Repeat("─", width-len(r))
}
