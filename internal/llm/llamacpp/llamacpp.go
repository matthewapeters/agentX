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

// Message is a single chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is a streaming chat request.
type ChatRequest struct {
	Model    string
	Messages []Message
	Think    bool
	NumCtx   int
}

// CompleteRequest is a non-streaming completion request.
type CompleteRequest struct {
	Model    string
	Messages []Message
}

// Chat streams a chat completion, invoking onDelta for each content chunk
// and onThink for each reasoning chunk, and returns the assembled response.
func (c *Client) Chat(ctx context.Context, req ChatRequest, onDelta, onThink func(string)) (string, error) {
	payload := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   true,
		"think":    req.Think,
	}
	if req.NumCtx > 0 {
		payload["n_ctx"] = req.NumCtx
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode chat: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build chat: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("chat request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("chat status %d: %s (model=%q)", resp.StatusCode, strings.TrimSpace(string(body)), req.Model)
	}
	var assembled strings.Builder
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
		if len(trimmed) == 0 {
			continue
		}
		// Skip done events
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
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
		}
	}
	if err := scanner.Err(); err != nil {
		return assembled.String(), fmt.Errorf("chat read: %w", err)
	}
	return assembled.String(), nil
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
func (l *LlamacppProvider) Chat(ctx context.Context, req provider.ChatRequest, onDelta, onThink func(string)) (string, error) {
	msgs := make([]Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = Message{Role: m.Role, Content: m.Content}
	}
	return l.Client.Chat(ctx, ChatRequest{Model: req.Model, Messages: msgs, Think: req.Think, NumCtx: req.NumCtx}, onDelta, onThink)
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