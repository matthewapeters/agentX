package chat

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"agentx/internal/state"
	"agentx/internal/surfaces/input"
	"agentx/internal/surfaces/output"
)

// inputHeight is the fixed row count reserved for the input panel.
const inputHeight = 3

// Model is the chat surface Bubble Tea model. It composes an output panel and an
// input panel and arranges them vertically with a separator.
type Model struct {
	width  int
	height int
	output *output.Model
	input  *input.Model
}

// New returns a chat surface model with input focused.
func New() Model {
	return Model{
		output: output.New(),
		input:  input.New(),
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

// View implements tea.Model: output panel, separator, input panel.
func (m Model) View() tea.View {
	separator := strings.Repeat("─", max(m.width, 0))
	content := strings.Join([]string{
		m.output.View(),
		separator,
		m.input.View(),
	}, "\n")
	return tea.NewView(content)
}
