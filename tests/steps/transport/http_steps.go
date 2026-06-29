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
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/cli"
	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/surfaces"
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

	streams []*sseStream

	launchResult cli.LaunchResult
	launchErr    error
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
		if w.httptst != nil {
			w.httptst.Close()
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

	// surface launch CLI (TRN-5).
	sc.Step(`^I launch a "([^"]*)" surface for session "([^"]*)" with the valid token$`, w.launchValid)
	sc.Step(`^I launch a "([^"]*)" surface for session "([^"]*)" with token "([^"]*)"$`, w.launchToken)
	sc.Step(`^I launch a "([^"]*)" surface for session "([^"]*)" against an unreachable endpoint$`, w.launchUnreachable)
	sc.Step(`^I launch via the compatibility alias for the running server$`, w.launchAlias)
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
