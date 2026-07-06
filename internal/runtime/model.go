package runtime

import (
	"context"

	"agentx/internal/llm/ollama"
	"agentx/internal/prompting"
)

// Model is the LLM the prompt cycle drives. It is an interface so the
// orchestrator can be tested with a stub in place of a live Ollama.
type Model interface {
	// Chat streams a completion for the assembled messages, invoking onDelta for
	// each content chunk and onThink (when non-nil) for each reasoning chunk, and
	// returns the assembled response. A non-nil onThink also requests thinking.
	Chat(ctx context.Context, model string, messages []prompting.Message, onDelta, onThink func(string)) (string, error)
	// Ready reports whether the model is available.
	Ready(ctx context.Context, model string) error
	// ContextLength reports the model's maximum context window in tokens. It is
	// both the num_ctx the prompt cycle requests and the denominator the
	// context-visualizer surface measures against.
	ContextLength(ctx context.Context, model string) (int, error)
}

// ollamaModel adapts *ollama.Client to the Model interface.
type ollamaModel struct {
	client *ollama.Client
}

func newOllamaModel(host string) ollamaModel {
	return ollamaModel{client: ollama.New(host)}
}

func (o ollamaModel) Chat(ctx context.Context, model string, messages []prompting.Message, onDelta, onThink func(string)) (string, error) {
	om := make([]ollama.Message, len(messages))
	for i, m := range messages {
		om[i] = ollama.Message{Role: m.Role, Content: m.Content}
	}
	req := ollama.ChatRequest{Model: model, Messages: om, Think: onThink != nil}
	// Request the model's full context window (cached lookup) instead of leaving
	// Ollama on its small server default. A lookup failure falls back to 0
	// (num_ctx unset), so a chat still runs on the default window.
	if n, err := o.client.ContextLength(ctx, model); err == nil {
		req.NumCtx = n
	}
	return o.client.Chat(ctx, req, onDelta, onThink)
}

func (o ollamaModel) Ready(ctx context.Context, model string) error {
	return o.client.Ready(ctx, model)
}

func (o ollamaModel) ContextLength(ctx context.Context, model string) (int, error) {
	return o.client.ContextLength(ctx, model)
}
