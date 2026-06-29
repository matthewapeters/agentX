// Package transportsteps implements the Godog steps for the HTTP/SSE transport
// (TRN-2+). It drives a real httptest server backed by an in-memory provider
// (bus, processing publisher, registry, session identity).
package transportsteps

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/surfaces"
	transporthttp "agentx/internal/transport/http"
)

// fakeProvider is an in-memory transporthttp.Provider for the steps.
type fakeProvider struct {
	bus  *state.Bus
	proc *state.ProcessingPublisher
	sess session.Identity
	reg  *surfaces.Registry
}

func (p *fakeProvider) Bus() *state.Bus                        { return p.bus }
func (p *fakeProvider) Processing() *state.ProcessingPublisher { return p.proc }
func (p *fakeProvider) Session() session.Identity              { return p.sess }
func (p *fakeProvider) Registry() *surfaces.Registry           { return p.reg }

type transportWorld struct {
	prov    *fakeProvider
	token   surfaces.AttachToken
	httptst *httptest.Server

	respStatus int
	respBody   []byte

	streams []*sseStream
}

// sseStream reads SSE data lines from an open /events connection on a goroutine.
type sseStream struct {
	cancel context.CancelFunc
	body   io.ReadCloser
	lines  chan string
}

// InitializeScenario registers the transport steps.
func InitializeScenario(sc *godog.ScenarioContext) {
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
		bus:  state.NewBus(),
		proc: state.NewProcessingPublisher(id.ID),
		sess: id,
		reg:  surfaces.NewRegistry(tok, id.ID, id.Name),
	}
	w.httptst = httptest.NewServer(transporthttp.NewServer(w.prov).Handler())
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
