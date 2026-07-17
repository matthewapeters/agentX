package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"agentx/internal/llm/ollama"
)

func TestCompleteWithThinkingBudgetDisabledPassesThrough(t *testing.T) {
	var calls int32
	var sawThinkKey bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, sawThinkKey = body["think"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"ok"}}`))
	}))
	defer srv.Close()

	client := ollama.New(srv.URL)
	req := ollama.CompleteRequest{Model: "m", Messages: []ollama.Message{{Role: "user", Content: "hi"}}}
	out, err := completeWithThinkingBudget(context.Background(), client, req, 0)
	if err != nil {
		t.Fatalf("completeWithThinkingBudget: %v", err)
	}
	if out != "ok" {
		t.Errorf("out = %q, want ok", out)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
	if sawThinkKey {
		t.Error("budget <= 0 must not set the think key at all")
	}
}

func TestCompleteWithThinkingBudgetSucceedsWithinBudget(t *testing.T) {
	var calls int32
	var sawThinkTrue bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if v, ok := body["think"]; ok && v == true {
			sawThinkTrue = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"ok"}}`))
	}))
	defer srv.Close()

	client := ollama.New(srv.URL)
	req := ollama.CompleteRequest{Model: "m", Messages: []ollama.Message{{Role: "user", Content: "hi"}}}
	out, err := completeWithThinkingBudget(context.Background(), client, req, 5*time.Second)
	if err != nil {
		t.Fatalf("completeWithThinkingBudget: %v", err)
	}
	if out != "ok" {
		t.Errorf("out = %q, want ok", out)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (no retry expected on success)", got)
	}
	if !sawThinkTrue {
		t.Error("expected think:true on the single, successful call")
	}
}

func TestCompleteWithThinkingBudgetRetriesWithoutThinkOnTimeout(t *testing.T) {
	var calls int32
	var retryThinkKeyPresent bool
	const budget = 50 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// Outlive the budget on a bounded sleep (not a block on r.Context().Done():
			// over loopback, client-side cancellation does not reliably/promptly close
			// the server-side connection, which would hang httptest.Server.Close()).
			// By the time this returns, the client has already given up and moved on;
			// the write below is a no-op against a dead connection.
			time.Sleep(4 * budget)
			_, _ = w.Write([]byte(`{"message":{"content":"too-late"}}`))
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, retryThinkKeyPresent = body["think"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"fallback"}}`))
	}))
	defer srv.Close()

	client := ollama.New(srv.URL)
	req := ollama.CompleteRequest{Model: "m", Messages: []ollama.Message{{Role: "user", Content: "hi"}}}
	out, err := completeWithThinkingBudget(context.Background(), client, req, budget)
	if err != nil {
		t.Fatalf("completeWithThinkingBudget: %v", err)
	}
	if out != "fallback" {
		t.Errorf("out = %q, want fallback (the retried call's response)", out)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d, want exactly 2 (one timeout, one retry)", got)
	}
	if retryThinkKeyPresent {
		t.Error("the retry must have Think forced off (no think key at all)")
	}
}

func TestCompleteWithThinkingBudgetNoRetryWhenParentAlreadyCancelled(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(100 * time.Millisecond) // bounded; see the timeout test's comment
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := ollama.New(srv.URL)
	req := ollama.CompleteRequest{Model: "m", Messages: []ollama.Message{{Role: "user", Content: "hi"}}}
	_, err := completeWithThinkingBudget(ctx, client, req, 5*time.Second)
	if err == nil {
		t.Fatal("expected an error from an already-cancelled parent context")
	}
	// The first attempt may or may not reach the network (net/http can fail fast on
	// an already-done context before dialing); what matters is that no retry fired.
	if got := atomic.LoadInt32(&calls); got > 1 {
		t.Errorf("calls = %d, want at most 1 (no retry against a dead parent context)", got)
	}
}
