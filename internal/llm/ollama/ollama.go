package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Client talks to a local Ollama runtime.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a Client for the given host (e.g. "localhost:11434"). A bare
// host:port is assumed to use http.
func New(host string) *Client {
	return &Client{baseURL: normalizeHost(host), http: http.DefaultClient}
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

// ChatRequest is a streaming chat completion request. When Think is set the
// request asks the model to emit reasoning, delivered separately from content.
type ChatRequest struct {
	Model    string
	Messages []Message
	Think    bool
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
