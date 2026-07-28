package ollama

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

// *Client itself does not implement provider.Provider (its method signatures
// accept Ollama-native request types). OllamaProvider wraps *Client to provide
// the Provider interface; see the compile-time check there.

// Client talks to a local Ollama runtime.
type Client struct {
	baseURL string
	http    *http.Client

	mu     sync.Mutex     // guards ctxLen
	ctxLen map[string]int // model → reported context length (cached; /api/show)
}

// New returns a Client for the given host (e.g. "localhost:11434"). A bare
// host:port is assumed to use http. It uses http.DefaultClient.
func New(host string) *Client {
	return NewWithHTTPClient(host, http.DefaultClient)
}

// NewWithHTTPClient is like New but uses the supplied *http.Client. Pass a
// client with a tuned Transport (e.g. raised MaxConnsPerHost /
// MaxIdleConnsPerHost) when issuing many concurrent requests, so the client's
// connection pool does not itself cap concurrency. A nil hc falls back to
// http.DefaultClient.
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

// Message is a single chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// toProvider converts ollama Messages to provider Messages.
func toProviderMessages(msgs []Message) []provider.Message {
	out := make([]provider.Message, len(msgs))
	for i, m := range msgs {
		out[i] = provider.Message{Role: m.Role, Content: m.Content}
	}
	return out
}

// fromProvider converts provider Messages to ollama Messages.
func fromProviderMessages(msgs []provider.Message) []Message {
	out := make([]Message, len(msgs))
	for i, m := range msgs {
		out[i] = Message{Role: m.Role, Content: m.Content}
	}
	return out
}

// ChatRequest is a streaming chat completion request. When Think is set the
// request asks the model to emit reasoning, delivered separately from content.
// NumCtx, when > 0, sets the context window (options.num_ctx) so Ollama allots
// the model's full window instead of its small server default.
type ChatRequest struct {
	Model    string
	Messages []Message
	Think    bool
	NumCtx   int
}

type chatChunk struct {
	Message struct {
		Role     string `json:"role"`
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	Done  bool   `json:"done"`
	Error string `json:"error,omitempty"`
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
		payload["options"] = map[string]any{"num_ctx": req.NumCtx}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
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
		return "", fmt.Errorf("ollama chat returned status %d", resp.StatusCode)
	}

	var assembled strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var chunk chatChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			return assembled.String(), fmt.Errorf("decode chat chunk: %w", err)
		}
		if chunk.Error != "" {
			return assembled.String(), fmt.Errorf("ollama error: %s", chunk.Error)
		}
		if chunk.Message.Thinking != "" && onThink != nil {
			onThink(chunk.Message.Thinking)
		}
		if chunk.Message.Content != "" {
			assembled.WriteString(chunk.Message.Content)
			if onDelta != nil {
				onDelta(chunk.Message.Content)
			}
		}
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return assembled.String(), fmt.Errorf("read chat stream: %w", err)
	}
	return assembled.String(), nil
}

// CompleteRequest is a non-streaming, optionally schema-constrained completion.
// Unlike Chat it returns the whole message at once and supports per-request
// sampling options (Temperature, Seed) and Format (a JSON schema for constrained
// decoding). It is the request shape the classifier fan-out and the decomposition
// planner use. Think requests reasoning, same as ChatRequest.Think — budget/retry
// policy for it is deliberately not here; see ADR 0012 Phase 1 (it lives at the
// orchestrator layer, mirroring how Chat's own budget dance lives in tool_cycle.go,
// not in this client).
type CompleteRequest struct {
	Model       string
	Messages    []Message
	Temperature float64
	Seed        int
	Format      json.RawMessage // JSON schema; nil leaves output unconstrained
	NumCtx      int
	Think       bool
}

// Complete runs a single non-streaming chat completion and returns the assembled
// message content. It honors ctx cancellation.
func (c *Client) Complete(ctx context.Context, req CompleteRequest) (string, error) {
	options := map[string]any{}
	if req.Temperature > 0 {
		options["temperature"] = req.Temperature
	}
	if req.Seed != 0 {
		options["seed"] = req.Seed
	}
	if req.NumCtx > 0 {
		options["num_ctx"] = req.NumCtx
	}

	payload := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   false,
	}
	if req.Think {
		payload["think"] = true
	}
	if len(options) > 0 {
		payload["options"] = options
	}
	if len(req.Format) > 0 {
		payload["format"] = req.Format
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode complete request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
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
		return "", fmt.Errorf("ollama chat returned status %d", resp.StatusCode)
	}

	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode complete response: %w", err)
	}
	if out.Error != "" {
		return "", fmt.Errorf("ollama error: %s", out.Error)
	}
	return out.Message.Content, nil
}

type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// Ready reports whether the host is reachable and the model is available.
func (c *Client) Ready(ctx context.Context, model string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("build tags request: %w", err)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama tags returned status %d", resp.StatusCode)
	}

	var tags tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return fmt.Errorf("decode tags: %w", err)
	}
	for _, m := range tags.Models {
		if modelMatches(m.Name, model) {
			return nil
		}
	}
	return fmt.Errorf("model %q is not available", model)
}

// showResponse is the subset of POST /api/show we need: model_info carries the
// architecture and the per-architecture context length (e.g. "llama.context_length").
type showResponse struct {
	ModelInfo map[string]any `json:"model_info"`
}

// ContextLength reports the model's maximum context window (in tokens) from
// POST /api/show. It reads model_info["<general.architecture>.context_length"].
// The value is cached per model — it is a fixed property of the model file — so
// repeat calls (per-prompt num_ctx, the visualizer poll) cost one round trip.
func (c *Client) ContextLength(ctx context.Context, model string) (int, error) {
	c.mu.Lock()
	if n, ok := c.ctxLen[model]; ok {
		c.mu.Unlock()
		return n, nil
	}
	c.mu.Unlock()

	body, err := json.Marshal(map[string]any{"model": model})
	if err != nil {
		return 0, fmt.Errorf("encode show request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/show", bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build show request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("ollama show unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("ollama show returned status %d", resp.StatusCode)
	}

	var show showResponse
	if err := json.NewDecoder(resp.Body).Decode(&show); err != nil {
		return 0, fmt.Errorf("decode show: %w", err)
	}
	n := contextLengthOf(show.ModelInfo)
	if n <= 0 {
		return 0, fmt.Errorf("model %q reports no context_length", model)
	}
	c.mu.Lock()
	c.ctxLen[model] = n
	c.mu.Unlock()
	return n, nil
}

// contextLengthOf extracts "<architecture>.context_length" from a model_info map.
// The architecture prefix (llama, qwen2, gemma, …) is itself a model_info field.
func contextLengthOf(info map[string]any) int {
	if info == nil {
		return 0
	}
	arch, _ := info["general.architecture"].(string)
	if arch != "" {
		if n, ok := asInt(info[arch+".context_length"]); ok {
			return n
		}
	}
	// Fallback: any *.context_length key (architecture prefix unknown).
	for k, v := range info {
		if strings.HasSuffix(k, ".context_length") {
			if n, ok := asInt(v); ok {
				return n
			}
		}
	}
	return 0
}

// asInt coerces a JSON number (decoded as float64) to a positive int.
func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), n > 0
	case int:
		return n, n > 0
	}
	return 0, false
}

// modelMatches treats "name" and "name:tag" as matching the configured model.
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
// methods accept Ollama-native request types (ollama.CompleteRequest,
// ollama.ChatRequest) whose field shapes differ from provider.CompleteRequest
// and provider.ChatRequest. Instead, OllamaProvider wraps *Client and converts
// between the two.
//
// Direct use of *Client (e.g. tests asserting on the exact Ollama wire shape)
// is preserved; the runtime talks to Ollama through OllamaProvider.
// ---------------------------------------------------------------------------

// OllamaProvider wraps *Client to satisfy provider.Provider.
//
// It converts provider.CompleteRequest → ollama.CompleteRequest, provider.ChatRequest
// → ollama.ChatRequest, and provider.Message → ollama.Message at the boundary,
// so the rest of the runtime stays provider-agnostic while tests can still
// assert on the native Ollama wire format.
type OllamaProvider struct {
	*Client
}

// NewOllamaProvider wraps an existing *Client to satisfy provider.Provider.
func NewOllamaProvider(c *Client) *OllamaProvider {
	return &OllamaProvider{Client: c}
}

// Compile-time check.
var _ provider.Provider = (*OllamaProvider)(nil)

// FormatStyle reports that Ollama honors JSON-schema constrained decoding via
// the "format" field on the request.
func (o *OllamaProvider) FormatStyle() provider.FormatStyle { return provider.FormatStyleNative }

// Complete runs a non-streaming completion through Ollama.
func (o *OllamaProvider) Complete(ctx context.Context, req provider.CompleteRequest) (string, error) {
	return o.Client.Complete(ctx, CompleteRequest{
		Model:       req.Model,
		Messages:    fromProviderMessages(req.Messages),
		Temperature: req.Temperature,
		Seed:        req.Seed,
		Format:      req.Format,
		NumCtx:      req.NumCtx,
		Think:       req.Think,
	})
}

// Chat streams a chat completion through Ollama.
func (o *OllamaProvider) Chat(ctx context.Context, req provider.ChatRequest, onDelta, onThink func(string)) (string, error) {
	return o.Client.Chat(ctx, ChatRequest{
		Model:    req.Model,
		Messages: fromProviderMessages(req.Messages),
		Think:    req.Think,
		NumCtx:   req.NumCtx,
	}, onDelta, onThink)
}

// Ready reports Ollama model availability.
func (o *OllamaProvider) Ready(ctx context.Context, model string) error {
	return o.Client.Ready(ctx, model)
}

// ContextLength reports the Ollama model's context window.
func (o *OllamaProvider) ContextLength(ctx context.Context, model string) (int, error) {
	return o.Client.ContextLength(ctx, model)
}
