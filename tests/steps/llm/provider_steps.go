// Package llmsteps implements the Godog steps for the LLM behavior domain.
// Currently: the parallel model-invocation pool, the Ollama-backed invoker,
// the provider abstraction, and the llama.cpp adapter.
package llmsteps

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/llm/fanout"
	"agentx/internal/llm/invoke"
	"agentx/internal/llm/provider"
)

// providerWorld owns the state for provider-abstraction scenarios: a stub
// provider (with configurable FormatStyle), the invoker built on it, the
// captured Complete request, and the runtime model adapter.
type providerWorld struct {
	stub     *stubProvider
	invoker  *invoke.Invoker
	invErr   error
	resp     fanout.Response
	sent     *provider.CompleteRequest

	// Invocation under test (populated by "an invocation requiring..." steps).
	invocation fanout.Invocation

	// Chat-specific capture.
	chatReq  provider.ChatRequest
	chatDeltas []string
	chatErr  error

	// Runtime model adapter capture.
	adapterReq   *provider.CompleteRequest
	adapterChat  provider.ChatRequest
	adapterErr   error
}

// registerProviderSteps registers the provider-abstraction steps.
// Called from fanout_steps.go's InitializeScenario.
func registerProviderSteps(sc *godog.ScenarioContext) {
	w := &providerWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		*w = providerWorld{}
		return ctx, nil
	})

	// --- setup ---
	sc.Step(`^an invoker backed by a stub provider with FormatStyle "([^"]*)"$`, w.invokerWithStyle)
	sc.Step(`^an invocation requiring fields "([^"]*)" and "([^"]*)"$`, w.invRequiring)
	sc.Step(`^an invocation with no output contract$`, w.invUnconstrained)
	sc.Step(`^a streaming chat request for model "([^"]*)" with messages$`, w.chatRequest)
	sc.Step(`^a non-streaming completion request for model "([^"]*)"$`, w.completeRequest)
	sc.Step(`^the invoker runs the provider invocation$`, w.invokerRuns)
	sc.Step(`^the invoker dispatches chat$`, w.invokerDispatchesChat)
	sc.Step(`^a runtime model adapter wrapping a stub provider with FormatStyle "([^"]*)"$`, w.adapterWithStyle)
	sc.Step(`^the adapter runs the completion$`, w.adapterRunsCompletion)
	sc.Step(`^the adapter runs the chat$`, w.adapterRunsChat)

	// --- assertions: format ---
	sc.Step(`^the provider received a format schema$`, w.gotFormat)
	sc.Step(`^the provider received no format schema$`, w.gotNoFormat)
	sc.Step(`^the provider received a prompt containing a JSON instruction$`, w.gotJSONInstruction)
	sc.Step(`^the provider received a prompt with no JSON instruction$`, w.gotNoJSONInstruction)
	sc.Step(`^the provider received the messages unchanged$`, w.chatMsgsUnchanged)
	sc.Step(`^the adapter forwards the request to the provider unchanged$`, w.adapterForwarded)
}

// ---- setup ----

func (w *providerWorld) invokerWithStyle(style string) error {
	sp := &stubProvider{formatStyle: provider.ParseFormatStyle(style)}
	sp.completeFn = func(_ context.Context, req provider.CompleteRequest) (string, error) {
		w.sent = &req
		return `{"verdict":"ok"}`, nil
	}
	w.stub = sp
	w.invoker = invoke.NewProvider("test-model", "", sp)
	w.invErr = nil
	w.sent = nil
	w.resp = fanout.Response{}
	return nil
}

func (w *providerWorld) invRequiring(a, b string) error {
	w.invoker = invoke.NewProvider("test-model", "", w.stub)
	w.invErr = nil
	w.sent = nil
	w.resp = fanout.Response{}
	w.invocation = fanout.Invocation{
		Tag:        "test",
		Prompt:     "classify this",
		VerdictField: "verdict",
		Contract:   fanout.Contract{RequireFields: []string{a, b}},
		Params:     fanout.Params{Temperature: 0.0},
	}
	return nil
}

func (w *providerWorld) invUnconstrained() error {
	w.invoker = invoke.NewProvider("test-model", "", w.stub)
	w.invErr = nil
	w.sent = nil
	w.resp = fanout.Response{}
	w.invocation = fanout.Invocation{
		Tag:        "test",
		Prompt:     "classify this",
		VerdictField: "verdict",
		Contract:   fanout.Contract{}, // no required fields
		Params:     fanout.Params{Temperature: 0.0},
	}
	return nil
}

func (w *providerWorld) chatRequest(model string) error {
	w.chatReq = provider.ChatRequest{Model: model, Messages: []provider.Message{
		{Role: "user", Content: "hi"},
	}}
	w.chatDeltas = nil
	w.chatErr = nil
	return nil
}

func (w *providerWorld) completeRequest(model string) error {
	return nil
}

func (w *providerWorld) invokerRuns() error {
	if w.invoker == nil {
		return fmt.Errorf("no invoker configured")
	}
	w.resp, w.invErr = w.invoker.Invoke(context.Background(), w.invocation)
	return nil
}

func (w *providerWorld) invokerDispatchesChat() error {
	w.chatDeltas = nil
	w.chatErr = nil
	_, w.chatErr = w.stub.Chat(context.Background(), w.chatReq,
		func(d string) { w.chatDeltas = append(w.chatDeltas, d) },
		nil,
	)
	return nil
}

func (w *providerWorld) adapterWithStyle(style string) error {
	w.stub = &stubProvider{formatStyle: provider.ParseFormatStyle(style)}
	w.stub.completeFn = func(_ context.Context, req provider.CompleteRequest) (string, error) {
		w.adapterReq = &req
		return "ok", nil
	}
	w.adapterReq = nil
	w.adapterErr = nil
	return nil
}

func (w *providerWorld) adapterRunsCompletion() error {
	w.adapterReq = nil
	w.adapterErr = nil
	_, w.adapterErr = w.stub.Complete(context.Background(), provider.CompleteRequest{
		Model:    "m",
		Messages: []provider.Message{{Role: "user", Content: "hi"}},
	})
	return nil
}

func (w *providerWorld) adapterRunsChat() error {
	w.chatDeltas = nil
	w.chatErr = nil
	req := provider.ChatRequest{
		Model:    "m",
		Messages: []provider.Message{{Role: "user", Content: "hi"}},
	}
	w.adapterChat = req
	_, w.chatErr = w.stub.Chat(context.Background(), req,
		func(string) {}, nil)
	return nil
}

// ---- assertions ----

func (w *providerWorld) gotFormat() error {
	if w.sent == nil {
		return fmt.Errorf("no request was captured")
	}
	if w.sent.Format == nil {
		return fmt.Errorf("expected format schema, got nil")
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(w.sent.Format, &schema); err != nil {
		return fmt.Errorf("format is not a JSON schema: %v", err)
	}
	if len(schema.Required) == 0 {
		return fmt.Errorf("format schema has no required fields")
	}
	return nil
}

func (w *providerWorld) gotNoFormat() error {
	if w.sent == nil {
		return fmt.Errorf("no request was captured")
	}
	if w.sent.Format != nil {
		return fmt.Errorf("expected no format schema, got %s", w.sent.Format)
	}
	return nil
}

func (w *providerWorld) gotJSONInstruction() error {
	if w.sent == nil {
		return fmt.Errorf("no request was captured")
	}
	for _, m := range w.sent.Messages {
		if m.Role == "user" && strings.Contains(m.Content, "JSON") {
			return nil
		}
	}
	return fmt.Errorf("no user message contains JSON instruction; messages: %v", w.sent.Messages)
}

func (w *providerWorld) gotNoJSONInstruction() error {
	if w.sent == nil {
		return fmt.Errorf("no request was captured")
	}
	for _, m := range w.sent.Messages {
		if m.Role == "user" && strings.Contains(m.Content, "JSON") {
			return fmt.Errorf("user message unexpectedly contains JSON instruction: %q", m.Content)
		}
	}
	return nil
}

func (w *providerWorld) chatMsgsUnchanged() error {
	// Determine which request to check: chatReq (from invoker dispatch) or
	// adapterChat (from adapter run). The feature scenario dictates which.
	req := w.chatReq
	if req.Model == "" {
		req = w.adapterChat
	}
	if req.Model != "m" {
		return fmt.Errorf("model = %q, want %q", req.Model, "m")
	}
	for _, m := range req.Messages {
		if m.Role != "user" || m.Content != "hi" {
			return fmt.Errorf("messages not unchanged: %v", req.Messages)
		}
	}
	return nil
}

func (w *providerWorld) adapterForwarded() error {
	if w.adapterReq == nil {
		return fmt.Errorf("no request was forwarded to the provider")
	}
	if w.adapterReq.Format != nil {
		return fmt.Errorf("adapter forwarded format schema, expected nil")
	}
	return nil
}
