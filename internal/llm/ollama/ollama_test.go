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
