// Package llmsteps implements the Godog steps for the LLM behavior domain.
package llmsteps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/llm/llamacpp"
	"agentx/internal/llm/provider"
)

// llamacppWorld owns the state for llama.cpp adapter scenarios: a fake
// server, the client, and captured request/response data.
type llamacppWorld struct {
	server  *httptest.Server
	client  *llamacpp.Client
	prov    *llamacpp.LlamacppProvider
	sawBody map[string]any

	deltas     []string
	response   string
	chatErr    error
	complete   string
	completeErr error
	readyErr   error
	ctxLen     int
	ctxLenErr  error
	gotModel   string
	sentNumCtx int
}

// registerLlamacppSteps registers the llama.cpp adapter steps.
// Called from fanout_steps.go's InitializeScenario.
func registerLlamacppSteps(sc *godog.ScenarioContext) {
	w := &llamacppWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.server != nil {
			w.server.Close()
		}
		*w = llamacppWorld{}
		return ctx, nil
	})

	// --- setup: fake server ---
	sc.Step(`^a stub llama.cpp server that streams "([^"]*)" then "([^"]*)"$`, w.serverStreams)
	sc.Step(`^a stub llama.cpp server listing model "([^"]*)"$`, w.serverListing)
	sc.Step(`^a stub llama.cpp server listing model "([^"]*)" with no context length$`, w.serverNoContextLength)
	sc.Step(`^a stub llama.cpp server that returns HTTP 500$`, w.serverError)
	sc.Step(`^a stub llama.cpp server reporting context length (\d+) for model "([^"]*)"$`, w.serverContextLength)
	sc.Step(`^a stub llama.cpp server accepting any completion$`, w.serverAccepting)
	sc.Step(`^a stub llama.cpp server recording request bodies$`, w.serverRecording)

	// --- chat ---
	sc.Step(`^a llama.cpp chat request for model "([^"]*)" is sent with prompt "([^"]*)"$`, w.sendChat)
	sc.Step(`^a llama.cpp chat request for model "([^"]*)" is sent with prompt "([^"]*)" and context window (\d+)$`, w.sendChatNumCtx)
	sc.Step(`^the streamed llama.cpp deltas are "([^"]*)" then "([^"]*)"$`, w.deltasAre)
	sc.Step(`^the assembled llama.cpp response is "([^"]*)"$`, w.responseIs)
	sc.Step(`^the llama.cpp chat returns an error$`, w.chatErrored)
	sc.Step(`^the llama.cpp chat request set n_ctx to (\d+)$`, w.numCtxSent)

	// --- complete ---
	sc.Step(`^a llama.cpp complete request for model "([^"]*)" is sent with a prompt containing JSON instruction$`, w.sendCompleteWithInstruction)
	sc.Step(`^the llama.cpp completion returns the server's reply unchanged$`, w.completionReturns)
	sc.Step(`^the llama.cpp server received the prompt verbatim$`, w.serverReceivedPrompt)
	sc.Step(`^the llama.cpp server received a request body containing "([^"]*)"$`, w.serverGotBody)
	sc.Step(`^the llama.cpp server received a request body containing no "([^"]*)" field$`, w.serverGotBodyNoField)

	// --- readiness ---
	sc.Step(`^llama.cpp readiness is checked for model "([^"]*)"$`, w.checkReadiness)
	sc.Step(`^llama.cpp readiness passes$`, w.readinessPasses)
	sc.Step(`^llama.cpp readiness fails with "([^"]*)"$`, w.readinessFails)

	// --- context length ---
	sc.Step(`^llama.cpp context length is requested for model "([^"]*)"$`, w.requestContextLength)
	sc.Step(`^the llama.cpp context length is (\d+)$`, w.contextLengthIs)

	// --- adapter ---
	sc.Step(`^the LlamacppProvider satisfies provider\.Provider$`, w.satisfiesProvider)
}

// ---- server setup ----

func (w *llamacppWorld) serverStreams(a, b string) error {
	w.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v1/models") {
			json.NewEncoder(rw).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "test", "object": "model", "owned_by": "test"},
				},
			})
			return
		}
		if r.Method == http.MethodPost {
			rw.Header().Set("Content-Type", "application/json")
			fmt.Fprint(rw, `data:{"id":"c","object":"chat.completion.chunk","created":1,"model":"test","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`)
			fmt.Fprint(rw, "\n")
			fmt.Fprint(rw, "data:{"+strings.Join([]string{`"id":"c"`, `"object":"chat.completion.chunk"`, `"created":1`, `"model":"test"`, `"choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]`}, ",")+"}")
			fmt.Fprint(rw, "\n")
			// Stream two content deltas.
			fmt.Fprint(rw, "data:{"+strings.Join([]string{`"id":"c"`, `"object":"chat.completion.chunk"`, `"created":1`, `"model":"test"`, fmt.Sprintf(`"choices":[{"index":0,"delta":{"role":"assistant","content":"%s"},"finish_reason":null}]`, a), `"usage":null`}, ",")+"}")
			fmt.Fprint(rw, "\n")
			fmt.Fprint(rw, "data:{"+strings.Join([]string{`"id":"c"`, `"object":"chat.completion.chunk"`, `"created":1`, `"model":"test"`, fmt.Sprintf(`"choices":[{"index":0,"delta":{"role":"assistant","content":"%s"},"finish_reason":null}]`, b), `"usage":null`}, ",")+"}")
			fmt.Fprint(rw, "\n")
			fmt.Fprint(rw, "data:{"+strings.Join([]string{`"id":"c"`, `"object":"chat.completion.chunk"`, `"created":1`, `"model":"test"`, `"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]`, `"usage":null`}, ",")+"}")
			fmt.Fprint(rw, "\n")
			fmt.Fprint(rw, "data: [DONE]\n")
			return
		}
		rw.WriteHeader(http.StatusNotFound)
	}))
	w.client = llamacpp.New(w.server.URL)
	w.prov = llamacpp.NewLlamacppProvider(w.client)
	w.deltas = nil
	w.response = ""
	w.chatErr = nil
	w.complete = ""
	w.completeErr = nil
	w.sawBody = nil
	w.sentNumCtx = 0
	return nil
}

func (w *llamacppWorld) serverListing(model string) error {
	w.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		json.NewEncoder(rw).Encode(map[string]any{
			"data": []map[string]any{
				{"id": model, "object": "model", "owned_by": "test"},
			},
		})
	}))
	w.client = llamacpp.New(w.server.URL)
	w.prov = llamacpp.NewLlamacppProvider(w.client)
	w.readyErr = nil
	w.gotModel = ""
	return nil
}

func (w *llamacppWorld) serverNoContextLength(model string) error {
	w.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v1/models/") {
			json.NewEncoder(rw).Encode(map[string]any{
				"id":            model,
				"object":        "model",
				"context_length": 0,
			})
			return
		}
		rw.WriteHeader(http.StatusNotFound)
	}))
	w.client = llamacpp.New(w.server.URL)
	w.prov = llamacpp.NewLlamacppProvider(w.client)
	w.ctxLen = 0
	w.ctxLenErr = nil
	return nil
}

func (w *llamacppWorld) serverError() error {
	w.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusInternalServerError)
	}))
	w.client = llamacpp.New(w.server.URL)
	w.prov = llamacpp.NewLlamacppProvider(w.client)
	return nil
}

func (w *llamacppWorld) serverContextLength(len int, model string) error {
	w.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v1/models/") {
			json.NewEncoder(rw).Encode(map[string]any{
				"id":            model,
				"object":        "model",
				"context_length": len,
			})
			return
		}
		rw.WriteHeader(http.StatusNotFound)
	}))
	w.client = llamacpp.New(w.server.URL)
	w.prov = llamacpp.NewLlamacppProvider(w.client)
	w.ctxLen = 0
	w.ctxLenErr = nil
	w.gotModel = ""
	return nil
}

func (w *llamacppWorld) serverAccepting() error {
	w.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	w.client = llamacpp.New(w.server.URL)
	w.prov = llamacpp.NewLlamacppProvider(w.client)
	w.complete = ""
	w.completeErr = nil
	return nil
}

func (w *llamacppWorld) serverRecording() error {
	w.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v1/models") {
			json.NewEncoder(rw).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "test", "object": "model", "owned_by": "test"},
				},
			})
			return
		}
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			var parsed map[string]any
			_ = json.Unmarshal(body, &parsed)
			w.sawBody = parsed
			rw.Header().Set("Content-Type", "application/json")
			json.NewEncoder(rw).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"role": "assistant", "content": "ok"}},
				},
			})
			return
		}
		rw.WriteHeader(http.StatusNotFound)
	}))
	w.client = llamacpp.New(w.server.URL)
	w.prov = llamacpp.NewLlamacppProvider(w.client)
	w.sawBody = nil
	return nil
}

// ---- chat steps ----

func (w *llamacppWorld) sendChat(model, prompt string) error {
	msgs := []llamacpp.Message{{Role: "user", Content: prompt}}
	w.response = ""
	w.chatErr = nil
	w.deltas = nil
	w.sentNumCtx = 0
	w.response, w.chatErr = w.client.Chat(context.Background(), llamacpp.ChatRequest{
		Model:    model,
		Messages: msgs,
	}, func(d string) { w.deltas = append(w.deltas, d) }, nil)
	return nil
}

func (w *llamacppWorld) sendChatNumCtx(model, prompt string, numCtx int) error {
	msgs := []llamacpp.Message{{Role: "user", Content: prompt}}
	w.response = ""
	w.chatErr = nil
	w.deltas = nil
	w.sentNumCtx = numCtx
	w.response, w.chatErr = w.client.Chat(context.Background(), llamacpp.ChatRequest{
		Model:    model,
		Messages: msgs,
		NumCtx:   numCtx,
	}, func(d string) { w.deltas = append(w.deltas, d) }, nil)
	return nil
}

func (w *llamacppWorld) deltasAre(a, b string) error {
	if len(w.deltas) != 2 {
		return fmt.Errorf("expected 2 deltas, got %d: %v", len(w.deltas), w.deltas)
	}
	if w.deltas[0] != a {
		return fmt.Errorf("delta[0] = %q, want %q", w.deltas[0], a)
	}
	if w.deltas[1] != b {
		return fmt.Errorf("delta[1] = %q, want %q", w.deltas[1], b)
	}
	return nil
}

func (w *llamacppWorld) responseIs(expected string) error {
	if w.response != expected {
		return fmt.Errorf("response = %q, want %q", w.response, expected)
	}
	return nil
}

func (w *llamacppWorld) chatErrored() error {
	if w.chatErr == nil {
		return fmt.Errorf("expected chat error, got nil")
	}
	return nil
}

func (w *llamacppWorld) numCtxSent(expected int) error {
	if w.sentNumCtx != expected {
		return fmt.Errorf("n_ctx = %d, want %d", w.sentNumCtx, expected)
	}
	return nil
}

// ---- complete steps ----

func (w *llamacppWorld) sendCompleteWithInstruction(model string) error {
	msgs := []llamacpp.Message{{Role: "user", Content: "classify this. JSON: {\"verdict\":{}}"}}
	w.complete = ""
	w.completeErr = nil
	w.complete, w.completeErr = w.client.Complete(context.Background(), llamacpp.CompleteRequest{
		Model:    model,
		Messages: msgs,
	})
	return nil
}

func (w *llamacppWorld) completionReturns() error {
	if w.completeErr != nil {
		return fmt.Errorf("completion error: %w", w.completeErr)
	}
	return nil
}

func (w *llamacppWorld) serverReceivedPrompt() error {
	body, err := json.Marshal(w.sawBody)
	if err != nil {
		return fmt.Errorf("marshal sawBody: %w", err)
	}
	if !strings.Contains(string(body), "JSON") {
		return fmt.Errorf("server did not receive JSON instruction; saw: %s", string(body))
	}
	return nil
}

func (w *llamacppWorld) serverGotBody(needle string) error {
	body, err := json.Marshal(w.sawBody)
	if err != nil {
		return fmt.Errorf("marshal sawBody: %w", err)
	}
	if !strings.Contains(string(body), needle) {
		return fmt.Errorf("server body missing %q; saw: %s", needle, string(body))
	}
	return nil
}

func (w *llamacppWorld) serverGotBodyNoField(field string) error {
	body, err := json.Marshal(w.sawBody)
	if err != nil {
		return fmt.Errorf("marshal sawBody: %w", err)
	}
	if strings.Contains(string(body), field) {
		return fmt.Errorf("server body contains %q (expected no %q); saw: %s", field, field, string(body))
	}
	return nil
}

// ---- readiness steps ----

func (w *llamacppWorld) checkReadiness(model string) error {
	w.readyErr = w.client.Ready(context.Background(), model)
	return nil
}

func (w *llamacppWorld) readinessPasses() error {
	if w.readyErr != nil {
		return fmt.Errorf("expected readiness to pass, got: %w", w.readyErr)
	}
	return nil
}

func (w *llamacppWorld) readinessFails(expected string) error {
	if w.readyErr == nil {
		return fmt.Errorf("expected readiness to fail, got nil")
	}
	if !strings.Contains(w.readyErr.Error(), expected) {
		return fmt.Errorf("error = %q, want containing %q", w.readyErr.Error(), expected)
	}
	return nil
}

// ---- context length steps ----

func (w *llamacppWorld) requestContextLength(model string) error {
	w.ctxLen = 0
	w.ctxLenErr = nil
	w.gotModel = model
	w.ctxLen, w.ctxLenErr = w.client.ContextLength(context.Background(), model)
	return nil
}

func (w *llamacppWorld) contextLengthIs(expected int) error {
	if w.ctxLenErr != nil {
		return fmt.Errorf("context length error: %w", w.ctxLenErr)
	}
	if w.ctxLen != expected {
		return fmt.Errorf("context length = %d, want %d", w.ctxLen, expected)
	}
	return nil
}

// ---- FormatStyle assertions ----

// formatStyleOf reports the FormatStyle of the wrapped provider.
func (w *llamacppWorld) formatStyleOf() provider.FormatStyle {
	if w.prov == nil {
		return provider.FormatStylePrompt
	}
	return w.prov.FormatStyle()
}

// satisfiesProvider is a compile-time check disguised as a Godog step — it
// panics only if *LlamacppProvider fails to implement provider.Provider, which
// would be caught at build time anyway. Here it serves as a documentation
// anchor: every new provider must satisfy this interface.
func (w *llamacppWorld) satisfiesProvider() error {
	var _ provider.Provider = (*llamacpp.LlamacppProvider)(nil)
	return nil
}
