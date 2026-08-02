package runtime

import (
	"context"
	"errors"
	"testing"

	"agentx/internal/prompting"
	"agentx/internal/tools"
)

// toolCall builds a native tool-call value for these tests.
func toolCall(name string, args map[string]any) prompting.ToolCall {
	return prompting.ToolCall{ID: "call-1", Name: name, Arguments: args}
}

// stubToolRunner records every Run call and returns a scripted result.
type stubToolRunner struct {
	result tools.Result
	err    error
	calls  int
}

func (s *stubToolRunner) Run(_ context.Context, _ tools.Descriptor, _ map[string]string) (tools.Result, error) {
	s.calls++
	return s.result, s.err
}

// stubApprovalSeeker scripts RequestApproval/RequestOutputSizeDecision and
// records how many times each was called.
type stubApprovalSeeker struct {
	verdict      tools.Verdict
	approveErr   error
	approveCalls int
}

func (s *stubApprovalSeeker) RequestApproval(context.Context, tools.Descriptor, map[string]string, *tools.Policy) (tools.Verdict, error) {
	s.approveCalls++
	return s.verdict, s.approveErr
}

func (s *stubApprovalSeeker) RequestOutputSizeDecision(_ context.Context, _ tools.Descriptor, _ map[string]string, res tools.Result) (tools.Result, bool, error) {
	return res, true, nil
}

// newTestToolCore builds a ConversationCore wired for runNativeToolCall/
// toolSchemas/toolsReady tests only — no assembler/hooks/model, since those
// aren't reached by these methods.
func newTestToolCore() (*ConversationCore, *fakeEventSink, *stubToolRunner, *stubApprovalSeeker) {
	events := &fakeEventSink{}
	runner := &stubToolRunner{result: tools.Result{ToolID: "list_dir", Status: "ok", Preview: "a\nb"}}
	approvals := &stubApprovalSeeker{verdict: tools.Verdict{Decision: tools.Allow}}
	c := &ConversationCore{
		events:       events,
		approvals:    approvals,
		registry:     tools.DefaultRegistry(),
		policy:       tools.NewPolicy(),
		runner:       runner,
		toolsEnabled: func() bool { return true },
	}
	return c, events, runner, approvals
}

// GIVEN toolsEnabled/runner/policy/registry are all present
// WHEN toolsReady is checked
// THEN it reports true; missing any one of the four reports false.
func TestConversationCoreToolsReady(t *testing.T) {
	c, _, _, _ := newTestToolCore()
	if !c.toolsReady() {
		t.Fatal("toolsReady() = false, want true when all four dependencies are present")
	}

	c2, _, _, _ := newTestToolCore()
	c2.toolsEnabled = func() bool { return false }
	if c2.toolsReady() {
		t.Error("toolsReady() = true with toolsEnabled() false, want false")
	}

	c3, _, _, _ := newTestToolCore()
	c3.runner = nil
	if c3.toolsReady() {
		t.Error("toolsReady() = true with runner nil, want false")
	}
}

// GIVEN tools are ready
// WHEN toolSchemas is called
// THEN it returns the registry's full catalog — and never includes plan_task
// (that's Orchestrator's job).
func TestConversationCoreToolSchemas(t *testing.T) {
	c, _, _, _ := newTestToolCore()
	schemas := c.toolSchemas()
	if len(schemas) == 0 {
		t.Fatal("toolSchemas() returned none, want the default registry's catalog")
	}
	for _, s := range schemas {
		if s.Name == planTaskToolName {
			t.Errorf("toolSchemas() included %q — plan_task must stay Orchestrator-only", planTaskToolName)
		}
	}

	c.toolsEnabled = func() bool { return false }
	if got := c.toolSchemas(); got != nil {
		t.Errorf("toolSchemas() with tools not ready = %v, want nil", got)
	}
}

// GIVEN a read-only tool call under a permissive policy
// WHEN runNativeToolCall runs
// THEN it executes directly (no approval needed), publishes a tool_call and a
// tool_result event, and returns the rendered result with a populated pin.
func TestConversationCoreRunNativeToolCallExecutesDirectly(t *testing.T) {
	c, events, runner, approvals := newTestToolCore()

	text, pin, err := c.runNativeToolCall(context.Background(), toolCall("list_dir", map[string]any{"path": "."}))
	if err != nil {
		t.Fatalf("runNativeToolCall error: %v", err)
	}
	if runner.calls != 1 {
		t.Errorf("runner.Run called %d times, want 1", runner.calls)
	}
	if approvals.approveCalls != 0 {
		t.Errorf("RequestApproval called %d times, want 0 (read tool, permissive policy)", approvals.approveCalls)
	}
	if pin == nil || pin.callOrdinal == 0 || pin.resultOrdinal == 0 {
		t.Fatalf("pin = %+v, want both ordinals populated", pin)
	}
	wantEvents := []string{"TOOL_CALL", "TOOL_RESULT"}
	if len(events.published) != len(wantEvents) || events.published[0] != wantEvents[0] || events.published[1] != wantEvents[1] {
		t.Fatalf("published = %v, want %v", events.published, wantEvents)
	}
	if text == "" {
		t.Error("runNativeToolCall returned empty result text")
	}
}

// GIVEN a tool call the policy marks NeedsApproval
// WHEN runNativeToolCall runs
// THEN it calls approvals.RequestApproval and, on approval, still executes the
// tool.
func TestConversationCoreRunNativeToolCallRequestsApproval(t *testing.T) {
	c, _, runner, approvals := newTestToolCore()
	// write_file is RequiresApproval: true in the default registry — exercises
	// the real policy path, no synthetic rule needed.
	approvals.verdict = tools.Verdict{Decision: tools.Allow}

	_, _, err := c.runNativeToolCall(context.Background(), toolCall("write_file", map[string]any{"path": "x.txt", "content": "hi"}))
	if err != nil {
		t.Fatalf("runNativeToolCall error: %v", err)
	}
	if approvals.approveCalls != 1 {
		t.Errorf("RequestApproval called %d times, want 1", approvals.approveCalls)
	}
	if runner.calls != 1 {
		t.Errorf("runner.Run called %d times, want 1 (approved)", runner.calls)
	}
}

// GIVEN RequestApproval returns an error (the user interrupted while awaiting
// approval)
// WHEN runNativeToolCall runs
// THEN it propagates that error without running the tool.
func TestConversationCoreRunNativeToolCallPropagatesApprovalInterrupt(t *testing.T) {
	c, _, runner, approvals := newTestToolCore()
	wantErr := errors.New("interrupted")
	approvals.approveErr = wantErr

	_, _, err := c.runNativeToolCall(context.Background(), toolCall("write_file", map[string]any{"path": "x.txt", "content": "hi"}))
	if !errors.Is(err, wantErr) {
		t.Fatalf("runNativeToolCall error = %v, want %v", err, wantErr)
	}
	if runner.calls != 0 {
		t.Errorf("runner.Run called %d times, want 0 (never reached after interrupt)", runner.calls)
	}
}
