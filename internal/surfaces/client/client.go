// Package client is the shared Bubble Tea host for external rendering surfaces
// (SS-2). It attaches to a running orchestrator over the transport, seeds from the
// durable log, resumes the live event stream by cursor, and drives a SurfaceModel
// — the per-surface contract that concrete surfaces (context, files, config, …)
// implement. The host owns the attach lifecycle (seed → live → resize → quit); each
// surface owns only its projection and rendering.
package client

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"agentx/internal/session"
	"agentx/internal/state"
	transporthttp "agentx/internal/transport/http"
)

// errNoEndpoint reports that a resolved session has no recorded transport
// endpoint yet (a newly-started process may not have finished writing its
// transport.json) — a retryable condition, not a permanent failure.
var errNoEndpoint = errors.New("session has no recorded transport endpoint")

// Presence opens an event stream tagged with surfaceID purely so the orchestrator
// marks the surface connected (SS-4), draining and discarding its events. It is for
// document-based surfaces (e.g. the working-memory editor) that don't consume the
// event stream but still need to report liveness. The returned cancel closes the
// stream — call it on program exit so the surface is marked disconnected. A failed
// subscribe is non-fatal: the surface simply shows as not-connected.
func Presence(ctx context.Context, endpoint, surfaceID string) context.CancelFunc {
	streamCtx, cancel := context.WithCancel(ctx)
	if ch, err := transporthttp.NewClient(endpoint).Subscribe(streamCtx, 0, surfaceID); err == nil {
		go func() {
			for range ch {
			}
		}()
	}
	return cancel
}

// SurfaceModel is the per-surface contract the host drives.
type SurfaceModel interface {
	// Apply folds one session event into the surface's projection.
	Apply(ev state.Event)
	// SetSize sets the inner render area (inside the host's title strip).
	SetSize(width, height int)
	// Key handles a surface-specific key (scroll, toggle, etc.) and may return a
	// command (e.g. a transport POST). Global keys (quit) are handled by the host
	// before this is called.
	Key(msg tea.KeyPressMsg) tea.Cmd
	// View renders the surface body to its current size.
	View() string
	// CapturesKeys reports whether the surface is capturing free-form text input
	// right now (e.g. a search-pattern prompt). While true, the host stops
	// treating "q" as a quit key and forwards it to Key like any other
	// character, so a captured pattern may contain a literal "q" (SS-8).
	// Ctrl-C always quits regardless of this — a hard-abort that stays
	// reachable even mid-capture, so a governed surface is never
	// unrecoverable.
	CapturesKeys() bool
	// Reset clears the surface's local projection entirely — every widget,
	// scroll position, and cached state — so a subsequent Apply sequence
	// starts from nothing. Called when Host reconnects to a different
	// session (docs/architecture/behavior/session_resume.feature.md §5): an
	// old session's content must not linger alongside the new one's.
	Reset()
}

// EventMsg carries one live session event to the host.
type EventMsg state.Event

// seedMsg carries the durable seed snapshot, applied before the live stream.
type seedMsg []state.Event

// streamClosedMsg signals the event stream ended.
type streamClosedMsg struct{}

// reconnectTickMsg drives the reconnect poll loop.
type reconnectTickMsg struct{}

// reconnectedMsg carries a freshly (re)established seed+live stream, either
// because a ContentSessionSwitching event named a new session or because a
// bare disconnect's retry succeeded against the same one.
type reconnectedMsg struct {
	sessionID string
	seed      []state.Event
	events    <-chan state.Event
	client    *transporthttp.Client
	token     string
	err       error
}

// ConnectionUpdater is an optional SurfaceModel capability: a surface whose
// own mutations (POSTs — toggling an event, pinning a result) go through a
// transport client it holds itself, not just Host's read-only
// seed/subscribe. Host calls UpdateConnection after a successful reconnect
// so those mutations keep targeting the newly attached session's endpoint
// and token, not stale ones from before the switch — without this, a
// surface's writes would silently keep failing (or worse, silently target
// the wrong session) after every reconnect
// (docs/architecture/behavior/session_resume.feature.md §5). A read-only
// surface with no mutations of its own (e.g. logs) doesn't implement this;
// Host type-asserts for it rather than requiring every SurfaceModel to.
type ConnectionUpdater interface {
	UpdateConnection(cl *transporthttp.Client, token string)
}

// reconnectPollInterval bounds how often Host retries resolving and
// attaching to a session after its stream unexpectedly closes — a
// newly-started process may not have finished writing its transport.json
// yet (docs/architecture/behavior/session_resume.feature.md §5).
const reconnectPollInterval = 500 * time.Millisecond

// maxReconnectAttempts bounds how long Host keeps retrying before giving up
// and quitting — a permanently gone session must not poll forever.
const maxReconnectAttempts = 20 // ~10s at reconnectPollInterval

// Host is the Bubble Tea model wrapping a SurfaceModel with the attach lifecycle.
type Host struct {
	surface  SurfaceModel
	title    string
	seed     []state.Event
	events   <-chan state.Event
	shutdown func()
	width    int
	height   int

	// Reconnect state. Reconnecting re-runs the exact same seed+subscribe
	// bootstrap Run already does once (via attach below), just repeatably —
	// not new bespoke logic. sessionRoot+surfaceID are enough to re-resolve
	// a session's current endpoint from disk (session.Store.ReadTransport)
	// without needing internal/cli, which internal/surfaces/client is not
	// allowed to import (docs/implementation/08_go_module_layout.md).
	// sessionRoot == "" (the launch-time root couldn't be resolved) simply
	// disables reconnection: a stream close quits exactly as it always did
	// before this existed.
	sessionRoot      string
	sessionID        string // the session Host is currently attached to
	surfaceID        string
	switchTarget     string // set by a received ContentSessionSwitching event; "" = none pending
	reconnectAttempt int    // reset to 0 on every successful (re)connect
}

// NewHost builds a host around a surface and its attach streams. seed is applied
// before live events; events is the live stream (resumed after the seed cursor);
// shutdown is invoked once on quit. sessionRoot/sessionID/surfaceID enable
// reconnection (docs/architecture/behavior/session_resume.feature.md §5);
// sessionRoot == "" leaves Host's original behavior unchanged (a closed
// stream quits).
func NewHost(surface SurfaceModel, title string, seed []state.Event, events <-chan state.Event, shutdown func(), sessionRoot, sessionID, surfaceID string) Host {
	return Host{
		surface: surface, title: title, seed: seed, events: events, shutdown: shutdown,
		width: 80, height: 24,
		sessionRoot: sessionRoot, sessionID: sessionID, surfaceID: surfaceID,
	}
}

// Init applies the seed, then begins listening for live events.
func (h Host) Init() tea.Cmd {
	return func() tea.Msg { return seedMsg(h.seed) }
}

// Update drives the attach lifecycle: seed, live events, resize, quit, reconnect.
func (h Host) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width = msg.Width
		h.height = msg.Height
		h.surface.SetSize(h.innerWidth(), h.innerHeight())
		return h, nil
	case seedMsg:
		for _, ev := range msg {
			h.applyTrackingSwitch(state.Event(ev))
		}
		return h, h.listen()
	case EventMsg:
		h.applyTrackingSwitch(state.Event(msg))
		return h, h.listen()
	case streamClosedMsg:
		if h.sessionRoot == "" {
			// No known session root: this Host was built before
			// reconnection existed (or launch-time root resolution
			// failed) — preserve the original behavior exactly.
			return h, tea.Quit
		}
		h.reconnectAttempt = 0
		return h, h.attemptReconnect()
	case reconnectTickMsg:
		return h, h.attemptReconnect()
	case reconnectedMsg:
		if msg.err != nil {
			h.reconnectAttempt++
			if h.reconnectAttempt >= maxReconnectAttempts {
				return h, tea.Quit
			}
			return h, tea.Tick(reconnectPollInterval, func(time.Time) tea.Msg { return reconnectTickMsg{} })
		}
		h.reconnectAttempt = 0
		h.surface.Reset()
		h.sessionID = msg.sessionID
		h.switchTarget = ""
		h.events = msg.events
		if cu, ok := h.surface.(ConnectionUpdater); ok && msg.client != nil {
			cu.UpdateConnection(msg.client, msg.token)
		}
		for _, ev := range msg.seed {
			h.surface.Apply(ev)
		}
		return h, h.listen()
	case tea.KeyPressMsg:
		if h.isQuit(msg) {
			if h.shutdown != nil {
				h.shutdown()
			}
			return h, tea.Quit
		}
		return h, h.surface.Key(msg)
	}
	return h, nil
}

// applyTrackingSwitch folds one event into the surface, additionally
// remembering the target session named by a ContentSessionSwitching event
// (never folded into the surface itself — DefaultEnabled(ContentSessionSwitching)
// is false, and it carries no rendering meaning for any surface).
func (h *Host) applyTrackingSwitch(ev state.Event) {
	if ev.ContentType == state.ContentSessionSwitching {
		if p, ok := ev.Payload.(map[string]any); ok {
			if id, ok := p["session_id"].(string); ok && id != "" {
				h.switchTarget = id
			}
		}
		return
	}
	h.surface.Apply(ev)
}

// attemptReconnect tries once to resolve and attach to a session: the
// switchTarget named by a received ContentSessionSwitching event if one
// arrived, otherwise the same session this Host was already attached to
// (the bare-disconnect fallback — a crash, not a deliberate switch, still
// recovers automatically). It re-resolves the endpoint/token from disk on
// every attempt rather than caching a value from an earlier one, so a
// changed port (the preferred-port handoff failed to land) is picked up as
// soon as the new process's transport.json reflects it
// (docs/architecture/behavior/session_resume.feature.md §5). Attempt
// counting and give-up-after-exhaustion live in Update's reconnectedMsg
// case, not here — this always tries exactly once and reports success/error.
func (h Host) attemptReconnect() tea.Cmd {
	target := h.switchTarget
	if target == "" {
		target = h.sessionID
	}
	root := h.sessionRoot
	surfaceID := h.surfaceID
	return func() tea.Msg {
		store := session.NewStore(root)
		info, err := store.ReadTransport(target)
		if err != nil {
			return reconnectedMsg{err: err}
		}
		if info.Endpoint == "" {
			return reconnectedMsg{err: errNoEndpoint}
		}
		token, _ := store.ReadAttachToken(target)
		c := transporthttp.NewClient(info.Endpoint)
		ctx := context.Background()
		seed, err := c.Seed(ctx)
		if err != nil {
			return reconnectedMsg{err: err}
		}
		var last uint64
		if n := len(seed); n > 0 {
			last = seed[n-1].Ordinal
		}
		events, err := c.Subscribe(ctx, last, surfaceID)
		if err != nil {
			return reconnectedMsg{err: err}
		}
		return reconnectedMsg{sessionID: target, seed: seed, events: events, client: c, token: token}
	}
}

// listen blocks for the next live event, mapping a closed stream to a quit signal.
func (h Host) listen() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-h.events
		if !ok {
			return streamClosedMsg{}
		}
		return EventMsg(ev)
	}
}

// View frames the surface body under a title strip.
func (h Host) View() tea.View {
	v := tea.NewView(h.frame(h.surface.View()))
	v.AltScreen = true
	return v
}

func (h Host) frame(body string) string {
	if h.title == "" {
		return body
	}
	line := "─ " + h.title + " "
	if n := len([]rune(line)); h.width > n {
		line += strings.Repeat("─", h.width-n)
	}
	return line + "\n" + body
}

func (h Host) innerWidth() int {
	if h.width < 0 {
		return 0
	}
	return h.width
}

func (h Host) innerHeight() int {
	if h.height-1 < 0 {
		return 0
	}
	return h.height - 1
}

// isQuit reports whether msg is the quit key given the surface's current
// input-capture state (SS-8). Ctrl-C always quits, even mid-capture; "q"
// quits only when the surface isn't capturing free-form text, since a
// captured search pattern may contain a literal "q".
func (h Host) isQuit(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "ctrl+c":
		return true
	case "q":
		return !h.surface.CapturesKeys()
	}
	return false
}

// Options configure a surface attach run.
type Options struct {
	Endpoint  string
	Token     string
	SurfaceID string
	Title     string
	Surface   SurfaceModel
	// SessionRoot and SessionID enable reconnection after the stream closes
	// (docs/architecture/behavior/session_resume.feature.md §5).
	// SessionRoot == "" disables it — Host then falls back to the original
	// quit-on-close behavior.
	SessionRoot string
	SessionID   string
}

// Run attaches the surface to a running orchestrator: it seeds from the durable
// log, resumes the live stream after the seed cursor, and runs the Bubble Tea
// program until the user quits or reconnection is exhausted.
func Run(ctx context.Context, opts Options) error {
	c := transporthttp.NewClient(opts.Endpoint)
	seed, _ := c.Seed(ctx)
	var last uint64
	if n := len(seed); n > 0 {
		last = seed[n-1].Ordinal
	}
	ch, err := c.Subscribe(ctx, last, opts.SurfaceID)
	if err != nil {
		return err
	}
	host := NewHost(opts.Surface, opts.Title, seed, ch, func() {
		_ = c.Shutdown(context.Background(), opts.SurfaceID, opts.Token)
	}, opts.SessionRoot, opts.SessionID, opts.SurfaceID)
	p := tea.NewProgram(host, tea.WithContext(ctx))
	_, err = p.Run()
	return err
}
