package chat

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"agentx/internal/state"
	"agentx/internal/surfaces/input"
	"agentx/internal/surfaces/output"
)

// inputHeight is the fixed row count reserved for the input panel.
const inputHeight = 3

// ProcessingStateMsg delivers a processing-state update to the chat surface.
type ProcessingStateMsg state.ProcessingState

// EventMsg delivers a bus event to the chat surface for rendering.
type EventMsg state.Event

// Bridge connects the chat surface to the runtime: a Submit function for prompts,
// a Stop function that interrupts the in-flight prompt, and the channels the
// surface listens on for events and processing-state. A zero Bridge (nil fields)
// leaves the surface in local-echo mode for unit tests.
type Bridge struct {
	Submit     func(text string)
	Stop       func()
	Events     <-chan state.Event
	Processing <-chan state.ProcessingState
}

// Model is the chat surface Bubble Tea model. It composes an output panel and an
// input panel separated by a status bar that doubles as the processing-state
// indicator.
type Model struct {
	width   int
	height  int
	output  *output.Model
	input   *input.Model
	proc    state.ProcessingState
	spinner spinner.Model
	bridge  *Bridge
}

// New returns an unwired chat surface model (input focused, idle status). Submit
// echoes locally; used by unit tests.
func New() Model {
	return Model{
		output:  output.New(),
		input:   input.New(),
		proc:    state.ProcessingState{State: state.StateIdle, Phase: state.PhaseNone},
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
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
	case tea.MouseWheelMsg:
		// Forwarded to the output viewport; only delivered when the program
		// enables the mouse (off by default to preserve native text selection).
		return m, m.output.Update(msg)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// Scrollback keys drive the output viewport rather than the input,
		// which the textarea ignores anyway.
		switch msg.String() {
		case "pgup":
			m.output.PageUp()
			return m, nil
		case "pgdown":
			m.output.PageDown()
			return m, nil
		case "ctrl+u":
			m.output.ScrollUp(1)
			return m, nil
		case "ctrl+d":
			m.output.ScrollDown(1)
			return m, nil
		}
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
		case input.ActionStop:
			m.input.SetStreaming(false)
		}
	}
	return m, nil
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

// relayout sizes the panels to the current terminal: the input panel takes a
// fixed height at the bottom, a one-row separator sits above it, and the output
// panel fills the rest.
func (m *Model) relayout() {
	outputHeight := m.height - inputHeight - 1
	if outputHeight < 0 {
		outputHeight = 0
	}
	m.output.SetSize(m.width, outputHeight)
	m.input.SetSize(m.width, inputHeight)
}

// View implements tea.Model: output panel, status bar (processing-state
// indicator), input panel.
func (m Model) View() tea.View {
	content := strings.Join([]string{
		m.output.View(),
		statusBar(m.proc, m.spinner.View(), m.width),
		m.input.View(),
	}, "\n")
	v := tea.NewView(content)
	v.AltScreen = true
	return v
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
