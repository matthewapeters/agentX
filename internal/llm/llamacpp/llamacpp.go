package llamacpp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"agentx/internal/llm/provider"
)

// Client talks to a local llama.cpp server (llama-server) over HTTP.
type Client struct {
	baseURL string
	http    *http.Client
	mu      sync.Mutex
	ctxLen  map[string]int
}

// New creates a Client pointing at host (e.g. "localhost:8080").
func New(host string) *Client {
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return &Client{baseURL: strings.TrimRight(host, "/"), http: http.DefaultClient, ctxLen: map[string]int{}}
	}
	return &Client{baseURL: "http://" + host, http: http.DefaultClient, ctxLen: map[string]int{}}
}

// Message is a single chat message. ToolCalls is set on an assistant message
// that invoked tools; ToolCallID is set on a role:"tool" message answering a
// prior call (required by the OpenAI-compatible wire format llama.cpp speaks).
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolFunction describes one callable tool: its name, a model-facing
// description, and its arguments as a JSON Schema object.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// Tool is a single entry in a ChatRequest's Tools list.
type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolCall is a model-issued invocation of one Tool, OpenAI-compatible wire
// shape: unlike Ollama, llama.cpp assigns an id and encodes Arguments as a
// JSON-object string rather than a native object.
type ToolCall struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ChatRequest is a streaming chat request. Tools, when non-empty, advertises
// native tool-calling.
type ChatRequest struct {
	Model    string
	Messages []Message
	Think    bool
	NumCtx   int
	Tools    []Tool
}

// ChatResult is a completed chat turn: the assembled text content and any
// native tool calls the model issued.
type ChatResult struct {
	Content   string
	ToolCalls []ToolCall
}

// CompleteRequest is a non-streaming completion request.
type CompleteRequest struct {
	Model    string
	Messages []Message
}

// toolCallDelta is one streamed fragment of a tool call, OpenAI's incremental
// wire shape: Index identifies which call a fragment belongs to across
// chunks; ID and Function.Name typically arrive once, on the first fragment
// for that index, while Function.Arguments arrives split across many
// fragments and must be concatenated, not treated as complete JSON on its
// own.
type toolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// accumulatedToolCall collects one tool call's fragments across the stream.
type accumulatedToolCall struct {
	id   string
	name string
	args strings.Builder
}

// Chat streams a chat completion, invoking onDelta for each content chunk
// and onThink for each reasoning chunk, and returns the assembled response
// plus any tool calls the model issued. llama.cpp's OpenAI-compatible stream
// emits tool_calls incrementally, keyed by index — id/name usually arrive
// once on a call's first fragment, arguments arrive split across many
// fragments — so fragments are accumulated by index and only assembled into
// complete ToolCalls once the stream ends, rather than treated as complete
// calls on arrival.
func (c *Client) Chat(ctx context.Context, req ChatRequest, onDelta, onThink func(string)) (ChatResult, error) {
	payload := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   true,
		"think":    req.Think,
	}
	if req.NumCtx > 0 {
		payload["n_ctx"] = req.NumCtx
	}
	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResult{}, fmt.Errorf("encode chat: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResult{}, fmt.Errorf("build chat: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return ChatResult{}, fmt.Errorf("chat request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return ChatResult{}, fmt.Errorf("chat status %d: %s (model=%q)", resp.StatusCode, strings.TrimSpace(string(body)), req.Model)
	}
	var assembled strings.Builder
	calls := map[int]*accumulatedToolCall{}
	var order []int // preserves first-seen index order regardless of map iteration
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}
		data := bytes.TrimPrefix(line, []byte("data: "))
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("[DONE]")) {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string          `json:"content"`
					ToolCalls []toolCallDelta `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(trimmed, &chunk); err != nil {
			continue
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				if onDelta != nil {
					onDelta(c.Delta.Content)
				}
				assembled.WriteString(c.Delta.Content)
			}
			for _, frag := range c.Delta.ToolCalls {
				acc, ok := calls[frag.Index]
				if !ok {
					acc = &accumulatedToolCall{}
					calls[frag.Index] = acc
					order = append(order, frag.Index)
				}
				if frag.ID != "" {
					acc.id = frag.ID
				}
				if frag.Function.Name != "" {
					acc.name = frag.Function.Name
				}
				acc.args.WriteString(frag.Function.Arguments)
			}
		}
	}
	toolCalls := make([]ToolCall, 0, len(order))
	for _, idx := range order {
		acc := calls[idx]
		tc := ToolCall{ID: acc.id}
		tc.Function.Name = acc.name
		tc.Function.Arguments = acc.args.String()
		toolCalls = append(toolCalls, tc)
	}
	if err := scanner.Err(); err != nil {
		return ChatResult{Content: assembled.String(), ToolCalls: toolCalls}, fmt.Errorf("chat read: %w", err)
	}
	return ChatResult{Content: assembled.String(), ToolCalls: toolCalls}, nil
}

// Complete runs a non-streaming completion and returns the assembled response.
func (c *Client) Complete(ctx context.Context, req CompleteRequest) (string, error) {
	payload := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode complete: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build complete: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("complete request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("complete status %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode complete: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("empty choices")
	}
	return out.Choices[0].Message.Content, nil
}

// Ready reports whether the host is reachable and the model is available.
func (c *Client) Ready(ctx context.Context, model string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return fmt.Errorf("build models: %w", err)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("models request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("models status %d", resp.StatusCode)
	}
	var tags struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return fmt.Errorf("decode models: %w", err)
	}
	for _, m := range tags.Data {
		if m.ID == model {
			return nil
		}
	}
	return fmt.Errorf("model %q not available", model)
}

// ContextLength reports the model's maximum context window in tokens.
func (c *Client) ContextLength(ctx context.Context, model string) (int, error) {
	c.mu.Lock()
	if n, ok := c.ctxLen[model]; ok {
		c.mu.Unlock()
		return n, nil
	}
	c.mu.Unlock()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models/"+model, nil)
	if err != nil {
		return 0, fmt.Errorf("build model: %w", err)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("model request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("model status %d", resp.StatusCode)
	}
	var info struct {
		ContextLength int `json:"context_length"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return 0, fmt.Errorf("decode model: %w", err)
	}
	if info.ContextLength <= 0 {
		return 0, fmt.Errorf("no context_length for %q", model)
	}
	c.mu.Lock()
	c.ctxLen[model] = info.ContextLength
	c.mu.Unlock()
	return info.ContextLength, nil
}

// LlamacppProvider wraps *Client to satisfy provider.Provider.
type LlamacppProvider struct {
	*Client
}

// NewLlamacppProvider wraps *Client as a provider.Provider.
func NewLlamacppProvider(c *Client) *LlamacppProvider {
	return &LlamacppProvider{Client: c}
}

// FormatStyle reports Prompt style: the invoker injects JSON instruction.
func (l *LlamacppProvider) FormatStyle() provider.FormatStyle {
	return provider.FormatStylePrompt
}

// Chat streams a chat completion.
func (l *LlamacppProvider) Chat(ctx context.Context, req provider.ChatRequest, onDelta, onThink func(string)) (provider.ChatResult, error) {
	msgs := make([]Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = toLlamacppMessage(m)
	}
	tools := make([]Tool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = Tool{Type: t.Type, Function: ToolFunction{
			Name: t.Function.Name, Description: t.Function.Description, Parameters: t.Function.Parameters,
		}}
	}
	res, err := l.Client.Chat(ctx, ChatRequest{Model: req.Model, Messages: msgs, Think: req.Think, NumCtx: req.NumCtx, Tools: tools}, onDelta, onThink)
	if err != nil {
		return provider.ChatResult{Content: res.Content}, err
	}
	return provider.ChatResult{Content: res.Content, ToolCalls: fromLlamacppToolCalls(res.ToolCalls)}, nil
}

// toLlamacppMessage converts a provider.Message to llama.cpp's OpenAI-compatible
// wire shape, JSON-string-encoding ToolCalls arguments (llama.cpp/OpenAI expect
// a string there, unlike Ollama's native object).
func toLlamacppMessage(m provider.Message) Message {
	out := Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
	if len(m.ToolCalls) == 0 {
		return out
	}
	out.ToolCalls = make([]ToolCall, len(m.ToolCalls))
	for i, tc := range m.ToolCalls {
		args, _ := json.Marshal(tc.Arguments)
		out.ToolCalls[i] = ToolCall{ID: tc.ID}
		out.ToolCalls[i].Function.Name = tc.Name
		out.ToolCalls[i].Function.Arguments = string(args)
	}
	return out
}

// fromLlamacppToolCalls decodes each call's JSON-string arguments into a
// native map so provider.ToolCall's shape matches Ollama's regardless of
// backend wire encoding.
func fromLlamacppToolCalls(calls []ToolCall) []provider.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]provider.ToolCall, len(calls))
	for i, c := range calls {
		var args map[string]any
		_ = json.Unmarshal([]byte(c.Function.Arguments), &args)
		out[i] = provider.ToolCall{ID: c.ID, Name: c.Function.Name, Arguments: args}
	}
	return out
}

// Complete runs a non-streaming completion.
func (l *LlamacppProvider) Complete(ctx context.Context, req provider.CompleteRequest) (string, error) {
	msgs := make([]Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = Message{Role: m.Role, Content: m.Content}
	}
	return l.Client.Complete(ctx, CompleteRequest{Model: req.Model, Messages: msgs})
}

// Ready reports model availability.
func (l *LlamacppProvider) Ready(ctx context.Context, model string) error {
	return l.Client.Ready(ctx, model)
}

// ContextLength reports the context window.
func (l *LlamacppProvider) ContextLength(ctx context.Context, model string) (int, error) {
	return l.Client.ContextLength(ctx, model)
}

// Config returns the llama.cpp provider's configuration as a map for transport.
func (l *LlamacppProvider) Config() map[string]any {
	return map[string]any{
		"host": l.Client.baseURL,
	}
}

// ListModels returns the list of models hosted on the llama.cpp server.
func (l *LlamacppProvider) ListModels() ([]string, error) {
	httpReq, err := http.NewRequest(http.MethodGet, l.Client.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("build models: %w", err)
	}
	resp, err := l.Client.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("models request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models status %d", resp.StatusCode)
	}
	var tags struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}
	models := make([]string, len(tags.Data))
	for i, m := range tags.Data {
		models[i] = m.ID
	}
	return models, nil
}

// ConfigSchema returns the llama.cpp configuration schema.
func (l *LlamacppProvider) ConfigSchema() map[string]provider.SchemaField {
	return map[string]provider.SchemaField{
		"host": {
			Name:        "Host",
			Type:        "host",
			Default:     "localhost:8080",
			Required:    true,
			ReadOnly:    false,
			Description: "The llama.cpp server host address (host:port).",
			RestartRequired: true,
		},
		"model": {
			Name:        "Model",
			Type:        "model",
			Default:     "",
			Required:    true,
			ReadOnly:    false,
			Description: "The model name (e.g., 'llama3.1').",
			RestartRequired: true,
		},
	}
}

// TestHost probes the llama.cpp server's /v1/models endpoint. A 200 response
// means the host is reachable; any other outcome returns the underlying error.
func (l *LlamacppProvider) TestHost(ctx context.Context) error {
	return l.Client.TestHost(ctx)
}

// TestHost probes the llama.cpp server's /v1/models endpoint. A 200 response
// means the host is reachable and responding; any other outcome returns the
// underlying error so the caller (transport, TUI) can surface it to the user.
func (c *Client) TestHost(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return fmt.Errorf("build models request: %w", err)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("llama.cpp unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("llama.cpp models returned status %d", resp.StatusCode)
	}
	return nil
}