package surfacesteps

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/cucumber/godog"

	"agentx/internal/state"
	"agentx/internal/surfaces/client"
)

// fakeSurface is a SurfaceModel that records the host's calls.
type fakeSurface struct {
	applied int
	width   int
	height  int
	keys    []string
}

func (f *fakeSurface) Apply(state.Event)       { f.applied++ }
func (f *fakeSurface) SetSize(w, h int)        { f.width = w; f.height = h }
func (f *fakeSurface) Key(msg tea.KeyPressMsg) { f.keys = append(f.keys, msg.String()) }
func (f *fakeSurface) View() string            { return "" }

type clientWorld struct {
	fake           *fakeSurface
	ch             chan state.Event
	host           client.Host
	lastCmd        tea.Cmd
	shutdownCalled bool
}

// registerClientSteps wires the surface-host reducer steps (SS-2).
func registerClientSteps(sc *godog.ScenarioContext) {
	w := &clientWorld{}

	sc.Step(`^a surface host seeded with (\d+) events$`, w.seeded)
	sc.Step(`^the host initializes$`, w.initialize)
	sc.Step(`^a live event arrives$`, w.liveArrives)
	sc.Step(`^the event stream closes$`, w.streamCloses)
	sc.Step(`^the host receives a window size (\d+) by (\d+)$`, w.windowSize)
	sc.Step(`^the host receives the "([^"]*)" key$`, w.keyPress)
	sc.Step(`^the surface has applied (\d+) events$`, w.applied)
	sc.Step(`^the surface size is (\d+) by (\d+)$`, w.surfaceSize)
	sc.Step(`^the surface shutdown was requested$`, w.shutdownRequested)
	sc.Step(`^the host signals quit$`, w.signalsQuit)
	sc.Step(`^the surface received key "([^"]*)"$`, w.receivedKey)
}

func makeClientEvent(ordinal uint64) state.Event {
	return state.Event{
		Epoch:       1,
		SessionID:   "s",
		EventType:   "USER_PROMPT",
		ContentType: state.ContentUserPrompt,
		Payload:     map[string]any{"text": "x"},
		Enabled:     true,
		Ordinal:     ordinal,
	}
}

func (w *clientWorld) seeded(n int) error {
	w.fake = &fakeSurface{}
	w.ch = make(chan state.Event, 8)
	w.shutdownCalled = false
	seed := make([]state.Event, n)
	for i := range seed {
		seed[i] = makeClientEvent(uint64(i + 1))
	}
	w.host = client.NewHost(w.fake, "ctx", seed, w.ch, func() { w.shutdownCalled = true })
	return nil
}

// update applies a message to the host, keeping the resulting model and command.
func (w *clientWorld) update(msg tea.Msg) {
	m, cmd := w.host.Update(msg)
	w.host = m.(client.Host)
	w.lastCmd = cmd
}

func (w *clientWorld) initialize() error {
	cmd := w.host.Init()
	if cmd == nil {
		return fmt.Errorf("Init returned no command")
	}
	w.update(cmd())
	return nil
}

func (w *clientWorld) liveArrives() error {
	w.update(client.EventMsg(makeClientEvent(99)))
	return nil
}

func (w *clientWorld) streamCloses() error {
	close(w.ch)
	if w.lastCmd == nil {
		return fmt.Errorf("no pending listen command")
	}
	w.update(w.lastCmd())
	return nil
}

func (w *clientWorld) windowSize(width, height int) error {
	w.update(tea.WindowSizeMsg{Width: width, Height: height})
	return nil
}

func (w *clientWorld) keyPress(key string) error {
	w.update(tea.KeyPressMsg{Code: rune(key[0])})
	return nil
}

func (w *clientWorld) applied(n int) error {
	if w.fake.applied != n {
		return fmt.Errorf("surface applied %d events, want %d", w.fake.applied, n)
	}
	return nil
}

func (w *clientWorld) surfaceSize(width, height int) error {
	if w.fake.width != width || w.fake.height != height {
		return fmt.Errorf("surface size = %dx%d, want %dx%d", w.fake.width, w.fake.height, width, height)
	}
	return nil
}

func (w *clientWorld) shutdownRequested() error {
	if !w.shutdownCalled {
		return fmt.Errorf("surface shutdown was not requested")
	}
	return nil
}

func (w *clientWorld) signalsQuit() error {
	if w.lastCmd == nil {
		return fmt.Errorf("host produced no command")
	}
	if _, ok := w.lastCmd().(tea.QuitMsg); !ok {
		return fmt.Errorf("host did not signal quit")
	}
	return nil
}

func (w *clientWorld) receivedKey(key string) error {
	if slices.Contains(w.fake.keys, key) {
		return nil
	}
	return fmt.Errorf("surface did not receive key %q (got %v)", key, w.fake.keys)
}
