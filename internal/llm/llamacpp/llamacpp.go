// Package llamacpp is the llama.cpp HTTP server client. It speaks the
// OpenAI-compatible API exposed by `llama-server`: streaming and non-streaming
// chat completions, model readiness via /v1/models, and context-length lookup
// from model metadata.
//
// FormatStyle is Prompt: the llama.cpp server does not honor a "format" field.
// The invoker injects a JSON instruction into the user prompt instead, so
// constrained decoding still works — just via prompt engineering rather than
// a server-side hook.
//
// Source contract: docs/architecture/adr/0013-llm-provider-abstraction.md.
// Behavior contract: tests/features/llm/llamacpp_adapter.feature.
package llamacpp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"agentx/internal/llm/provider"
)

// Client talks to a local llama.cpp server (llama-server). It speaks the
// OpenAI-compatible API: POST /v1/chat/completions for completions,
// GET /v1/models for readiness, GET /v1/models/{id} for context length.
//
// The Client's native Complete/Chat methods use llamacpp types (llamacpp.CompleteRequest,
// llamacpp.ChatRequest). To satisfy provider.Provider, see LlamacppProvider below.
type Client struct {
	baseURL string
	http    *http.Client

	mu     sync.Mutex
	ctxLen map[string]int // model → reported context length (cached)
}

// New returns a Client for the given host (e.g. "localhost:8080"). A bare
// host:port is assumed to use http.
func New(host string) *Client {
	return NewWithHTTPClient(host, http.DefaultClient)
}

// NewWithHTTPClient is like New but uses the supplied *http.Client.
func NewWithHTTPClient(host string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{baseURL: normalizeHost(host), http: hc, ctxLen: map[string]int{}}
}

func normalizeHost(host string) string {
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return strings.TrimRight(host, "/")
	}
	return "http://" + strings.TrimRight(host, "/")
}

// Message is a single chat message (matches the OpenAI wire format).
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompleteRequest is a non-streaming chat completion request.
type CompleteRequest struct {
	Model       string
	Messages    []Message
	Temperature float64
	Seed        int
	NumCtx      int
	Think       bool
}

// ChatRequest is a streaming chat completion request.
type ChatRequest struct {
	Model    string
	Messages []Message
	Think    bool
	NumCtx   int
}

// chatChunk is a single SSE chunk from the llama.cpp server.
type chatChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Delta   struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Complete runs a single non-streaming chat completion and returns the
// assembled message content. It honors ctx cancellation.
func (c *Client) Complete(ctx context.Context, req CompleteRequest) (string, error) {
	payload := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   false,
	}
	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}
	if req.Seed != 0 {
		payload["seed"] = req.Seed
	}
	if req.NumCtx > 0 {
		payload["n_ctx"] = req.NumCtx
	}
	if req.Think {
		payload["think"] = true
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode complete request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build complete request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("complete request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llama.cpp chat returned status %d", resp.StatusCode)
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode complete response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("llama.cpp error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llama.cpp returned empty choices")
	}
	return out.Choices[0].Message.Content, nil
}

// Chat streams a chat completion, invoking onDelta for each content chunk and
// onThink (when non-nil and Think is set) for each reasoning chunk, and returns
// the assembled content. It honors ctx cancellation.
func (c *Client) Chat(ctx context.Context, req ChatRequest, onDelta, onThink func(string)) (string, error) {
	payload := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   true,
	}
	if req.Think {
		payload["think"] = true
	}
	if req.NumCtx > 0 {
		payload["n_ctx"] = req.NumCtx
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("chat request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llama.cpp chat returned status %d", resp.StatusCode)
	}

	var assembled strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || !strings.HasPrefix(string(line), "data: ") {
			continue
		}
		data := strings.TrimPrefix(string(line), "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return assembled.String(), fmt.Errorf("decode chat chunk: %w", err)
		}
		if chunk.Error != nil {
			return assembled.String(), fmt.Errorf("llama.cpp error: %s", chunk.Error.Message)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				assembled.WriteString(choice.Delta.Content)
				if onDelta != nil {
					onDelta(choice.Delta.Content)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return assembled.String(), fmt.Errorf("read chat stream: %w", err)
	}
	return assembled.String(), nil
}

type modelsResponse struct {
	Data []struct {
		ID   string `json:"id"`
		Obj  string `json:"object"`
		Perm string `json:"permission"`
		Own  string `json:"owned_by"`
		// ContextLength is the optional context_length field returned by some
		// llama.cpp server builds (not all expose it).
		ContextLength int `json:"context_length"`
	} `json:"data"`
}

// Ready reports whether the host is reachable and the model is available.
func (c *Client) Ready(ctx context.Context, model string) error {
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

	var tags modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return fmt.Errorf("decode models: %w", err)
	}
	for _, m := range tags.Data {
		if modelMatches(m.ID, model) {
			return nil
		}
	}
	return fmt.Errorf("model %q is not available", model)
}

// ContextLength reports the model's maximum context window (in tokens) from
// GET /v1/models/{id}. The value is cached per model — it is a fixed property
// of the model file — so repeat calls cost one round trip.
func (c *Client) ContextLength(ctx context.Context, model string) (int, error) {
	c.mu.Lock()
	if n, ok := c.ctxLen[model]; ok {
		c.mu.Unlock()
		return n, nil
	}
	c.mu.Unlock()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models/"+model, nil)
	if err != nil {
		return 0, fmt.Errorf("build model request: %w", err)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("llama.cpp model unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("llama.cpp model returned status %d", resp.StatusCode)
	}

	var modelInfo struct {
		ID            string `json:"id"`
		ContextLength int    `json:"context_length"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modelInfo); err != nil {
		return 0, fmt.Errorf("decode model: %w", err)
	}
	if modelInfo.ContextLength <= 0 {
		return 0, fmt.Errorf("model %q reports no context_length", model)
	}
	c.mu.Lock()
	c.ctxLen[model] = modelInfo.ContextLength
	c.mu.Unlock()
	return modelInfo.ContextLength, nil
}

// modelMatches treats exact match or prefix match (name:tag) as matching.
func modelMatches(listed, want string) bool {
	if listed == want {
		return true
	}
	if i := strings.IndexByte(listed, ':'); i >= 0 && listed[:i] == want {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// provider.Provider adapter
//
// *Client itself does NOT implement provider.Provider because its Complete/Chat
// methods accept llamacpp-native request types. LlamacppProvider wraps *Client
// and converts between the two.
// ---------------------------------------------------------------------------

// LlamacppProvider wraps *Client to satisfy provider.Provider.
//
// It converts provider.CompleteRequest → llamacpp.CompleteRequest, provider.ChatRequest
// → llamacpp.ChatRequest, and provider.Message → llamacpp.Message at the boundary,
// so the rest of the runtime stays provider-agnostic while tests can still
// assert on the native llamacpp wire format.
type LlamacppProvider struct {
	*Client
}

// NewLlamacppProvider wraps an existing *Client to satisfy provider.Provider.
func NewLlamacppProvider(c *Client) *LlamacppProvider {
	return &LlamacppProvider{Client: c}
}

// Compile-time check.
var _ provider.Provider = (*LlamacppProvider)(nil)

// FormatStyle reports that llama.cpp does NOT honor "format"; the invoker
// injects JSON instruction into the user prompt instead.
func (l *LlamacppProvider) FormatStyle() provider.FormatStyle { return provider.FormatStylePrompt }

// Complete runs a non-streaming completion through llama.cpp.
func (l *LlamacppProvider) Complete(ctx context.Context, req provider.CompleteRequest) (string, error) {
	msgs := make([]Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = Message{Role: m.Role, Content: m.Content}
	}
	return l.Client.Complete(ctx, CompleteRequest{
		Model:       req.Model,
		Messages:    msgs,
		Temperature: req.Temperature,
		Seed:        req.Seed,
		NumCtx:      req.NumCtx,
		Think:       req.Think,
	})
}

// Chat streams a chat completion through llama.cpp.
func (l *LlamacppProvider) Chat(ctx context.Context, req provider.ChatRequest, onDelta, onThink func(string)) (string, error) {
	msgs := make([]Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = Message{Role: m.Role, Content: m.Content}
	}
	return l.Client.Chat(ctx, ChatRequest{
		Model:    req.Model,
		Messages: msgs,
		Think:    req.Think,
		NumCtx:   req.NumCtx,
	}, onDelta, onThink)
}

// Ready reports llama.cpp model availability.
func (l *LlamacppProvider) Ready(ctx context.Context, model string) error {
	return l.Client.Ready(ctx, model)
}

// ContextLength reports the llama.cpp model's context window.
func (l *LlamacppProvider) ContextLength(ctx context.Context, model string) (int, error) {
	return l.Client.ContextLength(ctx, model)
}
