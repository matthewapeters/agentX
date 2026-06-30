// Package client is the shared Bubble Tea host for external rendering surfaces
// (SS-2). It attaches to a running orchestrator over the transport, seeds from the
// durable log, resumes the live event stream by cursor, and drives a SurfaceModel
// — the per-surface contract that concrete surfaces (context, files, config, …)
// implement. The host owns the attach lifecycle (seed → live → resize → quit); each
// surface owns only its projection and rendering.
package client

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"agentx/internal/state"
	transporthttp "agentx/internal/transport/http"
)

// SurfaceModel is the per-surface contract the host drives.
type SurfaceModel interface {
	// Apply folds one session event into the surface's projection.
	Apply(ev state.Event)
	// SetSize sets the inner render area (inside the host's title strip).
	SetSize(width, height int)
	// Key handles a surface-specific key (scroll, etc.). Global keys (quit) are
	// handled by the host before this is called.
	Key(msg tea.KeyPressMsg)
	// View renders the surface body to its current size.
	View() string
}

// EventMsg carries one live session event to the host.
type EventMsg state.Event

// seedMsg carries the durable seed snapshot, applied before the live stream.
type seedMsg []state.Event

// streamClosedMsg signals the event stream ended (orchestrator gone); the host quits.
type streamClosedMsg struct{}

// Host is the Bubble Tea model wrapping a SurfaceModel with the attach lifecycle.
type Host struct {
	surface  SurfaceModel
	title    string
	seed     []state.Event
	events   <-chan state.Event
	shutdown func()
	width    int
	height   int
}

// NewHost builds a host around a surface and its attach streams. seed is applied
// before live events; events is the live stream (resumed after the seed cursor);
// shutdown is invoked once on quit.
func NewHost(surface SurfaceModel, title string, seed []state.Event, events <-chan state.Event, shutdown func()) Host {
	return Host{surface: surface, title: title, seed: seed, events: events, shutdown: shutdown, width: 80, height: 24}
}

// Init applies the seed, then begins listening for live events.
func (h Host) Init() tea.Cmd {
	return func() tea.Msg { return seedMsg(h.seed) }
}

// Update drives the attach lifecycle: seed, live events, resize, quit.
func (h Host) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width = msg.Width
		h.height = msg.Height
		h.surface.SetSize(h.innerWidth(), h.innerHeight())
		return h, nil
	case seedMsg:
		for _, ev := range msg {
			h.surface.Apply(state.Event(ev))
		}
		return h, h.listen()
	case EventMsg:
		h.surface.Apply(state.Event(msg))
		return h, h.listen()
	case streamClosedMsg:
		return h, tea.Quit
	case tea.KeyPressMsg:
		if isQuit(msg) {
			if h.shutdown != nil {
				h.shutdown()
			}
			return h, tea.Quit
		}
		h.surface.Key(msg)
		return h, nil
	}
	return h, nil
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

func isQuit(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "ctrl+c", "q":
		return true
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
}

// Run attaches the surface to a running orchestrator: it seeds from the durable
// log, resumes the live stream after the seed cursor, and runs the Bubble Tea
// program until the user quits or the stream ends.
func Run(ctx context.Context, opts Options) error {
	c := transporthttp.NewClient(opts.Endpoint)
	seed, _ := c.Seed(ctx)
	var last uint64
	if n := len(seed); n > 0 {
		last = seed[n-1].Ordinal
	}
	ch, err := c.Subscribe(ctx, last)
	if err != nil {
		return err
	}
	host := NewHost(opts.Surface, opts.Title, seed, ch, func() {
		_ = c.Shutdown(context.Background(), opts.SurfaceID, opts.Token)
	})
	p := tea.NewProgram(host, tea.WithContext(ctx))
	_, err = p.Run()
	return err
}
