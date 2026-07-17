package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentx/internal/llm/ollama"
	"agentx/internal/prompting/task"
)

func TestWavefrontPlanContextExcludesRoot(t *testing.T) {
	nodes := []task.Record{
		{ID: "root", Goal: "review the project", Status: task.Done, Value: "final answer"},
	}
	got := wavefrontPlanContext("root", nodes)
	if got != "" {
		t.Errorf("wavefrontPlanContext = %q, want empty (root excluded, nothing else resolved)", got)
	}
}

func TestWavefrontPlanContextIncludesKindStepAndKindTask(t *testing.T) {
	nodes := []task.Record{
		{ID: "root", Goal: "review the project", Status: task.Proposed},
		{ID: "root-1", Goal: "read README.md", Kind: task.KindTask, Status: task.Done, Value: "a demo project"},
		{ID: "root-2", Goal: "what language is used", Kind: task.KindStep, Status: task.Done, Value: "Go"},
	}
	got := wavefrontPlanContext("root", nodes)
	if !strings.Contains(got, "read README.md") || !strings.Contains(got, "a demo project") {
		t.Errorf("missing the KindTask finding: %q", got)
	}
	if !strings.Contains(got, "what language is used") || !strings.Contains(got, "Go") {
		t.Errorf("missing the KindStep finding: %q", got)
	}
	if strings.Contains(got, "review the project") {
		t.Errorf("root leaked into the findings: %q", got)
	}
}

func TestWavefrontPlanContextShowsErrorForFailedNodes(t *testing.T) {
	nodes := []task.Record{
		{ID: "root", Goal: "review", Status: task.Proposed},
		{ID: "root-1", Goal: "read missing.md", Kind: task.KindTask, Status: task.Failed, Error: "file not found"},
	}
	got := wavefrontPlanContext("root", nodes)
	if !strings.Contains(got, "file not found") {
		t.Errorf("missing the failure reason: %q", got)
	}
}

func TestWavefrontPlanContextEmptyWhenNothingResolved(t *testing.T) {
	nodes := []task.Record{
		{ID: "root", Goal: "review", Status: task.Proposed},
		{ID: "root-1", Goal: "still working", Kind: task.KindStep, Status: task.Proposed},
	}
	got := wavefrontPlanContext("root", nodes)
	if got != "" {
		t.Errorf("wavefrontPlanContext = %q, want empty (nothing resolved yet)", got)
	}
}

// TestNewCompleteChatPassesFormatThrough: a regression guard against accidentally
// hard-coding a schema into the shared closure — it must pass format straight
// through, present or absent, for both the classifier's constrained calls and the
// scheduler's schema-free synthesis calls to work correctly off the same closure.
func TestNewCompleteChatPassesFormatThrough(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"ok"}}`))
	}))
	defer srv.Close()

	client := ollama.New(srv.URL)
	chat := newCompleteChat(client, "test-model")

	if _, err := chat(context.Background(), "sys", "usr", json.RawMessage(`{"type":"object"}`)); err != nil {
		t.Fatalf("chat with format: %v", err)
	}
	if _, ok := gotBody["format"]; !ok {
		t.Error("format was not passed through to the request when supplied")
	}

	gotBody = nil // json.Unmarshal merges into an existing map rather than resetting
	// it, so a stale "format" key from the first call would otherwise survive here
	// regardless of what this second request actually contains.
	if _, err := chat(context.Background(), "sys", "usr", nil); err != nil {
		t.Fatalf("chat without format: %v", err)
	}
	if _, ok := gotBody["format"]; ok {
		t.Error("format leaked into the request when the caller passed nil")
	}
}
