// Package contextviz is the read-only context-visualizer surface (SS-7): it polls
// the orchestrator for the assembled context window's composition by content class
// and renders it as a budget meter — one bar per class (working memory,
// instructions, user, attachments, thinking, assistant, tools) plus a remaining-
// capacity ghost band, sized against the model's context window. It performs no
// writes; the enable/disable management affordance lives on the context pane.
// Re-authors legacy PD-10 (ContextMeterWidget) for the TUI.
package contextviz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"agentx/internal/session"
	"agentx/internal/surfaces"
	"agentx/internal/surfaces/client"
	transporthttp "agentx/internal/transport/http"
)

// pollInterval is how often the surface refetches the breakdown so agent-side
// turns and working-memory edits appear.
const pollInterval = 2 * time.Second

// barWidth is the column width of each class's meter bar.
const barWidth = 20

// band is one content class in the meter, in PD-10 band order, with the app's
// existing content emoji so it reads consistently with the output widgets.
type band struct {
	class string
	emoji string
	label string
}

// bands is the fixed, ordered class set the meter always renders (classes with no
// contribution show as zero — the legend stays complete and future-proof).
var bands = []band{
	{"working-memory", "🧠", "working memory"},
	{"instructions", "📜", "instructions"},
	{"user", "👤", "user"},
	{"attachments", "📎", "attachments"},
	{"thinking", "💭", "thinking"},
	{"assistant", "🤖", "assistant"},
	{"tools", "🔧", "tools"},
}

// Options configures the context-visualizer surface program.
type Options struct {
	Endpoint    string
	Token       string
	SurfaceID   string
	SessionName string
	// SessionRoot and SessionID enable re-attachment after a mid-session
	// resume (docs/architecture/behavior/session_resume.feature.md §5): like
	// the working-memory surface, this document-based surface holds its own
	// transport client/token captured once at launch, so a resume that swaps
	// the session out from under it otherwise leaves it stuck polling with a
	// now-invalid token forever. SessionRoot == "" (an older/headless caller)
	// disables re-attachment — an auth failure then just reports as before.
	SessionRoot string
	SessionID   string
}

// Run launches the context visualizer against a running orchestrator and blocks
// until the user quits.
func Run(ctx context.Context, opts Options) error {
	// Hold an event stream purely for connection presence (SS-4): the visualizer
	// polls (it is document-based), so it consumes no events, but the open stream
	// makes the launch widget show it 🟢. The stream closes on exit.
	defer client.Presence(ctx, opts.Endpoint, opts.SurfaceID)()

	m := New(transporthttp.NewClient(opts.Endpoint), opts)
	_, err := tea.NewProgram(m, tea.WithContext(ctx)).Run()
	return err
}

// ReportMsg delivers a fetched breakdown to the surface. It is exported so the
// surface can be driven without a live server (tests, future hosts).
type ReportMsg session.ContextReport

type (
	errMsg  struct{ err error }
	tickMsg struct{}
)

// reattachedMsg carries the result of re-resolving this session's transport
// endpoint and attach token from disk after an auth failure (see
// Options.SessionRoot).
type reattachedMsg struct {
	endpoint string
	token    string
	err      error
}

// isStaleAttachToken reports whether err is the server rejecting this
// surface's attach token — the specific, recoverable case (a resume swapped
// the underlying session out from under this surface) as opposed to a
// transport/validation failure, which re-resolving from disk cannot fix.
func isStaleAttachToken(err error) bool {
	var ae *transporthttp.AttachError
	return errors.As(err, &ae) && ae.Category == surfaces.CategoryAuth
}

// Model is the read-only context-visualizer.
type Model struct {
	cl   *transporthttp.Client
	opts Options

	report session.ContextReport
	width  int
	height int
	status string
}

// New returns a context visualizer bound to a transport client.
func New(cl *transporthttp.Client, opts Options) Model {
	return Model{cl: cl, opts: opts, width: 80, height: 24}
}

// Init fetches the initial breakdown and starts the refresh poll.
func (m Model) Init() tea.Cmd { return tea.Batch(m.fetch(), tickCmd()) }

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// reattach re-resolves this session's transport endpoint and attach token
// from disk (SS-5) — the same discovery a fresh `agentx surface launch` uses
// — after the server has rejected this surface's token, which happens when a
// mid-session resume (ESC,r) swaps the running session out from under it
// (docs/architecture/behavior/session_resume.feature.md §5). A no-op
// (returns nil) when SessionRoot is unset, matching the working-memory
// surface's own SessionRoot == "" convention for callers that predate
// reconnection.
func (m Model) reattach() tea.Cmd {
	if m.opts.SessionRoot == "" || m.opts.SessionID == "" {
		return nil
	}
	root, id := m.opts.SessionRoot, m.opts.SessionID
	return func() tea.Msg {
		store := session.NewStore(root)
		info, err := store.ReadTransport(id)
		if err != nil {
			return reattachedMsg{err: err}
		}
		token, err := store.ReadAttachToken(id)
		if err != nil {
			return reattachedMsg{err: err}
		}
		return reattachedMsg{endpoint: info.Endpoint, token: token}
	}
}

func (m Model) fetch() tea.Cmd {
	cl := m.cl
	return func() tea.Msg {
		report, err := cl.ContextBreakdown(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return ReportMsg(report)
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case ReportMsg:
		m.report = session.ContextReport(msg)
		m.status = ""
		return m, nil
	case errMsg:
		if isStaleAttachToken(msg.err) && m.opts.SessionRoot != "" {
			m.status = "reattaching…"
			return m, m.reattach()
		}
		m.status = "error: " + msg.err.Error()
		return m, nil
	case reattachedMsg:
		if msg.err != nil {
			m.status = "error: reattach failed: " + msg.err.Error()
			return m, nil
		}
		m.opts.Endpoint, m.opts.Token = msg.endpoint, msg.token
		m.cl = transporthttp.NewClient(msg.endpoint)
		m.status = ""
		return m, m.fetch()
	case tickMsg:
		return m, tea.Batch(m.fetch(), tickCmd())
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			_ = m.cl.Shutdown(context.Background(), m.opts.SurfaceID, m.opts.Token)
			return m, tea.Quit
		case "r":
			return m, m.fetch()
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() tea.View {
	var b strings.Builder

	title := "context"
	if m.opts.SessionName != "" {
		title += " · " + m.opts.SessionName
	}
	fmt.Fprintf(&b, "%s\n", clip(title, m.width))
	b.WriteString(rule(m.width))

	sizes := m.classTokens()
	window := m.report.WindowTokens
	total := m.report.TotalTokens()

	// The bar denominator is the window when known (honest budget), else the
	// largest class so the bars still convey relative proportion.
	denom := window
	if denom <= 0 {
		denom = maxTokens(sizes)
	}

	for _, bd := range bands {
		tok := sizes[bd.class]
		fmt.Fprintf(&b, " %s %-14s %s %5d  %s\n",
			bd.emoji, bd.label, bar(tok, denom, barWidth), tok, pct(tok, window))
	}
	// Remaining-capacity ghost band (only meaningful when the window is known).
	if window > 0 {
		remaining := window - total
		if remaining < 0 {
			remaining = 0
		}
		fmt.Fprintf(&b, " %s %-14s %s %5d  %s\n",
			"░░", "remaining", ghostBar(remaining, denom, barWidth), remaining, pct(remaining, window))
	}

	b.WriteString(rule(m.width))
	b.WriteString(" " + m.totalLine(total, window) + "\n")
	b.WriteString(" est. = chars ÷ 4 · read-only · manage in the context pane\n")
	b.WriteString(" r refresh · q quit\n")
	if m.status != "" {
		fmt.Fprintf(&b, " %s\n", clip(m.status, m.width))
	}

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// classTokens indexes the report's per-class token estimates by class id.
func (m Model) classTokens() map[string]int {
	out := make(map[string]int, len(m.report.Components))
	for _, c := range m.report.Components {
		out[c.Class] = c.EstTokens()
	}
	return out
}

// totalLine summarizes usage: "3344 / 8192 est. tokens (41%) · qwen2.5" — or a
// window-unknown variant when the model reports no context length.
func (m Model) totalLine(total, window int) string {
	model := m.report.Model
	if window <= 0 {
		return fmt.Sprintf("%d est. tokens · %s (window unknown)", total, model)
	}
	line := fmt.Sprintf("%d / %d est. tokens (%s) · %s", total, window, pct(total, window), model)
	switch {
	case total >= window:
		line += "  ⛔ full"
	case total*100 >= window*80:
		line += "  ⚠ near limit"
	}
	return line
}

// bar renders a proportional filled bar of the given width.
func bar(value, denom, width int) string {
	return strings.Repeat("█", filled(value, denom, width)) +
		strings.Repeat(" ", width-filled(value, denom, width))
}

// ghostBar renders the remaining-capacity band with a distinct glyph.
func ghostBar(value, denom, width int) string {
	f := filled(value, denom, width)
	return strings.Repeat("░", f) + strings.Repeat(" ", width-f)
}

// filled maps value/denom onto [0,width]. A non-zero value always shows at least
// one cell so a present-but-tiny class is visible.
func filled(value, denom, width int) int {
	if denom <= 0 || value <= 0 {
		return 0
	}
	n := value * width / denom
	if n == 0 {
		n = 1
	}
	if n > width {
		n = width
	}
	return n
}

// pct formats value as a right-aligned integer percent of window, or a dash when
// the window is unknown.
func pct(value, window int) string {
	if window <= 0 {
		return "   —"
	}
	return fmt.Sprintf("%3d%%", value*100/window)
}

// maxTokens returns the largest per-class token count (bar denominator fallback).
func maxTokens(sizes map[string]int) int {
	m := 0
	for _, v := range sizes {
		if v > m {
			m = v
		}
	}
	return m
}

// rule draws a horizontal separator clipped to the width.
func rule(width int) string {
	width = max(width, 1)
	width = min(width, 60)
	return strings.Repeat("─", width) + "\n"
}

// clip truncates s to w columns (best-effort, byte-based for ASCII labels).
func clip(s string, w int) string {
	if w <= 0 || len(s) <= w {
		return s
	}
	return s[:w]
}
