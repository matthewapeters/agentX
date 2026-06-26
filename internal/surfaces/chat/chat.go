package chat

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"agentx/internal/state"
	"agentx/internal/surfaces/input"
	"agentx/internal/surfaces/output"
)

// inputHeight is the fixed row count reserved for the input panel.
const inputHeight = 3

// ProcessingStateMsg delivers a processing-state update to the chat surface. The
// program wiring (CHT-B5) bridges the orchestrator's processing-state feed into
// these messages.
type ProcessingStateMsg state.ProcessingState

// Model is the chat surface Bubble Tea model. It composes an output panel and an
// input panel separated by a status bar that doubles as the processing-state
// indicator.
type Model struct {
	width  int
	height int
	output *output.Model
	input  *input.Model
	proc   state.ProcessingState
}

// New returns a chat surface model with input focused and an idle status.
func New() Model {
	return Model{
		output: output.New(),
		input:  input.New(),
		proc:   state.ProcessingState{State: state.StateIdle, Phase: state.PhaseNone},
	}
}

// Output returns the output panel.
func (m Model) Output() *output.Model { return m.output }

// Input returns the input panel.
func (m Model) Input() *input.Model { return m.input }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model: it handles resize and the quit key, and routes
// other keys to the focused panel (panel key handling lands in CHT-B3).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()
	case ProcessingStateMsg:
		m.proc = state.ProcessingState(msg)
		m.input.SetStreaming(m.proc.State == state.StateWorking)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.input.Update(msg) {
		case input.ActionSubmit:
			// Local echo for now; CHT-B5 routes the prompt to the orchestrator
			// and streams the real response back.
			m.output.Apply(state.Event{
				ContentType: state.ContentUserPrompt,
				Payload:     map[string]any{"text": m.input.Value()},
			})
			m.input.Reset()
		case input.ActionStop:
			m.input.SetStreaming(false)
		}
	}
	return m, nil
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
		statusBar(m.proc, m.width),
		m.input.View(),
	}, "\n")
	return tea.NewView(content)
}

// statusBar renders the processing-state indicator as a single full-width row
// that also separates the output and input panels.
func statusBar(ps state.ProcessingState, width int) string {
	marker := "○"
	if ps.State != state.StateIdle {
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
