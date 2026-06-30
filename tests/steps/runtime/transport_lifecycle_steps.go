package runtimesteps

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/cli"
	"agentx/internal/runtime"
	"agentx/internal/state"
	"agentx/internal/surfaces"
	contextsurface "agentx/internal/surfaces/context"
	transporthttp "agentx/internal/transport/http"
)

type tlWorld struct {
	dir       string
	orc       *runtime.Orchestrator
	endpoint  string
	registry  *surfaces.Registry
	launch    cli.LaunchResult
	launchErr error
	stream    *sseReader

	ctxModel  *contextsurface.Model
	ctxCh     <-chan state.Event
	ctxCancel context.CancelFunc
}

// registerTransportLifecycleSteps wires the serve-alongside lifecycle steps (TRN-6).
func registerTransportLifecycleSteps(sc *godog.ScenarioContext) {
	w := &tlWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.stream != nil {
			w.stream.close()
		}
		if w.ctxCancel != nil {
			w.ctxCancel()
		}
		if w.orc != nil {
			shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = w.orc.Shutdown(shutCtx)
			cancel()
		}
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = tlWorld{}
		return ctx, err
	})

	sc.Step(`^a running orchestrator serving the transport$`, w.serving)
	sc.Step(`^a running orchestrator with the transport disabled$`, w.servingDisabled)
	sc.Step(`^the transport health endpoint responds ok$`, w.healthOK)
	sc.Step(`^I attach a "([^"]*)" surface with the launch CLI$`, w.attach)
	sc.Step(`^the attach succeeds$`, w.attachSucceeds)
	sc.Step(`^a prompt is submitted over the transport$`, w.submitPrompt)
	sc.Step(`^the response streams back over the event stream$`, w.responseStreams)
	sc.Step(`^the serving orchestrator is shut down$`, w.shutdown)
	sc.Step(`^the transport endpoint is unreachable$`, w.endpointUnreachable)
	sc.Step(`^the attached surface is stopped$`, w.surfaceStopped)
	sc.Step(`^the orchestrator publishes no transport endpoint$`, w.noEndpoint)

	// Context surface attach over the real transport (SS-3).
	sc.Step(`^a recorded exchange prompt "([^"]*)" response "([^"]*)"$`, w.recordExchange)
	sc.Step(`^a context surface attaches over the transport$`, w.ctxAttach)
	sc.Step(`^the attached context view contains "([^"]*)"$`, w.ctxContains)
	sc.Step(`^a live response "([^"]*)" is recorded$`, w.recordResponse)
	sc.Step(`^the attached context view renders "([^"]*)"$`, w.ctxRenders)
}

func (w *tlWorld) publishEvent(eventType string, ct state.ContentType, text string) {
	w.orc.Bus().Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   w.orc.Session().ID,
		EventType:   eventType,
		ContentType: ct,
		Payload:     map[string]any{"text": text},
		Enabled:     state.DefaultEnabled(ct),
	})
}

func (w *tlWorld) recordExchange(prompt, response string) error {
	w.publishEvent("USER_PROMPT", state.ContentUserPrompt, prompt)
	w.publishEvent("AGENT_RESPONSE", state.ContentAgentResponse, response)
	// Wait for the recorder to persist both before the surface seeds from disk.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if h, err := w.orc.History(); err == nil && len(h) >= 2 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("exchange not persisted in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (w *tlWorld) recordResponse(text string) error {
	w.publishEvent("AGENT_RESPONSE", state.ContentAgentResponse, text)
	return nil
}

func (w *tlWorld) ctxAttach() error {
	c := transporthttp.NewClient(w.orc.Endpoint())
	seed, err := c.Seed(context.Background())
	if err != nil {
		return err
	}
	w.ctxModel = contextsurface.New()
	w.ctxModel.SetSize(80, 24)
	var last uint64
	for _, ev := range seed {
		w.ctxModel.Apply(ev)
		last = ev.Ordinal
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.ctxCancel = cancel
	ch, err := c.Subscribe(ctx, last, "")
	if err != nil {
		cancel()
		return err
	}
	w.ctxCh = ch
	return nil
}

func (w *tlWorld) ctxContains(want string) error {
	if !strings.Contains(w.ctxModel.View(), want) {
		return fmt.Errorf("context view does not contain %q", want)
	}
	return nil
}

// ctxRenders applies live events until the view shows want or the deadline passes.
func (w *tlWorld) ctxRenders(want string) error {
	deadline := time.After(3 * time.Second)
	for {
		if strings.Contains(w.ctxModel.View(), want) {
			return nil
		}
		select {
		case ev, ok := <-w.ctxCh:
			if !ok {
				return fmt.Errorf("stream closed before %q rendered", want)
			}
			w.ctxModel.Apply(ev)
		case <-deadline:
			return fmt.Errorf("timed out waiting for %q to render", want)
		}
	}
}

// freePort discovers a currently-free loopback port.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	return port, ln.Close()
}

func (w *tlWorld) start(enabled bool) error {
	dir, err := os.MkdirTemp("", "agentx-tl-*")
	if err != nil {
		return err
	}
	w.dir = dir
	s := runtime.Settings{SessionRoot: dir, OllamaModel: "stub", TransportEnabled: enabled}
	if enabled {
		port, err := freePort()
		if err != nil {
			return err
		}
		s.TransportHost = "127.0.0.1"
		s.TransportPortStart = port
		s.TransportPortEnd = port + 30
	}
	w.orc = runtime.New(s, runtime.WithModel(stubModel{deltas: []string{"pong"}}))
	return w.orc.Start()
}

func (w *tlWorld) serving() error         { return w.start(true) }
func (w *tlWorld) servingDisabled() error { return w.start(false) }

func (w *tlWorld) healthOK() error {
	resp, err := http.Get(w.orc.Endpoint() + "/health")
	if err != nil {
		return fmt.Errorf("health request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health status = %d, want 200", resp.StatusCode)
	}
	return nil
}

func (w *tlWorld) attach(kind string) error {
	w.launch, w.launchErr = cli.Launch(context.Background(), cli.LaunchArgs{
		SurfaceKind: kind,
		Session:     w.orc.Session().Name,
		Connect:     w.orc.Endpoint(),
		Token:       w.orc.AttachToken().Raw(),
	})
	return nil
}

func (w *tlWorld) attachSucceeds() error {
	if w.launchErr != nil {
		return fmt.Errorf("attach failed: %v", w.launchErr)
	}
	return nil
}

func (w *tlWorld) submitPrompt() error {
	// Open the event stream before submitting so no event is missed.
	st, err := openSSE(w.orc.Endpoint() + "/events")
	if err != nil {
		return err
	}
	w.stream = st

	req, err := http.NewRequest(http.MethodPost, w.orc.Endpoint()+"/prompt", strings.NewReader(`{"text":"ping"}`))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+w.orc.AttachToken().Raw())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("prompt status = %d, want 202", resp.StatusCode)
	}
	return nil
}

func (w *tlWorld) responseStreams() error {
	return w.stream.waitFor("pong", 5*time.Second)
}

func (w *tlWorld) shutdown() error {
	// Capture endpoint + registry before shutdown so post-shutdown assertions work.
	w.endpoint = w.orc.Endpoint()
	w.registry = w.orc.Registry()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := w.orc.Shutdown(ctx)
	w.orc = nil // shut down; skip the After-hook re-shutdown
	return err
}

func (w *tlWorld) endpointUnreachable() error {
	client := http.Client{Timeout: time.Second}
	resp, err := client.Get(w.endpoint + "/health")
	if err != nil {
		return nil // connection refused as expected
	}
	resp.Body.Close()
	return fmt.Errorf("endpoint still reachable, status %d", resp.StatusCode)
}

func (w *tlWorld) surfaceStopped() error {
	reg, ok := w.registry.Get(w.launch.SurfaceID)
	if !ok {
		return fmt.Errorf("surface %q not found", w.launch.SurfaceID)
	}
	if reg.LifecycleState != "stopped" {
		return fmt.Errorf("surface lifecycle = %q, want stopped", reg.LifecycleState)
	}
	return nil
}

func (w *tlWorld) noEndpoint() error {
	if ep := w.orc.Endpoint(); ep != "" {
		return fmt.Errorf("expected no endpoint, got %q", ep)
	}
	return nil
}

// sseReader reads SSE data lines from an /events connection on a goroutine.
type sseReader struct {
	cancel context.CancelFunc
	lines  chan string
}

func openSSE(url string) (*sseReader, error) {
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	r := &sseReader{cancel: cancel, lines: make(chan string, 64)}
	go func() {
		defer close(r.lines)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if data, ok := strings.CutPrefix(scanner.Text(), "data:"); ok {
				select {
				case r.lines <- strings.TrimSpace(data):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return r, nil
}

func (r *sseReader) waitFor(want string, timeout time.Duration) error {
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-r.lines:
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

func (r *sseReader) close() { r.cancel() }
