// Package context is the context surface (SS-3): a SurfaceModel that projects the
// session's complete conversation elements into the shared collapsible output
// renderer as a navigable summary (every element collapsed by default). It is the
// context-management surface — selecting a user or agent element and pressing space
// toggles whether it participates in the agent's upcoming context (persisted by the
// orchestrator). It sees only complete events (never the live agent_delta stream,
// which is the chat window's job). Launched with `agentx surface launch context`.
package context

import (
	stdctx "context"

	tea "charm.land/bubbletea/v2"

	"agentx/internal/state"
	"agentx/internal/surfaces/output"
	transporthttp "agentx/internal/transport/http"
)

// Model renders the session's conversation elements as a collapsed, navigable
// summary and drives enable/disable toggles back to the orchestrator. It reuses
// output.Model for the boxed widgets/scroll and intercepts processing-state events
// for a status line.
type Model struct {
	out   *output.Model
	proc  state.ProcessingState
	cl    *transporthttp.Client // transport for toggle POSTs (nil in unit tests)
	token string
}

// New returns a context surface bound to a transport client for toggle POSTs. Its
// output panel is always focused — it is the sole panel in the process — and every
// element renders collapsed (a navigable summary).
func New(cl *transporthttp.Client, token string) *Model {
	out := output.New()
	out.SetFocus(true)
	out.SetCollapseByDefault(true)
	out.SetShowToggleState(true)
	return &Model{out: out, cl: cl, token: token}
}

// Apply folds one event into the projection. Processing-state events update the
// status line; everything else renders as an output widget.
func (m *Model) Apply(ev state.Event) {
	// Ephemeral events (the startup bootstrap exchange) engage the session but are
	// not part of the user's conversation, so the read-only viewer omits them.
	if ev.Ephemeral {
		return
	}
	if ev.ContentType == state.ContentProcessingState {
		m.proc = decodeProcessing(ev.Payload)
		return
	}
	// Discards the returned tea.Cmd deliberately: this surface has no message loop
	// of its own driving a plan node's spinner tick (client.SurfaceModel.Apply has no
	// return value), so a plan here renders correct static glyphs/coloring but never
	// animates — an accepted scope boundary, not an oversight (ADR 0009 §9c redesign
	// plan). It's a collapsed navigable summary here, not the live transcript.
	m.out.Apply(ev)
}

// SetSize sets the render area, reserving one row for the status line.
func (m *Model) SetSize(width, height int) {
	m.out.SetSize(width, max(0, height-1))
}

// Key handles navigation, the enable/disable toggle, and Pin, mirroring the chat
// output panel so the keys are consistent: PgUp/PgDn move the selection between
// elements, Up/Down (k/j) scroll within the selected element, Enter
// expands/collapses it, space toggles whether a user/agent/tool element
// participates in the agent's context (session-scoped), p pins a selected
// tool_result — or, inside a selected plan widget, the active node's own
// result/value (durable — PD-CTX-AF-012 / PD-WM, ADR 0012 amendment) — into
// working memory, and Tab/Shift+Tab move the node-level pin cursor within a
// selected plan widget (a no-op on any other selection).
func (m *Model) Key(msg tea.KeyPressMsg) tea.Cmd {
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
	case " ", "space":
		return m.toggleSelected()
	case "p":
		return m.pinSelected()
	case "tab":
		m.out.ActiveNodeNext()
	case "shift+tab":
		m.out.ActiveNodePrev()
	}
	return nil
}

// toggleSelected flips the selected element's context participation: it updates the
// surface optimistically and returns a command that persists the change through the
// orchestrator. It is a no-op for non-toggleable elements (thinking/tool/etc.).
func (m *Model) toggleSelected() tea.Cmd {
	ordinal, enabled, ok := m.out.SelectedToggleable()
	if !ok {
		return nil
	}
	newEnabled := !enabled
	m.out.SetSelectedEnabled(newEnabled)
	if m.cl == nil {
		return nil
	}
	cl, token := m.cl, m.token
	return func() tea.Msg {
		_ = cl.SetEventEnabled(stdctx.Background(), token, ordinal, newEnabled)
		return nil
	}
}

// pinSelected copies the selected tool_result element — or, when a plan widget
// is selected, its active node's own result/value (ADR 0012 amendment) — into
// working memory as a static pin (PD-CTX-AF-012) and disables the source event in
// context server-side where one exists, so the same output is never represented
// twice. Live vs static is decided afterward in the working-memory surface (play/
// pause, PD-WM-AF-008; a value-sourced pin has no Source to ever go live at all —
// see PinPlanNode's doc comment), not here — Pin always starts static. It is a
// no-op for a selection with nothing pinnable.
func (m *Model) pinSelected() tea.Cmd {
	if pin, ok := m.out.SelectedPlanNode(); ok {
		return m.pinPlanNode(pin)
	}
	ordinal, ok := m.out.SelectedToolEvent()
	if !ok || m.cl == nil {
		return nil
	}
	cl, token := m.cl, m.token
	return func() tea.Msg {
		_, _ = cl.PinToolEvent(stdctx.Background(), token, ordinal, false)
		return nil
	}
}

// pinPlanNode dispatches a plan node's own pin: a Task's arrived tool_result
// reuses the existing ordinal-keyed path (PinToolEvent, same as a flat
// tool_result); a Step's resolved Value with no backing tool call goes through
// PinPlanNode instead.
func (m *Model) pinPlanNode(pin output.PlanNodePin) tea.Cmd {
	if m.cl == nil {
		return nil
	}
	cl, token := m.cl, m.token
	switch {
	case pin.HasOrdinal:
		ordinal := pin.Ordinal
		return func() tea.Msg {
			_, _ = cl.PinToolEvent(stdctx.Background(), token, ordinal, false)
			return nil
		}
	case pin.HasValue:
		root, nodeID := pin.RootID, pin.NodeID
		return func() tea.Msg {
			_, _ = cl.PinPlanNode(stdctx.Background(), token, root, nodeID)
			return nil
		}
	}
	return nil
}

// CapturesKeys reports whether the surface is capturing free-form text input.
// The context surface has no text-entry mode, so this is always false (SS-8).
func (m *Model) CapturesKeys() bool { return false }

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
