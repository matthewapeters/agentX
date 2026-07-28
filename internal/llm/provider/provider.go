// Package provider defines the backend-agnostic provider interface that
// every LLM backend (Ollama, llama.cpp, ...) must implement so the invoker
// can stay provider-agnostic.
package provider

import (
	"context"
	"encoding/json"
)

// FormatStyle indicates how the provider handles JSON-schema constrained
// decoding. Native providers (e.g. Ollama) honor a "format" field in the
// request; Prompt-style providers (e.g. llama.cpp) require the invoker to
// inject a JSON instruction into the user prompt instead.
type FormatStyle int

const (
	// FormatStyleNative means the provider natively honors a "format" field.
	FormatStyleNative FormatStyle = iota
	// FormatStylePrompt means the invoker must inject a JSON instruction into
	// the user prompt for constrained decoding.
	FormatStylePrompt
)

// ParseFormatStyle converts a string to a FormatStyle.
func ParseFormatStyle(s string) FormatStyle {
	switch s {
	case "native":
		return FormatStyleNative
	case "prompt":
		return FormatStylePrompt
	default:
		return FormatStylePrompt
	}
}

// Message is a single chat message (role + content).
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompleteRequest is a non-streaming completion request.
type CompleteRequest struct {
	Model       string
	Messages    []Message
	Temperature float64
	Seed        int
	NumCtx      int
	Think       bool
	Format      json.RawMessage
}

// ChatRequest is a streaming chat request.
type ChatRequest struct {
	Model    string
	Messages []Message
	Think    bool
	NumCtx   int
}

// Provider is the backend-agnostic interface every LLM backend must implement.
// The invoker uses this interface to dispatch completions and chat requests
// without knowing which backend is in use.
type Provider interface {
	// FormatStyle reports how the provider handles JSON-schema constrained decoding.
	FormatStyle() FormatStyle
	// Complete runs a non-streaming completion and returns the assembled response.
	Complete(ctx context.Context, req CompleteRequest) (string, error)
	// Chat streams a chat completion, invoking onDelta for each content chunk
	// and onThink for each reasoning chunk, and returns the assembled response.
	Chat(ctx context.Context, req ChatRequest, onDelta, onThink func(string)) (string, error)
	// Ready reports whether the model is available.
	Ready(ctx context.Context, model string) error
	// ContextLength reports the model's maximum context window in tokens.
	ContextLength(ctx context.Context, model string) (int, error)
}