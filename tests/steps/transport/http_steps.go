// Package transportsteps implements the Godog steps for the HTTP/SSE transport
// (TRN-2+). It drives a real httptest server backed by an in-memory provider
// (bus, processing publisher, registry, session identity).
package transportsteps

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/cli"
	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/surfaces"
	"agentx/internal/surfaces/client"
	transporthttp "agentx/internal/transport/http"
)

// fakeProvider is an in-memory transporthttp.Provider for the steps. Submit
// echoes the prompt back as an agent_response event so the wire path is
// observable over SSE.
type fakeProvider struct {
	bus  *state.Bus
	proc *state.ProcessingPublisher
	sess session.Identity
	reg  *surfaces.Registry

	mu           sync.Mutex
	accepting    bool
	lastDecision string
	history      []state.Event
	wm           session.WorkingMemory
}

func (p *fakeProvider) Bus() *state.Bus                        { return p.bus }
func (p *fakeProvider) Processing() *state.ProcessingPublisher { return p.proc }
func (p *fakeProvider) Session() session.Identity              { return p.sess }
func (p *fakeProvider) Registry() *surfaces.Registry           { return p.reg }

func (p *fakeProvider) Submit(_ context.Context, text string) error {
	p.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   p.sess.ID,
		EventType:   "AGENT_RESPONSE",
		ContentType: state.ContentAgentResponse,
		Payload:     map[string]any{"text": text},
	})
	return nil
}

func (p *fakeProvider) Resolve(decision string) {
	p.mu.Lock()
	p.lastDecision = decision
	p.mu.Unlock()
}

func (p *fakeProvider) Accepting() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.accepting
}

func (p *fakeProvider) History() ([]state.Event, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]state.Event, len(p.history))
	copy(out, p.history)
	return out, nil
}

// publishRecorded publishes an event on the bus (stamping its ordinal) and mirrors
// it into the fake durable history, simulating the recorder synchronously so seed
// + resume can be tested deterministically.
func (p *fakeProvider) publishRecorded(ev state.Event) {
	p.bus.Publish(ev)
	p.mu.Lock()
	// Re-stamp from the bus so history carries the same ordinal as the live event.
	ev.Ordinal = p.bus.CurrentOrdinal()
	p.history = append(p.history, ev)
	p.mu.Unlock()
}

func (p *fakeProvider) WorkingMemory() ([]session.Fact, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]session.Fact, len(p.wm.Facts))
	copy(out, p.wm.Facts)
	return out, nil
}

func (p *fakeProvider) SetFact(key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.wm.Set(key, value)
	return nil
}

func (p *fakeProvider) DeleteFact(key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.wm.Delete(key)
	return nil
}

func (p *fakeProvider) SetFactEnabled(key string, enabled bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.wm.SetEnabled(key, enabled) {
		return fmt.Errorf("unknown fact %q", key)
	}
	return nil
}

func (p *fakeProvider) decision() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastDecision
}

type transportWorld struct {
	prov    *fakeProvider
	token   surfaces.AttachToken
	httptst *httptest.Server

	authToken string

	respStatus int
	respBody   []byte

	streams     []*sseStream
	connStreams map[string]*sseStream // events streams opened with a surface_id (SS-4)

	launchResult cli.LaunchResult
	launchErr    error
	tmpRoot        string             // temp session root for auto-discovery launch (SS-5)
	wmErr          error              // last working-memory mutation error (SS-6)
	presenceCancel context.CancelFunc // held presence stream (SS-4 / SS-6)

	seed       []state.Event
	cursor     uint64
	liveCh     <-chan state.Event
	liveCancel context.CancelFunc
	received   []state.Event
	recorded   int
}

// sseStream reads SSE data lines from an open /events connection on a goroutine.
type sseStream struct {
	cancel context.CancelFunc
	body   io.ReadCloser
	lines  chan string
}

// InitializeScenario registers the transport steps.
func InitializeScenario(sc *godog.ScenarioContext) {
	registerPortSteps(sc)

	w := &transportWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		for _, st := range w.streams {
			st.cancel()
			_ = st.body.Close()
		}
		if w.liveCancel != nil {
			w.liveCancel()
		}
		if w.httptst != nil {
			w.httptst.Close()
		}
		if w.tmpRoot != "" {
			_ = os.RemoveAll(w.tmpRoot)
		}
		if w.presenceCancel != nil {
			w.presenceCancel()
		}
		*w = transportWorld{}
		return ctx, err
	})

	sc.Step(`^a running transport server$`, w.serverNamed("calm-otter"))
	sc.Step(`^a running transport server for session "([^"]*)"$`, w.serverFor)
	sc.Step(`^the processing state is set to "([^"]*)" phase "([^"]*)"$`, w.setProcessing)
	sc.Step(`^a surface "([^"]*)" is registered on the transport$`, w.registerSurface)
	sc.Step(`^an open events stream$`, w.openOneStream)
	sc.Step(`^(\d+) open events streams$`, w.openStreams)
	sc.Step(`^a client GETs "([^"]*)"$`, w.get)
	sc.Step(`^an agent_response event "([^"]*)" is published$`, w.publishResponse)
	sc.Step(`^the response status is (\d+)$`, w.statusIs)
	sc.Step(`^the response JSON field "([^"]*)" is "([^"]*)"$`, w.jsonFieldIs)
	sc.Step(`^the response JSON field "([^"]*)" is not empty$`, w.jsonFieldNotEmpty)
	sc.Step(`^the response body contains "([^"]*)"$`, w.bodyContains)
	sc.Step(`^the events stream delivers "([^"]*)"$`, w.streamDelivers)
	sc.Step(`^all events streams deliver "([^"]*)"$`, w.allStreamsDeliver)

	// Write endpoints (TRN-3).
	sc.Step(`^a running transport server that is not accepting prompts$`, w.serverNotAccepting)
	sc.Step(`^the client is authorized with the attach token$`, w.authorize)
	sc.Step(`^a client POSTs to "/surface/register" with a valid registration$`, w.postRegisterValid)
	sc.Step(`^a client POSTs to "/surface/register" with token "([^"]*)"$`, w.postRegisterToken)
	sc.Step(`^a client POSTs to "/surface/register" for id "([^"]*)" with a valid token$`, w.postRegisterID)
	sc.Step(`^a client POSTs "/prompt" with text "([^"]*)"$`, w.postPrompt)
	sc.Step(`^a client POSTs "/tool/approval" with decision "([^"]*)"$`, w.postApproval)
	sc.Step(`^a client POSTs to "/surface/([^"]*)/shutdown"$`, w.postShutdown)
	sc.Step(`^a client POSTs "/model/switch"$`, w.postModelSwitch)
	sc.Step(`^the orchestrator received decision "([^"]*)"$`, w.receivedDecision)
	sc.Step(`^the surface "([^"]*)" on the transport has lifecycle "([^"]*)"$`, w.lifecycleOf)

	// seed + resume (SS-1).
	sc.Step(`^(\d+) events are recorded$`, w.recordEvents)
	sc.Step(`^a recorded user_prompt event "([^"]*)"$`, w.recordPrompt)
	sc.Step(`^(\d+) more events? (?:is|are) recorded$`, w.recordEvents)
	sc.Step(`^a client seeds the session events$`, w.seedEvents)
	sc.Step(`^the seed has (\d+) events with ordinals "([^"]*)"$`, w.seedOrdinals)
	sc.Step(`^the seed contains "([^"]*)"$`, w.seedContains)
	sc.Step(`^the seed event "([^"]*)" is enabled$`, w.seedEnabled)
	sc.Step(`^the client subscribes after the seed cursor$`, w.subscribeAfterSeed)
	sc.Step(`^the client subscribes from the beginning$`, w.subscribeFromZero)
	sc.Step(`^the live stream delivers exactly (\d+) events$`, w.liveDeliversExactly)
	sc.Step(`^the live stream delivers no event at or before the cursor$`, w.liveNoneBeforeCursor)

	// connection liveness (SS-4).
	sc.Step(`^surface "([^"]*)" connects its events stream$`, w.connectEventsStream)
	sc.Step(`^surface "([^"]*)" disconnects its events stream$`, w.disconnectEventsStream)
	sc.Step(`^the transport reports connected kinds "([^"]*)"$`, w.transportConnectedKinds)
	sc.Step(`^the transport connected kinds become "([^"]*)"$`, w.transportConnectedBecome)
	sc.Step(`^surface "([^"]*)" holds a presence stream$`, w.holdPresence)
	sc.Step(`^surface "([^"]*)" drops its presence stream$`, w.dropPresence)

	// surface launch CLI (TRN-5).
	sc.Step(`^I launch a "([^"]*)" surface for session "([^"]*)" with the valid token$`, w.launchValid)
	sc.Step(`^I launch a "([^"]*)" surface for session "([^"]*)" with token "([^"]*)"$`, w.launchToken)
	sc.Step(`^I launch a "([^"]*)" surface for session "([^"]*)" against an unreachable endpoint$`, w.launchUnreachable)
	sc.Step(`^I launch via the compatibility alias for the running server$`, w.launchAlias)
	sc.Step(`^the session's transport is published to a temp session root$`, w.publishTransportToDisk)
	sc.Step(`^I launch a "([^"]*)" surface with auto-discovery$`, w.launchAuto)

	// working-memory CRUD (SS-6).
	sc.Step(`^the client adds a fact "([^"]*)" valued "([^"]*)"$`, w.wmAdd)
	sc.Step(`^the client edits fact "([^"]*)" to "([^"]*)"$`, w.wmAdd)
	sc.Step(`^the client disables fact "([^"]*)"$`, w.wmDisable)
	sc.Step(`^the client enables fact "([^"]*)"$`, w.wmEnable)
	sc.Step(`^the client deletes fact "([^"]*)"$`, w.wmDelete)
	sc.Step(`^an unauthorized client adds a fact "([^"]*)" valued "([^"]*)"$`, w.wmUnauthorizedAdd)
	sc.Step(`^reading working memory includes "([^"]*)" valued "([^"]*)" enabled$`, w.wmIncludesEnabled)
	sc.Step(`^reading working memory shows "([^"]*)" disabled$`, w.wmShowsDisabled)
	sc.Step(`^reading working memory has no "([^"]*)" fact$`, w.wmHasNo)
	sc.Step(`^the working-memory mutation is rejected as "([^"]*)"$`, w.wmRejected)
	sc.Step(`^the launch succeeds$`, w.launchSucceeds)
	sc.Step(`^the launched surface kind is "([^"]*)"$`, w.launchedKind)
	sc.Step(`^the launched surface appears in the registry$`, w.launchedInRegistry)
	sc.Step(`^the launch fails with category "([^"]*)"$`, w.launchFailsCategory)
}

func (w *transportWorld) serverNamed(name string) func() error {
	return func() error { return w.serverFor(name) }
}

func (w *transportWorld) serverFor(name string) error {
	tok, err := surfaces.MintToken()
	if err != nil {
		return err
	}
	w.token = tok
	id := session.Identity{ID: "sess-1", Name: name, CreatedEpoch: 1}
	w.prov = &fakeProvider{
		bus:       state.NewBus(),
		proc:      state.NewProcessingPublisher(id.ID),
		sess:      id,
		reg:       surfaces.NewRegistry(tok, id.ID, id.Name),
		accepting: true,
	}
	w.httptst = httptest.NewServer(transporthttp.NewServer(w.prov).Handler())
	return nil
}

// serverNotAccepting builds a server whose orchestrator rejects new prompts.
func (w *transportWorld) serverNotAccepting() error {
	if err := w.serverFor("calm-otter"); err != nil {
		return err
	}
	w.prov.accepting = false
	return nil
}

func (w *transportWorld) setProcessing(stateName, phase string) error {
	w.prov.proc.Set(state.RunState(stateName), state.Phase(phase))
	return nil
}

func (w *transportWorld) registerSurface(id string) error {
	_, err := w.prov.reg.Register(surfaces.RegisterRequest{
		SurfaceID:        id,
		SurfaceKind:      "files",
		TransportAddress: "http://127.0.0.1:7777",
		Token:            w.token.Raw(),
	})
	return err
}

func (w *transportWorld) get(path string) error {
	resp, err := http.Get(w.httptst.URL + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	w.respStatus = resp.StatusCode
	w.respBody = body
	return nil
}

// connectEventsStream opens an SSE stream tagged with surface_id, so the server
// marks that registered surface live (SS-4). Headers arrive after the handler has
// called MarkLive, so connection status is observable as soon as this returns.
func (w *transportWorld) connectEventsStream(id string) error {
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.httptst.URL+"/events?surface_id="+url.QueryEscape(id), nil)
	if err != nil {
		cancel()
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return err
	}
	st := &sseStream{cancel: cancel, body: resp.Body, lines: make(chan string, 64)}
	if w.connStreams == nil {
		w.connStreams = make(map[string]*sseStream)
	}
	w.connStreams[id] = st
	w.streams = append(w.streams, st) // ensure After cancels it
	return nil
}

func (w *transportWorld) disconnectEventsStream(id string) error {
	st, ok := w.connStreams[id]
	if !ok {
		return fmt.Errorf("no connected events stream for surface %q", id)
	}
	st.cancel()
	_ = st.body.Close()
	delete(w.connStreams, id)
	return nil
}

func (w *transportWorld) holdPresence(id string) error {
	w.presenceCancel = client.Presence(context.Background(), w.httptst.URL, id)
	return nil
}

func (w *transportWorld) dropPresence(string) error {
	if w.presenceCancel != nil {
		w.presenceCancel()
		w.presenceCancel = nil
	}
	return nil
}

func (w *transportWorld) transportConnectedKinds(want string) error {
	if got := strings.Join(w.prov.reg.ConnectedKinds(), ", "); got != want {
		return fmt.Errorf("connected kinds = %q, want %q", got, want)
	}
	return nil
}

// transportConnectedBecome polls, since MarkDead runs asynchronously after the
// stream's context is canceled and the handler returns.
func (w *transportWorld) transportConnectedBecome(want string) error {
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := strings.Join(w.prov.reg.ConnectedKinds(), ", ")
		if got == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("connected kinds = %q, want %q (timed out)", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (w *transportWorld) openOneStream() error { return w.openStreams(1) }

func (w *transportWorld) openStreams(n int) error {
	for range n {
		st, err := w.openStream()
		if err != nil {
			return err
		}
		w.streams = append(w.streams, st)
	}
	return nil
}

func (w *transportWorld) openStream() (*sseStream, error) {
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.httptst.URL+"/events", nil)
	if err != nil {
		cancel()
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	st := &sseStream{cancel: cancel, body: resp.Body, lines: make(chan string, 64)}
	go func() {
		defer close(st.lines)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if data, ok := strings.CutPrefix(line, "data:"); ok {
				select {
				case st.lines <- strings.TrimSpace(data):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return st, nil
}

func (w *transportWorld) publishResponse(text string) error {
	w.prov.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   w.prov.sess.ID,
		EventType:   "AGENT_RESPONSE",
		ContentType: state.ContentAgentResponse,
		Payload:     map[string]any{"text": text},
	})
	return nil
}

func (w *transportWorld) statusIs(want int) error {
	if w.respStatus != want {
		return fmt.Errorf("response status = %d, want %d", w.respStatus, want)
	}
	return nil
}

func (w *transportWorld) jsonField(field string) (string, error) {
	var m map[string]any
	if err := json.Unmarshal(w.respBody, &m); err != nil {
		return "", fmt.Errorf("decode JSON body: %w (body=%s)", err, w.respBody)
	}
	v, ok := m[field]
	if !ok {
		return "", fmt.Errorf("field %q absent from body %s", field, w.respBody)
	}
	return fmt.Sprintf("%v", v), nil
}

func (w *transportWorld) jsonFieldIs(field, want string) error {
	got, err := w.jsonField(field)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("field %q = %q, want %q", field, got, want)
	}
	return nil
}

func (w *transportWorld) jsonFieldNotEmpty(field string) error {
	got, err := w.jsonField(field)
	if err != nil {
		return err
	}
	if got == "" {
		return fmt.Errorf("field %q is empty", field)
	}
	return nil
}

func (w *transportWorld) bodyContains(want string) error {
	if !strings.Contains(string(w.respBody), want) {
		return fmt.Errorf("body %s does not contain %q", w.respBody, want)
	}
	return nil
}

func (w *transportWorld) streamDelivers(want string) error {
	if len(w.streams) == 0 {
		return fmt.Errorf("no open events stream")
	}
	return w.streams[0].waitFor(want, 3*time.Second)
}

func (w *transportWorld) allStreamsDeliver(want string) error {
	if len(w.streams) == 0 {
		return fmt.Errorf("no open events streams")
	}
	for i, st := range w.streams {
		if err := st.waitFor(want, 3*time.Second); err != nil {
			return fmt.Errorf("stream %d: %w", i, err)
		}
	}
	return nil
}

func (w *transportWorld) record(text string) {
	w.prov.publishRecorded(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   w.prov.sess.ID,
		EventType:   "USER_PROMPT",
		ContentType: state.ContentUserPrompt,
		Payload:     map[string]any{"text": text},
		Enabled:     state.DefaultEnabled(state.ContentUserPrompt),
	})
}

func (w *transportWorld) recordEvents(n int) error {
	for range n {
		w.recorded++
		w.record(fmt.Sprintf("ev-%d", w.recorded))
	}
	return nil
}

func (w *transportWorld) recordPrompt(text string) error {
	w.record(text)
	return nil
}

func (w *transportWorld) seedEvents() error {
	seed, err := transporthttp.NewClient(w.httptst.URL).Seed(context.Background())
	if err != nil {
		return err
	}
	w.seed = seed
	return nil
}

func (w *transportWorld) seedOrdinals(n int, ords string) error {
	if len(w.seed) != n {
		return fmt.Errorf("seed has %d events, want %d", len(w.seed), n)
	}
	got := make([]string, len(w.seed))
	for i, ev := range w.seed {
		got[i] = fmt.Sprintf("%d", ev.Ordinal)
	}
	if j := strings.Join(got, ", "); j != ords {
		return fmt.Errorf("seed ordinals = %q, want %q", j, ords)
	}
	return nil
}

func (w *transportWorld) seedContains(text string) error {
	for _, ev := range w.seed {
		if payloadText(ev) == text {
			return nil
		}
	}
	return fmt.Errorf("seed does not contain %q", text)
}

func (w *transportWorld) seedEnabled(text string) error {
	for _, ev := range w.seed {
		if payloadText(ev) == text {
			if !ev.Enabled {
				return fmt.Errorf("seed event %q is not enabled", text)
			}
			return nil
		}
	}
	return fmt.Errorf("seed event %q not found", text)
}

func (w *transportWorld) subscribeAfterSeed() error {
	if len(w.seed) > 0 {
		w.cursor = w.seed[len(w.seed)-1].Ordinal
	}
	return w.subscribe(w.cursor)
}

func (w *transportWorld) subscribeFromZero() error {
	w.cursor = 0
	return w.subscribe(0)
}

func (w *transportWorld) subscribe(after uint64) error {
	return w.subscribeAs(after, "")
}

func (w *transportWorld) subscribeAs(after uint64, surfaceID string) error {
	ctx, cancel := context.WithCancel(context.Background())
	w.liveCancel = cancel
	ch, err := transporthttp.NewClient(w.httptst.URL).Subscribe(ctx, after, surfaceID)
	if err != nil {
		cancel()
		return err
	}
	w.liveCh = ch
	return nil
}

func (w *transportWorld) liveDeliversExactly(n int) error {
	got := readN(w.liveCh, n, 3*time.Second)
	if len(got) != n {
		return fmt.Errorf("live stream delivered %d events, want %d", len(got), n)
	}
	if extra := readN(w.liveCh, 1, 200*time.Millisecond); len(extra) > 0 {
		return fmt.Errorf("live stream delivered more than %d events", n)
	}
	w.received = got
	return nil
}

func (w *transportWorld) liveNoneBeforeCursor() error {
	for _, ev := range w.received {
		if ev.Ordinal <= w.cursor {
			return fmt.Errorf("received event ordinal %d at or before cursor %d", ev.Ordinal, w.cursor)
		}
	}
	return nil
}

// payloadText extracts the "text" field from an event payload (post JSON decode).
func payloadText(ev state.Event) string {
	m, ok := ev.Payload.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m["text"].(string)
	return s
}

// readN reads up to n events from ch or returns early on timeout.
func readN(ch <-chan state.Event, n int, timeout time.Duration) []state.Event {
	out := make([]state.Event, 0, n)
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			return out
		}
	}
	return out
}

func (w *transportWorld) authorize() error {
	w.authToken = w.token.Raw()
	return nil
}

// post sends a JSON POST, attaching the bearer token when authorized, and stores
// the response status/body.
func (w *transportWorld) post(path string, body any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequest(http.MethodPost, w.httptst.URL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if w.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+w.authToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	w.respStatus = resp.StatusCode
	w.respBody = data
	return nil
}

func (w *transportWorld) postRegisterValid() error {
	return w.post("/surface/register", map[string]any{
		"surface_kind":      "files",
		"transport_address": "http://127.0.0.1:7777",
		"token":             w.token.Raw(),
	})
}

func (w *transportWorld) postRegisterToken(token string) error {
	return w.post("/surface/register", map[string]any{
		"surface_kind":      "files",
		"transport_address": "http://127.0.0.1:7777",
		"token":             token,
	})
}

func (w *transportWorld) postRegisterID(id string) error {
	return w.post("/surface/register", map[string]any{
		"surface_id":        id,
		"surface_kind":      "files",
		"transport_address": "http://127.0.0.1:7777",
		"token":             w.token.Raw(),
	})
}

func (w *transportWorld) postPrompt(text string) error {
	return w.post("/prompt", map[string]any{"text": text})
}

func (w *transportWorld) postApproval(decision string) error {
	return w.post("/tool/approval", map[string]any{"decision": decision})
}

func (w *transportWorld) postShutdown(id string) error {
	return w.post("/surface/"+id+"/shutdown", nil)
}

func (w *transportWorld) postModelSwitch() error {
	return w.post("/model/switch", nil)
}

func (w *transportWorld) receivedDecision(want string) error {
	if got := w.prov.decision(); got != want {
		return fmt.Errorf("orchestrator decision = %q, want %q", got, want)
	}
	return nil
}

func (w *transportWorld) lifecycleOf(id, lifecycle string) error {
	reg, ok := w.prov.reg.Get(id)
	if !ok {
		return fmt.Errorf("surface %q not found", id)
	}
	if reg.LifecycleState != lifecycle {
		return fmt.Errorf("surface %q lifecycle = %q, want %q", id, reg.LifecycleState, lifecycle)
	}
	return nil
}

func (w *transportWorld) launch(args cli.LaunchArgs) error {
	w.launchResult, w.launchErr = cli.Launch(context.Background(), args)
	return nil
}

// publishTransportToDisk writes the running server's endpoint + attach token to a
// temp session root, so an auto-discovery launch (no flags) can resolve them (SS-5).
func (w *transportWorld) publishTransportToDisk() error {
	root, err := os.MkdirTemp("", "agentx-ss5-*")
	if err != nil {
		return err
	}
	w.tmpRoot = root
	store := session.NewStore(root)
	id := w.prov.sess.ID
	if err := os.MkdirAll(store.Dir(id), 0o700); err != nil {
		return err
	}
	if err := store.WriteTransport(id, session.TransportInfo{SessionID: id, Endpoint: w.httptst.URL}); err != nil {
		return err
	}
	return store.WriteAttachToken(id, w.token.Raw())
}

func (w *transportWorld) launchAuto(kind string) error {
	return w.launch(cli.LaunchArgs{SurfaceKind: kind, SessionRoot: w.tmpRoot})
}

func (w *transportWorld) wmClient() *transporthttp.Client {
	return transporthttp.NewClient(w.httptst.URL)
}

func (w *transportWorld) wmAdd(key, value string) error {
	return w.wmClient().SetFact(context.Background(), w.token.Raw(), key, value)
}

func (w *transportWorld) wmDisable(key string) error {
	return w.wmClient().SetFactEnabled(context.Background(), w.token.Raw(), key, false)
}

func (w *transportWorld) wmEnable(key string) error {
	return w.wmClient().SetFactEnabled(context.Background(), w.token.Raw(), key, true)
}

func (w *transportWorld) wmDelete(key string) error {
	return w.wmClient().DeleteFact(context.Background(), w.token.Raw(), key)
}

func (w *transportWorld) wmUnauthorizedAdd(key, value string) error {
	w.wmErr = w.wmClient().SetFact(context.Background(), "", key, value)
	return nil
}

func (w *transportWorld) wmFind(key string) (session.Fact, error) {
	facts, err := w.wmClient().WorkingMemory(context.Background())
	if err != nil {
		return session.Fact{}, err
	}
	for _, f := range facts {
		if f.Key == key {
			return f, nil
		}
	}
	return session.Fact{}, fmt.Errorf("fact %q not found", key)
}

func (w *transportWorld) wmIncludesEnabled(key, value string) error {
	f, err := w.wmFind(key)
	if err != nil {
		return err
	}
	if f.Value != value {
		return fmt.Errorf("fact %q value = %q, want %q", key, f.Value, value)
	}
	if !f.Enabled {
		return fmt.Errorf("fact %q is disabled, want enabled", key)
	}
	return nil
}

func (w *transportWorld) wmShowsDisabled(key string) error {
	f, err := w.wmFind(key)
	if err != nil {
		return err
	}
	if f.Enabled {
		return fmt.Errorf("fact %q is enabled, want disabled", key)
	}
	return nil
}

func (w *transportWorld) wmHasNo(key string) error {
	if _, err := w.wmFind(key); err == nil {
		return fmt.Errorf("fact %q is still present", key)
	}
	return nil
}

func (w *transportWorld) wmRejected(category string) error {
	if w.wmErr == nil {
		return fmt.Errorf("expected the mutation to be rejected, got success")
	}
	var ae *transporthttp.AttachError
	if errors.As(w.wmErr, &ae) {
		if ae.Category != category {
			return fmt.Errorf("rejection category = %q, want %q", ae.Category, category)
		}
		return nil
	}
	return fmt.Errorf("error is not an AttachError: %v", w.wmErr)
}

func (w *transportWorld) launchValid(kind, sessionSel string) error {
	return w.launch(cli.LaunchArgs{
		SurfaceKind: kind,
		Session:     sessionSel,
		Connect:     w.httptst.URL,
		Token:       w.token.Raw(),
	})
}

func (w *transportWorld) launchToken(kind, sessionSel, token string) error {
	return w.launch(cli.LaunchArgs{
		SurfaceKind: kind,
		Session:     sessionSel,
		Connect:     w.httptst.URL,
		Token:       token,
	})
}

func (w *transportWorld) launchUnreachable(kind, sessionSel string) error {
	return w.launch(cli.LaunchArgs{
		SurfaceKind: kind,
		Session:     sessionSel,
		Connect:     "http://127.0.0.1:1",
		Token:       w.token.Raw(),
	})
}

func (w *transportWorld) launchAlias() error {
	u, err := url.Parse(w.httptst.URL)
	if err != nil {
		return err
	}
	cmd, err := cli.Parse([]string{"-l", "files", "-s", "calm-otter", "-p", u.Port(), "-t", w.token.Raw()})
	if err != nil {
		w.launchErr = err
		return nil
	}
	if cmd.Launch == nil {
		return fmt.Errorf("alias did not parse to a launch command")
	}
	return w.launch(*cmd.Launch)
}

func (w *transportWorld) launchSucceeds() error {
	if w.launchErr != nil {
		return fmt.Errorf("launch failed: %v", w.launchErr)
	}
	return nil
}

func (w *transportWorld) launchedKind(want string) error {
	if w.launchResult.SurfaceKind != want {
		return fmt.Errorf("launched kind = %q, want %q", w.launchResult.SurfaceKind, want)
	}
	return nil
}

func (w *transportWorld) launchedInRegistry() error {
	for _, reg := range w.prov.reg.List() {
		if reg.SurfaceID == w.launchResult.SurfaceID {
			return nil
		}
	}
	return fmt.Errorf("launched surface %q not found in registry", w.launchResult.SurfaceID)
}

func (w *transportWorld) launchFailsCategory(category string) error {
	if w.launchErr == nil {
		return fmt.Errorf("launch succeeded, expected failure with category %q", category)
	}
	var ae *transporthttp.AttachError
	if !errors.As(w.launchErr, &ae) {
		return fmt.Errorf("error %v is not an *AttachError", w.launchErr)
	}
	if ae.Category != category {
		return fmt.Errorf("category = %q, want %q", ae.Category, category)
	}
	return nil
}

func (st *sseStream) waitFor(want string, timeout time.Duration) error {
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-st.lines:
			if !ok {
				return fmt.Errorf("stream closed before %q arrived", want)
			}
			if strings.Contains(line, want) {
				return nil
			}
		case <-deadline:
			return fmt.Errorf("timed out waiting for %q", want)
		}
	}
}
