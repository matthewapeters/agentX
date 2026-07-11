// Package workmemory is the working-memory editor surface (SS-6): a read-write peer
// surface that lists the session's working-memory facts and lets the user add, edit,
// delete, and enable/disable them. Unlike the context viewer it is not event-driven —
// working memory is a document, so it reads on attach, polls for live refresh, and
// mutates through the dedicated transport endpoints.
package workmemory

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"agentx/internal/session"
	"agentx/internal/surfaces/client"
	transporthttp "agentx/internal/transport/http"
)

// pollInterval is how often the surface refetches working memory so agent-side or
// other-surface changes appear.
const pollInterval = 2 * time.Second

// Options configures the working-memory surface program.
type Options struct {
	Endpoint    string
	Token       string
	SurfaceID   string
	SessionName string
}

// Run launches the working-memory editor against a running orchestrator and blocks
// until the user quits.
func Run(ctx context.Context, opts Options) error {
	// Hold an event stream purely for connection presence (SS-4): the orchestrator
	// reports a surface connected while it has an open stream. WM is document-based
	// (it polls), so it consumes no events — but it still holds a stream so the
	// launch widget shows it as 🟢. The stream closes on exit.
	defer client.Presence(ctx, opts.Endpoint, opts.SurfaceID)()

	m := New(transporthttp.NewClient(opts.Endpoint), opts)
	_, err := tea.NewProgram(m, tea.WithContext(ctx)).Run()
	return err
}

type editMode int

const (
	modeList editMode = iota
	modeAdd           // entering "key=value"
	modeEdit          // entering a new value for the selected fact
)

// FactsMsg delivers a fetched working-memory snapshot to the surface. It is exported
// so the surface can be driven without a live server (tests, future hosts).
type FactsMsg []session.Fact

type (
	errMsg  string
	doneMsg struct{} // a mutation completed — refetch
	tickMsg struct{}
)

// Model is the working-memory editor.
type Model struct {
	cl   *transporthttp.Client
	opts Options

	facts    []session.Fact
	selected int
	width    int
	height   int

	mode   editMode
	buf    []rune // inline editor buffer
	status string
}

// New returns a working-memory editor bound to a transport client.
func New(cl *transporthttp.Client, opts Options) Model {
	return Model{cl: cl, opts: opts, width: 80, height: 24}
}

// Init fetches the initial facts and starts the refresh poll.
func (m Model) Init() tea.Cmd { return tea.Batch(m.fetch(), tickCmd()) }

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) fetch() tea.Cmd {
	cl := m.cl
	return func() tea.Msg {
		facts, err := cl.WorkingMemory(context.Background())
		if err != nil {
			return errMsg(err.Error())
		}
		return FactsMsg(facts)
	}
}

func (m Model) setFact(key, value string) tea.Cmd {
	cl, token := m.cl, m.opts.Token
	return func() tea.Msg {
		if err := cl.SetFact(context.Background(), token, key, value); err != nil {
			return errMsg(err.Error())
		}
		return doneMsg{}
	}
}

func (m Model) toggle(key string, enabled bool) tea.Cmd {
	cl, token := m.cl, m.opts.Token
	return func() tea.Msg {
		if err := cl.SetFactEnabled(context.Background(), token, key, enabled); err != nil {
			return errMsg(err.Error())
		}
		return doneMsg{}
	}
}

func (m Model) deleteFact(key string) tea.Cmd {
	cl, token := m.cl, m.opts.Token
	return func() tea.Msg {
		if err := cl.DeleteFact(context.Background(), token, key); err != nil {
			return errMsg(err.Error())
		}
		return doneMsg{}
	}
}

// toggleLive flips a pinned fact between live (▶, re-run before every turn) and
// static (⏸, frozen) — PD-WM-AF-008. The server refuses it on a fact with no
// tool Source (a plain user/agent fact).
func (m Model) toggleLive(key string, live bool) tea.Cmd {
	cl, token := m.cl, m.opts.Token
	return func() tea.Msg {
		if err := cl.SetFactLive(context.Background(), token, key, live); err != nil {
			return errMsg(err.Error())
		}
		return doneMsg{}
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case FactsMsg:
		m.facts = []session.Fact(msg)
		if m.selected >= len(m.facts) {
			m.selected = max(0, len(m.facts)-1)
		}
		return m, nil
	case errMsg:
		m.status = "error: " + string(msg)
		return m, nil
	case doneMsg:
		return m, m.fetch()
	case tickMsg:
		return m, tea.Batch(m.fetch(), tickCmd())
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

// cur returns the selected fact, if any.
func (m Model) cur() (session.Fact, bool) {
	if m.selected < 0 || m.selected >= len(m.facts) {
		return session.Fact{}, false
	}
	return m.facts[m.selected], true
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeList {
		return m.editKey(msg)
	}
	switch msg.String() {
	case "ctrl+c", "q":
		_ = m.cl.Shutdown(context.Background(), m.opts.SurfaceID, m.opts.Token)
		return m, tea.Quit
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.facts)-1 {
			m.selected++
		}
	case " ", "space":
		if f, ok := m.cur(); ok {
			return m, m.toggle(f.Key, !f.Enabled)
		}
	case "d", "x":
		if f, ok := m.cur(); ok {
			return m, m.deleteFact(f.Key)
		}
	case "l":
		if f, ok := m.cur(); ok && f.Source != nil {
			return m, m.toggleLive(f.Key, !f.Live)
		}
	case "a", "n":
		m.mode = modeAdd
		m.buf = nil
		m.status = "add — type key=value, enter to save, esc to cancel"
	case "e":
		if f, ok := m.cur(); ok {
			m.mode = modeEdit
			m.buf = []rune(f.Value)
			m.status = "edit " + f.Key + " — enter to save, esc to cancel"
		}
	case "r":
		return m, m.fetch()
	}
	return m, nil
}

// editKey handles the inline single-line editor for add/edit.
func (m Model) editKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "escape":
		m.mode, m.buf, m.status = modeList, nil, ""
		return m, nil
	case "enter":
		mode, buf := m.mode, strings.TrimSpace(string(m.buf))
		m.mode, m.buf, m.status = modeList, nil, ""
		switch mode {
		case modeAdd:
			key, value, ok := strings.Cut(buf, "=")
			key = strings.TrimSpace(key)
			if !ok || key == "" {
				m.status = "expected key=value"
				return m, nil
			}
			return m, m.setFact(key, strings.TrimSpace(value))
		case modeEdit:
			if f, ok := m.cur(); ok {
				return m, m.setFact(f.Key, buf)
			}
		}
		return m, nil
	case "backspace":
		if n := len(m.buf); n > 0 {
			m.buf = m.buf[:n-1]
		}
		return m, nil
	}
	if msg.Text != "" {
		m.buf = append(m.buf, []rune(msg.Text)...)
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() tea.View {
	var b strings.Builder
	title := "working memory"
	if m.opts.SessionName != "" {
		title += " · " + m.opts.SessionName
	}
	fmt.Fprintf(&b, "%s\n\n", clip(title, m.width))

	if len(m.facts) == 0 {
		b.WriteString("  (no facts — press 'a' to add)\n")
	}
	for i, f := range m.facts {
		cursor := "  "
		if i == m.selected {
			cursor = "› "
		}
		dot := "○"
		if f.Enabled {
			dot = "●"
		}
		owner := ""
		switch f.Owner {
		case session.OwnerAgent:
			owner = " (agent)"
		case session.OwnerPin:
			state, mark := "static", "⏸"
			if f.Live {
				state, mark = "live", "▶"
			}
			owner = fmt.Sprintf(" (pin %s %s, %s)", mark, state, f.Age().Round(time.Second))
		}
		fmt.Fprintf(&b, "%s%s %s = %s%s\n", cursor, dot, f.Key, f.Value, owner)
	}

	b.WriteString("\n")
	if m.mode != modeList {
		fmt.Fprintf(&b, "> %s█\n", string(m.buf))
	} else {
		b.WriteString("↑/↓ move · space toggle · l live/static (pinned) · a add · e edit · d delete · r refresh · q quit\n")
	}
	if m.status != "" {
		fmt.Fprintf(&b, "%s\n", clip(m.status, m.width))
	}

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// clip truncates s to w columns (best-effort, byte-based for ASCII labels).
func clip(s string, w int) string {
	if w <= 0 || len(s) <= w {
		return s
	}
	return s[:w]
}
