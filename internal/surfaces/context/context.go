// Package context is the read-only context viewer surface (SS-3): a SurfaceModel
// that projects the session event stream into the shared collapsible output
// renderer and shows a processing-state line. It is launched as a separate process
// (`agentx surface launch context`) and attaches over the transport, mirroring the
// conversation the chat surface shows.
package context

import (
	tea "charm.land/bubbletea/v2"

	"agentx/internal/state"
	"agentx/internal/surfaces/output"
)

// Model renders the session event stream read-only. It reuses output.Model for the
// collapsible boxed widgets and viewport scroll, and intercepts processing-state
// events for a status line (output ignores them).
type Model struct {
	out  *output.Model
	proc state.ProcessingState
}

// New returns an empty context viewer. Its output panel is always focused — it is
// the sole panel in the surface process — so the selected object is highlighted as
// the user navigates.
func New() *Model {
	out := output.New()
	out.SetFocus(true)
	return &Model{out: out}
}

// Apply folds one event into the projection. Processing-state events update the
// status line; everything else renders as an output widget.
func (m *Model) Apply(ev state.Event) {
	if ev.ContentType == state.ContentProcessingState {
		m.proc = decodeProcessing(ev.Payload)
		return
	}
	m.out.Apply(ev)
}

// SetSize sets the render area, reserving one row for the status line.
func (m *Model) SetSize(width, height int) {
	m.out.SetSize(width, max(0, height-1))
}

// Key handles read-only navigation: scroll and collapse/expand. There is no prompt
// input — this surface only observes.
// Key handles read-only navigation, mirroring the chat output panel so the keys are
// consistent: PgUp/PgDn move the selection between objects, Up/Down (k/j) scroll
// within the selected object, and Enter expands/collapses it.
func (m *Model) Key(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "pgup":
		m.out.SelectUp()
	case "pgdown":
		m.out.SelectDown()
	case "up", "k":
		m.out.ScrollSelected(-1)
	case "down", "j":
		m.out.ScrollSelected(1)
	case "enter", "ctrl+o":
		m.out.ToggleSelected()
	}
}

// View renders the output body above a processing-state line.
func (m *Model) View() string {
	return m.out.View() + "\n" + m.statusLine()
}

// statusLine summarizes the live processing state.
func (m *Model) statusLine() string {
	st := string(m.proc.State)
	if st == "" || st == string(state.StateIdle) {
		return "● idle"
	}
	line := "● " + st
	if m.proc.Phase != "" && m.proc.Phase != state.PhaseNone {
		line += " · " + string(m.proc.Phase)
	}
	return line
}

// decodeProcessing reads a processing-state payload, accepting both the in-process
// struct and the JSON-decoded map a surface receives over the transport.
func decodeProcessing(payload any) state.ProcessingState {
	switch p := payload.(type) {
	case state.ProcessingState:
		return p
	case map[string]any:
		st, _ := p["state"].(string)
		ph, _ := p["phase"].(string)
		return state.ProcessingState{State: state.RunState(st), Phase: state.Phase(ph)}
	}
	return state.ProcessingState{}
}
