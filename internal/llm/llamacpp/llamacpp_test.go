package llamacpp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentx/internal/llm/provider"
)

// sseServer serves a fixed sequence of SSE "data: " lines from /v1/chat/completions.
func sseServer(t *testing.T, lines []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, l := range lines {
			if _, err := w.Write([]byte("data: " + l + "\n\n")); err != nil {
				t.Fatalf("write chunk: %v", err)
			}
		}
	}))
}

// GIVEN a stream that sends one tool call whole in a single delta (id, name,
// and complete arguments together — some servers don't fragment at all)
// WHEN Chat reads the stream
// THEN it returns exactly one ToolCall with the full arguments string intact.
func TestChatToolCallSingleChunk(t *testing.T) {
	srv := sseServer(t, []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read_file","arguments":"{\"path\":\"a.go\"}"}}]}}]}`,
		`[DONE]`,
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
	tc := res.ToolCalls[0]
	if tc.ID != "call_1" || tc.Function.Name != "read_file" || tc.Function.Arguments != `{"path":"a.go"}` {
		t.Fatalf("ToolCalls[0] = %+v, want id=call_1 name=read_file args={\"path\":\"a.go\"}", tc)
	}
}

// GIVEN a stream that fragments one tool call's arguments across many deltas
// (id/name on the first fragment only, arguments split mid-JSON-token —
// OpenAI's real incremental streaming shape) WHEN Chat reads the stream THEN
// it reconstructs one ToolCall whose Arguments is the fully concatenated,
// valid JSON string — not a partial fragment.
func TestChatToolCallFragmentedArguments(t *testing.T) {
	srv := sseServer(t, []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read_file","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"pa"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"a.go\"}"}}]}}]}`,
		`[DONE]`,
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
	tc := res.ToolCalls[0]
	if tc.ID != "call_1" || tc.Function.Name != "read_file" {
		t.Fatalf("ToolCalls[0] id/name = %+v, want call_1/read_file", tc)
	}
	if want := `{"path":"a.go"}`; tc.Function.Arguments != want {
		t.Fatalf("ToolCalls[0].Function.Arguments = %q, want %q (fragments must concatenate, not overwrite)", tc.Function.Arguments, want)
	}
}

// GIVEN a stream that interleaves fragments from two parallel tool calls
// (index 0 and index 1, chunks arriving out of call order) WHEN Chat reads
// the stream THEN each call's fragments accumulate separately by index, and
// both are returned in first-seen order — no cross-contamination between
// calls, no chunk-arrival-order dependence.
func TestChatToolCallParallelInterleaved(t *testing.T) {
	srv := sseServer(t, []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read_file","arguments":"{\"pa"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_2","function":{"name":"list_dir","arguments":"{\"pa"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"a.go\"}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"th\":\".\"}"}}]}}]}`,
		`[DONE]`,
	})
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.Chat(context.Background(), ChatRequest{Model: "m"}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(res.ToolCalls) != 2 {
		t.Fatalf("ToolCalls = %+v, want 2 entries", res.ToolCalls)
	}
	first, second := res.ToolCalls[0], res.ToolCalls[1]
	if first.ID != "call_1" || first.Function.Name != "read_file" || first.Function.Arguments != `{"path":"a.go"}` {
		t.Errorf("ToolCalls[0] = %+v, want call_1/read_file/{\"path\":\"a.go\"}", first)
	}
	if second.ID != "call_2" || second.Function.Name != "list_dir" || second.Function.Arguments != `{"path":"."}` {
		t.Errorf("ToolCalls[1] = %+v, want call_2/list_dir/{\"path\":\".\"}", second)
	}
}

// GIVEN a stream with only content deltas, no tool calls
// WHEN Chat reads the stream
// THEN it returns the assembled content and a nil/empty ToolCalls slice.
func TestChatContentOnlyNoToolCalls(t *testing.T) {
	srv := sseServer(t, []string{
		`{"choices":[{"delta":{"content":"Hel"}}]}`,
		`{"choices":[{"delta":{"content":"lo"}}]}`,
		`[DONE]`,
	})
	defer srv.Close()

	var deltas []string
	c := New(srv.URL)
	res, err := c.Chat(context.Background(), ChatRequest{Model: "m"}, func(d string) { deltas = append(deltas, d) }, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Content != "Hello" {
		t.Errorf("Content = %q, want %q", res.Content, "Hello")
	}
	if len(res.ToolCalls) != 0 {
		t.Errorf("ToolCalls = %+v, want none", res.ToolCalls)
	}
	if len(deltas) != 2 || deltas[0] != "Hel" || deltas[1] != "lo" {
		t.Errorf("onDelta calls = %v, want [Hel lo]", deltas)
	}
}

// GIVEN an assistant message carrying a prior tool call (history being
// replayed back to the model on a later turn)
// WHEN toLlamacppMessage converts it to llama.cpp's wire shape
// THEN each tool_calls entry includes "type":"function" — llama-server's
// parser rejects a replayed message that omits it ("Missing tool call type"),
// a real failure hit testing against a live llama.cpp server.
func TestToLlamacppMessageIncludesToolCallType(t *testing.T) {
	msg := toLlamacppMessage(provider.Message{
		Role: "assistant",
		ToolCalls: []provider.ToolCall{
			{ID: "call_1", Name: "tree", Arguments: map[string]any{"path": "/repo"}},
		},
	})
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %+v, want 1 entry", msg.ToolCalls)
	}
	if msg.ToolCalls[0].Type != "function" {
		t.Fatalf("ToolCalls[0].Type = %q, want %q", msg.ToolCalls[0].Type, "function")
	}

	// Also assert at the wire level: the JSON llama-server actually receives
	// must contain "type":"function", not just the Go struct field.
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		ToolCalls []struct {
			Type string `json:"type"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.ToolCalls) != 1 || decoded.ToolCalls[0].Type != "function" {
		t.Fatalf("wire JSON tool_calls = %s, want type=function present", raw)
	}
}
