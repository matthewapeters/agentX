package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"agentx/internal/prompting"
	"agentx/internal/runtime/hooks"
	"agentx/internal/tools"
)

// standaloneCore builds a ConversationCore wired entirely to itself — execTool
// and toolSchemasFn point at the instance's own runNativeToolCall/toolSchemas,
// not an Orchestrator-bound closure — proving the shape a future minimal,
// plan_task-free consumer would use (ADR 0013 Phase 5). No Orchestrator, no
// session store, no transport anywhere in the dependency graph.
func standaloneCore(hookRan *int, chatFn func(callN int) (ChatResult, error)) (*ConversationCore, *fakeEventSink, *fakeContextStore, *stubToolRunner, *stubApprovalSeeker) {
	events := &fakeEventSink{}
	convo := &fakeContextStore{}
	runner := &stubToolRunner{result: tools.Result{ToolID: "write_file", Status: "ok", Preview: "wrote 12 bytes"}}
	approvals := &stubApprovalSeeker{verdict: tools.Verdict{Decision: tools.Allow}}

	c := &ConversationCore{
		assembler: prompting.New(""),
		hooks:     hooks.NewRegistry(),
		events:    events,
		convo:     convo,
		approvals: approvals,

		registry: tools.DefaultRegistry(),
		policy:   tools.NewPolicy(),
		runner:   runner,

		modelName:          "standalone-model",
		thinkingEnabled:    func() bool { return false },
		thinkingPromptText: func() string { return "" },
		thinkingBudget:     func() time.Duration { return 0 },
		toolsEnabled:       func() bool { return true },
		toolReadOnly:       func() bool { return false },
		maxIterSetting:     func() int { return 10 },
	}
	c.hooks.RegisterSync(countingHook{calls: hookRan})

	var callN int
	c.model = stubModel{chatFn: func(context.Context, string, []prompting.Message, []tools.ToolSchema, func(string), func(string)) (ChatResult, error) {
		callN++
		return chatFn(callN)
	}}

	// Self-wired, not Orchestrator-bound — the point of this phase.
	c.execTool = c.runNativeToolCall
	c.toolSchemasFn = c.toolSchemas

	return c, events, convo, runner, approvals
}

// GIVEN a ConversationCore wired entirely to itself (execTool/toolSchemasFn point
// at its own runNativeToolCall/toolSchemas, not an Orchestrator closure), with no
// Orchestrator, session store, or transport anywhere in its dependency graph
// WHEN RunPrompt drives a turn that issues an approval-gated tool call, then
// answers with final text
// THEN the sync hook fires, the approval seeker is consulted, the tool actually
// runs, and the full event sequence (USER_PROMPT, TOOL_CALL, TOOL_RESULT,
// AGENT_RESPONSE) publishes in order — the complete loop, standalone.
func TestConversationCoreRunsStandaloneEndToEnd(t *testing.T) {
	var hookRan int
	c, events, convo, runner, approvals := standaloneCore(&hookRan, func(callN int) (ChatResult, error) {
		if callN == 1 {
			return ChatResult{ToolCalls: []prompting.ToolCall{
				{ID: "call-1", Name: "write_file", Arguments: map[string]any{"path": "out.txt", "content": "hi"}},
			}}, nil
		}
		return ChatResult{Content: "done writing the file"}, nil
	})

	outcome, err := c.RunPrompt(context.Background(), RunOptions{Text: "write a file", RecordUserPrompt: true})
	if err != nil {
		t.Fatalf("RunPrompt error: %v", err)
	}
	if outcome.Response != "done writing the file" || outcome.Interrupted {
		t.Fatalf("outcome = %+v, want the final chat response", outcome)
	}
	if hookRan != 2 {
		t.Errorf("sync hook ran %d times, want 2 (once before the model call, once after tool execution)", hookRan)
	}
	if approvals.approveCalls != 1 {
		t.Errorf("RequestApproval called %d times, want 1 (write_file requires approval)", approvals.approveCalls)
	}
	if runner.calls != 1 {
		t.Errorf("runner.Run called %d times, want 1", runner.calls)
	}
	// No TOOL_CALL event: runNativeToolCall only calls publishToolCall on the
	// no-approval-needed path (approval.go's original doc comment, preserved by
	// Phase 3) — an approval-gated call's audit trail is the approval
	// request/decision exchange instead (toolPin's doc comment: "the
	// approval-gated path publishes no separate tool_call widget"). Our stub
	// approvals seeker doesn't publish those, so only TOOL_RESULT shows here.
	wantEvents := []string{"USER_PROMPT", "TOOL_RESULT", "AGENT_RESPONSE"}
	if len(events.published) != len(wantEvents) {
		t.Fatalf("published = %v, want %v", events.published, wantEvents)
	}
	for i, want := range wantEvents {
		if events.published[i] != want {
			t.Errorf("published[%d] = %q, want %q (full sequence: %v)", i, events.published[i], want, events.published)
		}
	}
	if len(convo.recorded) != 1 || len(convo.recorded[0].Pins) != 1 {
		t.Errorf("recorded = %+v, want one entry with one pin (the write_file call/result)", convo.recorded)
	}
}

// GIVEN two independently-constructed ConversationCore instances, each with its
// own fakes and no shared pointers between them
// WHEN both run a turn concurrently
// THEN neither instance's recorded events/history reflect the other's run — the
// concrete version of ADR 0013's founding motivation: a second, nested loop must
// not share mutable state with the first. Run with -race to catch what assertions
// alone can't.
func TestTwoConversationCoresRunConcurrentlyWithoutInterference(t *testing.T) {
	var hookRanA, hookRanB int
	coreA, eventsA, convoA, _, _ := standaloneCore(&hookRanA, func(int) (ChatResult, error) {
		return ChatResult{Content: "answer from A"}, nil
	})
	coreB, eventsB, convoB, _, _ := standaloneCore(&hookRanB, func(int) (ChatResult, error) {
		return ChatResult{Content: "answer from B"}, nil
	})

	var wg sync.WaitGroup
	var outcomeA, outcomeB RunOutcome
	var errA, errB error
	wg.Add(2)
	go func() {
		defer wg.Done()
		outcomeA, errA = coreA.RunPrompt(context.Background(), RunOptions{Text: "prompt A", RecordUserPrompt: true})
	}()
	go func() {
		defer wg.Done()
		outcomeB, errB = coreB.RunPrompt(context.Background(), RunOptions{Text: "prompt B", RecordUserPrompt: true})
	}()
	wg.Wait()

	if errA != nil || errB != nil {
		t.Fatalf("RunPrompt errors: A=%v B=%v", errA, errB)
	}
	if outcomeA.Response != "answer from A" {
		t.Errorf("outcomeA.Response = %q, want %q", outcomeA.Response, "answer from A")
	}
	if outcomeB.Response != "answer from B" {
		t.Errorf("outcomeB.Response = %q, want %q", outcomeB.Response, "answer from B")
	}
	if len(convoA.recorded) != 1 || convoA.recorded[0].Response != "answer from A" {
		t.Errorf("convoA.recorded = %+v, want exactly A's own turn", convoA.recorded)
	}
	if len(convoB.recorded) != 1 || convoB.recorded[0].Response != "answer from B" {
		t.Errorf("convoB.recorded = %+v, want exactly B's own turn", convoB.recorded)
	}
	if len(eventsA.published) != 2 || len(eventsB.published) != 2 {
		t.Errorf("published event counts = A:%v B:%v, want 2 each (USER_PROMPT, AGENT_RESPONSE) with no cross-contamination", eventsA.published, eventsB.published)
	}
}
