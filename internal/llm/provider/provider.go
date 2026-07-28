// Package provider defines the backend-agnostic LLM adapter seam. Both
// Ollama and llama.cpp (and any future provider) implement Provider; the
// fan-out pool, the invoker, and the runtime Model adapter never see the
// concrete backend.
//
// The only behavioral divergence between backends at the invocation layer is
// how JSON-schema constrained decoding is honored. That is controlled by
// FormatStyle, reported by Provider.FormatStyle():
//
//   - FormatStyleNative: the provider accepts a "format" field in the payload
//     (Ollama). The invoker passes the schema through unchanged.
//   - FormatStylePrompt: the provider does NOT accept "format". The invoker
//     injects a JSON instruction into the user prompt instead.
//
// Source contract: docs/architecture/adr/0013-llm-provider-abstraction.md.
// Behavior contract: tests/features/llm/provider.feature.
package provider

import (
	"context"
	"encoding/json"
)

// Message is a single chat message (role + content). Each provider package
// defines its own Message type matching its backend's wire format; this one is
// shared by the runtime adapter and test stubs.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// FormatStyle controls how the invoker asks the model to constrain output.
// See ADR 0013 for the full rationale.
type FormatStyle int

const (
	// FormatStyleNone means no constraint; Format must be nil on the request.
	FormatStyleNone FormatStyle = iota
	// FormatStyleNative means the provider honors "format" natively in the
	// request payload (Ollama). The invoker passes Format through unchanged.
	FormatStyleNative
	// FormatStylePrompt means the provider does NOT honor "format". The invoker
	// injects a JSON instruction into the user prompt so the model still emits
	// structured output — but via prompt engineering rather than a server-side
	// constrained-decoding hook.
	FormatStylePrompt
)

// String returns a human-readable name for the FormatStyle. Used in test
// assertions and error messages.
func (s FormatStyle) String() string {
	switch s {
	case FormatStyleNative:
		return "native"
	case FormatStylePrompt:
		return "prompt"
	default:
		return "none"
	}
}

// ParseFormatStyle converts a lowercase string to its FormatStyle value. Used
// by test steps and config parsing. Unknown values return FormatStyleNone.
func ParseFormatStyle(s string) FormatStyle {
	switch s {
	case "native":
		return FormatStyleNative
	case "prompt":
		return FormatStylePrompt
	default:
		return FormatStyleNone
	}
}

// CompleteRequest is a non-streaming chat completion. Format carries a JSON
// schema when the provider supports native constrained decoding; nil when the
// invoker injects the constraint into the prompt instead (FormatStylePrompt).
// NumCtx, when > 0, sets the context window. Think requests reasoning.
type CompleteRequest struct {
	Model       string
	Messages    []Message
	Temperature float64
	Seed        int
	Format      json.RawMessage // nil = unconstrained
	NumCtx      int
	Think       bool
}

// ChatRequest is a streaming chat completion.
type ChatRequest struct {
	Model    string
	Messages []Message
	Think    bool
	NumCtx   int
}

// Provider is the LLM backend seam. Every provider speaks chat-completion HTTP;
// the shape of each request and the meaning of each field are provider-specific,
// but the surface contract is uniform across implementations.
//
// Implementations must be safe for concurrent use (the fan-out pool dispatches
// many completions against a single provider). Each provider is responsible for
// its own connection pooling, timeout handling, and error wrapping.
type Provider interface {
	// FormatStyle reports how this provider handles JSON-schema constrained
	// decoding. The invoker reads this once at construction and adjusts its
	// request-building accordingly.
	FormatStyle() FormatStyle

	// Complete runs a single non-streaming chat completion and returns the
	// assembled message content. It honors ctx cancellation.
	Complete(ctx context.Context, req CompleteRequest) (string, error)

	// Chat streams a chat completion, invoking onDelta for each content chunk
	// and onThink (when non-nil and Think is set) for each reasoning chunk,
	// and returns the assembled response. It honors ctx cancellation.
	Chat(ctx context.Context, req ChatRequest, onDelta, onThink func(string)) (string, error)

	// Ready reports whether the host is reachable and the model is available.
	// It is called at startup to verify the configured model before taking
	// over the terminal.
	Ready(ctx context.Context, model string) error

	// ContextLength reports the model's maximum context window in tokens. The
	// runtime uses this as the num_ctx the prompt cycle requests and as the
	// denominator for the context-visualizer surface. Implementations cache
	// the value — it is a fixed property of the model file.
	ContextLength(ctx context.Context, model string) (int, error)
}
