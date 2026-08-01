package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureServer returns an httptest.Server that decodes each request body into dst
// (via a fresh pointer each call is the caller's job — here dst is shared, so tests
// using it are single-request) and replies with a minimal valid /api/chat response.
func captureServer(t *testing.T, dst *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"ok"}}`))
	}))
}

func TestCompleteThinkTrueSetsTopLevelPayloadField(t *testing.T) {
	var got map[string]any
	srv := captureServer(t, &got)
	defer srv.Close()

	c := New(srv.URL)
	out, err := c.Complete(context.Background(), CompleteRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Think:    true,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "ok" {
		t.Errorf("content = %q, want %q", out, "ok")
	}
	think, ok := got["think"]
	if !ok {
		t.Fatal("payload missing top-level \"think\" key")
	}
	if think != true {
		t.Errorf("payload[\"think\"] = %v, want true", think)
	}
	if opts, ok := got["options"].(map[string]any); ok {
		if _, nested := opts["think"]; nested {
			t.Error("\"think\" must be top-level, not nested under \"options\"")
		}
	}
}

func TestCompleteThinkFalseOmitsPayloadField(t *testing.T) {
	var got map[string]any
	srv := captureServer(t, &got)
	defer srv.Close()

	c := New(srv.URL)
	if _, err := c.Complete(context.Background(), CompleteRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, ok := got["think"]; ok {
		t.Errorf("payload has \"think\" key = %v, want absent for Think: false", got["think"])
	}
}

// ndjsonServer serves a fixed sequence of raw NDJSON lines from /api/chat —
// Ollama's streaming wire format has no SSE "data: " prefix, unlike
// llama.cpp's OpenAI-compatible endpoint.
func ndjsonServer(t *testing.T, lines []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, l := range lines {
			if _, err := w.Write([]byte(l + "\n")); err != nil {
				t.Fatalf("write line: %v", err)
			}
		}
	}))
}

// GIVEN a streamed response whose final chunk carries a real tool-call id
// (Ollama does assign one, observed live as e.g. "call_d40il62x" — the prior
// assumption that it never does was wrong and silently discarded it via a
// json:"-" tag)
// WHEN Chat parses the stream
// THEN the returned ToolCall keeps that id rather than a synthesized one.
func TestChatPreservesToolCallID(t *testing.T) {
	srv := ndjsonServer(t, []string{
		`{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_d40il62x","function":{"name":"list_dir","arguments":{"path":"/tmp"}}}]},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true}`,
	})
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.Chat(context.Background(), ChatRequest{Model: "m"}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(res.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %+v, want 1 entry", res.ToolCalls)
	}
	if got := res.ToolCalls[0].ID; got != "call_d40il62x" {
		t.Errorf("ToolCalls[0].ID = %q, want the server's real id, not a synthesized one", got)
	}
}

// GIVEN a streamed response whose tool call carries no id (an older
// server/model)
// WHEN Chat parses the stream
// THEN it synthesizes a stable id rather than leaving it empty.
func TestChatSynthesizesMissingToolCallID(t *testing.T) {
	srv := ndjsonServer(t, []string{
		`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"list_dir","arguments":{"path":"/tmp"}}}]},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true}`,
	})
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.Chat(context.Background(), ChatRequest{Model: "m"}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].ID == "" {
		t.Fatalf("ToolCalls = %+v, want 1 entry with a synthesized (non-empty) id", res.ToolCalls)
	}
}

// GIVEN a request message carrying a prior tool call with a real id (history
// being replayed back to the model)
// WHEN the request is marshaled onto the wire
// THEN the id is present in the JSON, not silently dropped by a json:"-" tag.
func TestChatRequestToolCallIDOnWire(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"message":{"content":""},"done":true}` + "\n"))
	}))
	defer srv.Close()

	replayed := Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_d40il62x"}}}
	replayed.ToolCalls[0].Function.Name = "list_dir"
	replayed.ToolCalls[0].Function.Arguments = map[string]any{"path": "/tmp"}

	c := New(srv.URL)
	if _, err := c.Chat(context.Background(), ChatRequest{Model: "m", Messages: []Message{replayed}}, nil, nil); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	msgs, ok := got["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("request messages = %v, want 1 entry", got["messages"])
	}
	msg, ok := msgs[0].(map[string]any)
	if !ok {
		t.Fatalf("messages[0] = %v, want an object", msgs[0])
	}
	toolCalls, ok := msg["tool_calls"].([]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("messages[0].tool_calls = %v, want 1 entry", msg["tool_calls"])
	}
	tc, ok := toolCalls[0].(map[string]any)
	if !ok || tc["id"] != "call_d40il62x" {
		t.Fatalf("wire tool_calls[0] = %v, want id=call_d40il62x present", toolCalls[0])
	}
}
