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
	return o.client.Chat(ctx, req, onDelta, onThink)
}

func (o ollamaModel) Ready(ctx context.Context, model string) error {
	return o.client.Ready(ctx, model)
}
