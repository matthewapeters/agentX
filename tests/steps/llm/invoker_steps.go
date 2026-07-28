package llmsteps

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"

	"github.com/cucumber/godog"

	"agentx/internal/llm/fanout"
	"agentx/internal/llm/invoke"
	"agentx/internal/llm/ollama"
	"agentx/internal/llm/provider"
)

type invokerWorld struct {
	canned   string
	sent     *provider.CompleteRequest
	stub     *stubProvider
	inv      fanout.Invocation
	resp     fanout.Response
	invErr   error

	// fake-ollama (integration) bookkeeping
	server   *httptest.Server
	gotBody  map[string]any
	complete string
	compErr  error
}

// stubProvider is a test-double provider.Provider: it returns canned text from
// Complete and records the last request sent, so scenarios can assert on both
// the parsed response and the wire-shape of the request.
type stubProvider struct {
	formatStyle provider.FormatStyle
	completeFn  func(ctx context.Context, req provider.CompleteRequest) (string, error)
}

func (s *stubProvider) FormatStyle() provider.FormatStyle { return s.formatStyle }
func (s *stubProvider) Complete(ctx context.Context, req provider.CompleteRequest) (string, error) {
	if s.completeFn != nil {
		return s.completeFn(ctx, req)
	}
	return "", nil
}
func (s *stubProvider) Chat(ctx context.Context, req provider.ChatRequest, onDelta, onThink func(string)) (string, error) {
	return "", nil
}
func (s *stubProvider) Ready(ctx context.Context, model string) error { return nil }
func (s *stubProvider) ContextLength(ctx context.Context, model string) (int, error) {
	return 0, nil
}

func registerInvokerSteps(sc *godog.ScenarioContext) {
	w := &invokerWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.server != nil {
			w.server.Close()
		}
		*w = invokerWorld{}
		return ctx, nil
	})

	sc.Step(`^an invoker whose model returns '([^']*)'$`, w.invokerReturns)
	sc.Step(`^an invoker capturing its request$`, w.invokerCapturing)
	sc.Step(`^an invocation whose verdict field is "([^"]*)"$`, w.invVerdictField)
	sc.Step(`^an invocation requiring fields "([^"]*)" and "([^"]*)" at temperature ([\d.]+)$`, w.invRequiring)
	sc.Step(`^the invoker runs the invocation$`, w.invokerRuns)
	sc.Step(`^the response verdict is "([^"]*)"$`, w.respVerdict)
	sc.Step(`^the response confidence is ([\d.]+)$`, w.respConfidence)
	sc.Step(`^the response field "([^"]*)" is "([^"]*)"$`, w.respField)
	sc.Step(`^the response has no verdict$`, w.respNoVerdict)
	sc.Step(`^the sent request format requires "([^"]*)" and "([^"]*)"$`, w.sentFormatRequires)
	sc.Step(`^the sent request temperature is ([\d.]+)$`, w.sentTemperature)

	sc.Step(`^a fake Ollama returning content '([^']*)'$`, w.fakeOllama)
	sc.Step(`^a structured completion is sent at temperature ([\d.]+) with a format schema$`, w.sendCompletion)
	sc.Step(`^the completion returns '([^']*)'$`, w.completionReturns)
	sc.Step(`^the fake Ollama received a format schema and temperature ([\d.]+)$`, w.fakeReceived)
}

// ---- invoker (stubbed model) ----

func (w *invokerWorld) invokerReturns(canned string) error {
	w.canned = canned
	return nil
}

func (w *invokerWorld) invokerCapturing() error {
	w.canned = `{"relation":"new","confidence":0.5}`
	return nil
}

func (w *invokerWorld) invVerdictField(field string) error {
	w.inv.VerdictField = field
	w.inv.Prompt = "classify this"
	return nil
}

func (w *invokerWorld) invRequiring(a, b string, temp float64) error {
	w.inv.Contract = fanout.Contract{RequireFields: []string{a, b}}
	w.inv.Params = fanout.Params{Temperature: temp}
	w.inv.Prompt = "classify this"
	return nil
}

func (w *invokerWorld) invokerRuns() error {
	w.stub = &stubProvider{formatStyle: provider.FormatStyleNative}
	w.stub.completeFn = func(_ context.Context, req provider.CompleteRequest) (string, error) {
		captured := req
		w.sent = &captured
		return w.canned, nil
	}
	inv := invoke.NewProvider("test-model", "", w.stub)
	w.resp, w.invErr = inv.Invoke(context.Background(), w.inv)
	return nil
}

func (w *invokerWorld) respVerdict(want string) error {
	if w.invErr != nil {
		return fmt.Errorf("invoke errored: %v", w.invErr)
	}
	if w.resp.Verdict != want {
		return fmt.Errorf("verdict = %q, want %q", w.resp.Verdict, want)
	}
	return nil
}

func (w *invokerWorld) respConfidence(want float64) error {
	if math.Abs(w.resp.Confidence-want) > 1e-9 {
		return fmt.Errorf("confidence = %v, want %v", w.resp.Confidence, want)
	}
	return nil
}

func (w *invokerWorld) respField(key, want string) error {
	if got := w.resp.Fields[key]; got != want {
		return fmt.Errorf("field %q = %q, want %q", key, got, want)
	}
	return nil
}

func (w *invokerWorld) respNoVerdict() error {
	if w.resp.Verdict != "" {
		return fmt.Errorf("expected no verdict, got %q", w.resp.Verdict)
	}
	return nil
}

func (w *invokerWorld) sentFormatRequires(a, b string) error {
	if w.sent == nil {
		return fmt.Errorf("no request was captured")
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(w.sent.Format, &schema); err != nil {
		return fmt.Errorf("format is not a JSON schema: %v", err)
	}
	has := map[string]bool{}
	for _, r := range schema.Required {
		has[r] = true
	}
	if !has[a] || !has[b] {
		return fmt.Errorf("format requires %v, want %q and %q", schema.Required, a, b)
	}
	return nil
}

func (w *invokerWorld) sentTemperature(want float64) error {
	if w.sent == nil {
		return fmt.Errorf("no request was captured")
	}
	if math.Abs(w.sent.Temperature-want) > 1e-9 {
		return fmt.Errorf("temperature = %v, want %v", w.sent.Temperature, want)
	}
	return nil
}

// ---- fake Ollama (integration) ----

func (w *invokerWorld) fakeOllama(content string) error {
	w.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.gotBody = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&w.gotBody)
		reply := map[string]any{"message": map[string]any{"content": content}, "done": true}
		_ = json.NewEncoder(rw).Encode(reply)
	}))
	return nil
}

func (w *invokerWorld) sendCompletion(temp float64) error {
	client := ollama.NewWithHTTPClient(w.server.URL, w.server.Client())
	w.complete, w.compErr = client.Complete(context.Background(), ollama.CompleteRequest{
		Model:       "test-model",
		Messages:    []ollama.Message{{Role: "user", Content: "hi"}},
		Temperature: temp,
		Format:      json.RawMessage(`{"type":"object","required":["relation"]}`),
	})
	return nil
}

func (w *invokerWorld) completionReturns(want string) error {
	if w.compErr != nil {
		return fmt.Errorf("complete errored: %v", w.compErr)
	}
	if w.complete != want {
		return fmt.Errorf("completion = %q, want %q", w.complete, want)
	}
	return nil
}

func (w *invokerWorld) fakeReceived(temp float64) error {
	if _, ok := w.gotBody["format"]; !ok {
		return fmt.Errorf("server received no format; body keys: %v", keysOf(w.gotBody))
	}
	opts, ok := w.gotBody["options"].(map[string]any)
	if !ok {
		return fmt.Errorf("server received no options")
	}
	got, _ := opts["temperature"].(float64)
	if math.Abs(got-temp) > 1e-9 {
		return fmt.Errorf("server received temperature %v, want %v", got, temp)
	}
	if stream, _ := w.gotBody["stream"].(bool); stream {
		return fmt.Errorf("expected a non-streaming request")
	}
	return nil
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
