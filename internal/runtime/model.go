package runtime

import (
	"context"

	"agentx/internal/llm/llamacpp"
	"agentx/internal/llm/ollama"
	"agentx/internal/llm/provider"
	"agentx/internal/prompting"
	"agentx/internal/tools"
)

// ChatResult is a completed chat turn: the assembled text content and any
// native tool calls the model issued.
type ChatResult struct {
	Content   string
	ToolCalls []prompting.ToolCall
}

// Model is the LLM the prompt cycle drives. It is an interface so the
// orchestrator can be tested with a stub in place of a live Ollama.
type Model interface {
	// Chat streams a completion for the assembled messages, invoking onDelta for
	// each content chunk and onThink (when non-nil) for each reasoning chunk, and
	// returns the assembled response plus any native tool calls the model
	// issued. A non-nil onThink also requests thinking. toolSchemas, when
	// non-empty, advertises native tool-calling for this turn.
	Chat(ctx context.Context, model string, messages []prompting.Message, toolSchemas []tools.ToolSchema, onDelta, onThink func(string)) (ChatResult, error)
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

// llamacppModel adapts *llamacpp.LlamacppProvider to the Model interface.
type llamacppModel struct {
	client *llamacpp.LlamacppProvider
	model  string // cached model name for context-length lookups
}

func newLlamacppModel(host, model string) llamacppModel {
	return llamacppModel{client: llamacpp.NewLlamacppProvider(llamacpp.New(host)), model: model}
}

func (o ollamaModel) Chat(ctx context.Context, model string, messages []prompting.Message, toolSchemas []tools.ToolSchema, onDelta, onThink func(string)) (ChatResult, error) {
	om := make([]ollama.Message, len(messages))
	for i, m := range messages {
		om[i] = toOllamaMessage(m)
	}
	req := ollama.ChatRequest{Model: model, Messages: om, Think: onThink != nil, Tools: toOllamaTools(toolSchemas)}
	// Request the model's full context window (cached lookup) instead of leaving
	// Ollama on its small server default. A lookup failure falls back to 0
	// (num_ctx unset), so a chat still runs on the default window.
	if n, err := o.client.ContextLength(ctx, model); err == nil {
		req.NumCtx = n
	}
	res, err := o.client.Chat(ctx, req, onDelta, onThink)
	if err != nil {
		return ChatResult{Content: res.Content}, err
	}
	return ChatResult{Content: res.Content, ToolCalls: fromOllamaToolCalls(res.ToolCalls)}, nil
}

// toOllamaMessage converts a prompting.Message to Ollama's wire shape,
// carrying native (non-stringified) tool-call arguments straight through.
func toOllamaMessage(m prompting.Message) ollama.Message {
	out := ollama.Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
	if len(m.ToolCalls) == 0 {
		return out
	}
	out.ToolCalls = make([]ollama.ToolCall, len(m.ToolCalls))
	for i, tc := range m.ToolCalls {
		out.ToolCalls[i].ID = tc.ID
		out.ToolCalls[i].Function.Name = tc.Name
		out.ToolCalls[i].Function.Arguments = tc.Arguments
	}
	return out
}

// toOllamaTools maps the provider-agnostic tool schema (internal/tools owns
// Descriptor; internal/llm/* must not import it, per the import-direction
// matrix) to Ollama's wire-format Tool list.
func toOllamaTools(schemas []tools.ToolSchema) []ollama.Tool {
	if len(schemas) == 0 {
		return nil
	}
	out := make([]ollama.Tool, len(schemas))
	for i, s := range schemas {
		out[i] = ollama.Tool{Type: "function", Function: ollama.ToolFunction{
			Name: s.Name, Description: s.Description, Parameters: s.Parameters,
		}}
	}
	return out
}

func fromOllamaToolCalls(calls []ollama.ToolCall) []prompting.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]prompting.ToolCall, len(calls))
	for i, c := range calls {
		out[i] = prompting.ToolCall{ID: c.ID, Name: c.Function.Name, Arguments: c.Function.Arguments}
	}
	return out
}

func (o ollamaModel) Ready(ctx context.Context, model string) error {
	return o.client.Ready(ctx, model)
}

func (o ollamaModel) ContextLength(ctx context.Context, model string) (int, error) {
	return o.client.ContextLength(ctx, model)
}

func (l llamacppModel) Chat(ctx context.Context, model string, messages []prompting.Message, toolSchemas []tools.ToolSchema, onDelta, onThink func(string)) (ChatResult, error) {
	msgs := make([]provider.Message, len(messages))
	for i, m := range messages {
		msgs[i] = toProviderMessage(m)
	}
	req := provider.ChatRequest{Model: model, Messages: msgs, Think: onThink != nil, Tools: toProviderTools(toolSchemas)}
	if n, err := l.client.ContextLength(ctx, model); err == nil {
		req.NumCtx = n
	}
	res, err := l.client.Chat(ctx, req, onDelta, onThink)
	if err != nil {
		return ChatResult{Content: res.Content}, err
	}
	return ChatResult{Content: res.Content, ToolCalls: fromProviderToolCalls(res.ToolCalls)}, nil
}

func toProviderMessage(m prompting.Message) provider.Message {
	out := provider.Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
	if len(m.ToolCalls) == 0 {
		return out
	}
	out.ToolCalls = make([]provider.ToolCall, len(m.ToolCalls))
	for i, tc := range m.ToolCalls {
		out.ToolCalls[i] = provider.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments}
	}
	return out
}

func toProviderTools(schemas []tools.ToolSchema) []provider.Tool {
	if len(schemas) == 0 {
		return nil
	}
	out := make([]provider.Tool, len(schemas))
	for i, s := range schemas {
		out[i] = provider.Tool{Type: "function", Function: provider.ToolFunction{
			Name: s.Name, Description: s.Description, Parameters: s.Parameters,
		}}
	}
	return out
}

func fromProviderToolCalls(calls []provider.ToolCall) []prompting.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]prompting.ToolCall, len(calls))
	for i, c := range calls {
		out[i] = prompting.ToolCall{ID: c.ID, Name: c.Name, Arguments: c.Arguments}
	}
	return out
}

func (l llamacppModel) Ready(ctx context.Context, model string) error {
	return l.client.Ready(ctx, model)
}

func (l llamacppModel) ContextLength(ctx context.Context, model string) (int, error) {
	return l.client.ContextLength(ctx, model)
}
