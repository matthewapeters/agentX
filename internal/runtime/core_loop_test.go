package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentx/internal/prompting"
	"agentx/internal/runtime/hooks"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// fakeEventSink and fakeContextStore back the ConversationCore.RunPrompt tests
// below directly — no Orchestrator, no bus, no session store, proving Phase 2's
// extraction actually decoupled the loop the way ADR 0013 intends.
type fakeEventSink struct {
	nextOrd   uint64
	published []string // eventType per Publish/PublishTool call, in order
}

func (f *fakeEventSink) Publish(eventType string, _ state.ContentType, _ any, _ bool) uint64 {
	f.nextOrd++
	f.published = append(f.published, eventType)
	return f.nextOrd
}

func (f *fakeEventSink) PublishTool(evt ToolEvent) uint64 {
	f.nextOrd++
	f.published = append(f.published, evt.EventType)
	return f.nextOrd
}

type fakeContextStore struct {
	recorded []TurnRecord
}

func (f *fakeContextStore) Augment(base []prompting.Message) []prompting.Message { return base }
func (f *fakeContextStore) Record(entry TurnRecord)                              { f.recorded = append(f.recorded, entry) }

// stubModel backs ConversationCore.streamResponse in these tests (RunPrompt no
// longer takes a streamFn closure as of Phase 3 — streamResponse is native and
// calls c.model.Chat directly).
type stubModel struct {
	chatFn func(ctx context.Context, model string, messages []prompting.Message, toolSchemas []tools.ToolSchema, onDelta, onThink func(string)) (ChatResult, error)
}

func (s stubModel) Chat(ctx context.Context, model string, messages []prompting.Message, toolSchemas []tools.ToolSchema, onDelta, onThink func(string)) (ChatResult, error) {
	return s.chatFn(ctx, model, messages, toolSchemas, onDelta, onThink)
}
func (s stubModel) Ready(context.Context, string) error                { return nil }
func (s stubModel) ContextLength(context.Context, string) (int, error) { return 0, nil }

// countingHook fails on its failAt'th call (1-indexed); failAt == 0 never fails.
type countingHook struct {
	calls  *int
	failAt int
	err    error
}

func (h countingHook) Run(_ context.Context, _ *hooks.Turn) error {
	*h.calls++
	if h.failAt != 0 && *h.calls == h.failAt {
		return h.err
	}
	return nil
}

// newTestCore builds a ConversationCore with no-op/trivial defaults, letting each
// test override just what it cares about. execTool is stubbed directly (bypassing
// runNativeToolCall entirely), so approvals/registry/policy/runner are left nil —
// core_tools_test.go covers runNativeToolCall itself.
func newTestCore() (*ConversationCore, *fakeEventSink, *fakeContextStore) {
	events := &fakeEventSink{}
	convo := &fakeContextStore{}
	c := &ConversationCore{
		assembler: prompting.New(""),
		hooks:     hooks.NewRegistry(),
		events:    events,
		convo:     convo,
		execTool: func(context.Context, prompting.ToolCall) (string, *toolPin, error) {
			return "", nil, nil
		},
		model: stubModel{chatFn: func(context.Context, string, []prompting.Message, []tools.ToolSchema, func(string), func(string)) (ChatResult, error) {
			return ChatResult{Content: "default response"}, nil
		}},
		modelName:          "test-model",
		thinkingEnabled:    func() bool { return false },
		thinkingPromptText: func() string { return "" },
		thinkingBudget:     func() time.Duration { return 0 },
		toolSchemasFn:      func() []tools.ToolSchema { return nil },
		maxIterSetting:     func() int { return 25 },
	}
	return c, events, convo
}

// GIVEN a model call that returns plain text with no tool calls
// WHEN RunPrompt runs
// THEN it returns that text as RunOutcome.Response, publishes USER_PROMPT then
// AGENT_RESPONSE (via streamResponse), and records exactly one TurnRecord
// carrying the response.
func TestConversationCoreRunPromptChatResponse(t *testing.T) {
	c, events, convo := newTestCore()
	c.model = stubModel{chatFn: func(context.Context, string, []prompting.Message, []tools.ToolSchema, func(string), func(string)) (ChatResult, error) {
		return ChatResult{Content: "hi there"}, nil
	}}

	outcome, err := c.RunPrompt(context.Background(), RunOptions{Text: "hello", RecordUserPrompt: true})
	if err != nil {
		t.Fatalf("RunPrompt error: %v", err)
	}
	if outcome.Response != "hi there" || outcome.Interrupted {
		t.Fatalf("outcome = %+v, want Response=%q Interrupted=false", outcome, "hi there")
	}
	wantEvents := []string{"USER_PROMPT", "AGENT_RESPONSE"}
	if len(events.published) != len(wantEvents) || events.published[0] != wantEvents[0] || events.published[1] != wantEvents[1] {
		t.Fatalf("published events = %v, want %v", events.published, wantEvents)
	}
	if len(convo.recorded) != 1 || convo.recorded[0].Response != "hi there" {
		t.Fatalf("recorded = %+v, want one entry with Response=%q", convo.recorded, "hi there")
	}
}

// GIVEN a model call that first issues a tool call, then answers with text
// WHEN RunPrompt runs
// THEN it executes the tool call via execTool, loops once more, and returns the
// second call's text as the final response.
func TestConversationCoreRunPromptToolCallThenChatResponse(t *testing.T) {
	c, _, convo := newTestCore()
	var streamCalls int
	c.model = stubModel{chatFn: func(context.Context, string, []prompting.Message, []tools.ToolSchema, func(string), func(string)) (ChatResult, error) {
		streamCalls++
		if streamCalls == 1 {
			return ChatResult{ToolCalls: []prompting.ToolCall{{ID: "call-1", Name: "list_dir"}}}, nil
		}
		return ChatResult{Content: "done"}, nil
	}}
	var execCalls int
	c.execTool = func(_ context.Context, call prompting.ToolCall) (string, *toolPin, error) {
		execCalls++
		if call.ID != "call-1" {
			t.Errorf("execTool called with %+v, want call-1", call)
		}
		return "file1\nfile2", nil, nil
	}

	outcome, err := c.RunPrompt(context.Background(), RunOptions{Text: "list files"})
	if err != nil {
		t.Fatalf("RunPrompt error: %v", err)
	}
	if streamCalls != 2 {
		t.Errorf("model.Chat called %d times, want 2", streamCalls)
	}
	if execCalls != 1 {
		t.Errorf("execTool called %d times, want 1", execCalls)
	}
	if outcome.Response != "done" {
		t.Fatalf("outcome.Response = %q, want %q", outcome.Response, "done")
	}
	if len(convo.recorded) != 1 || len(convo.recorded[0].Pins) != 0 {
		// execTool returned a nil pin, so no pin should propagate.
		t.Errorf("recorded = %+v, want one entry with no pins", convo.recorded)
	}
}

// GIVEN a sync hook that fails on the loop's first hook point (right after the
// prompt is assembled, before any model call)
// WHEN RunPrompt runs
// THEN it returns that error immediately, never calling the model.
func TestConversationCoreRunPromptHookFailureAtFirstPointAborts(t *testing.T) {
	c, _, _ := newTestCore()
	wantErr := errors.New("hook boom")
	var hookCalls int
	c.hooks = hooks.NewRegistry()
	c.hooks.RegisterSync(countingHook{calls: &hookCalls, failAt: 1, err: wantErr})

	var streamCalls int
	c.model = stubModel{chatFn: func(context.Context, string, []prompting.Message, []tools.ToolSchema, func(string), func(string)) (ChatResult, error) {
		streamCalls++
		return ChatResult{Content: "should not be reached"}, nil
	}}

	outcome, err := c.RunPrompt(context.Background(), RunOptions{Text: "hello"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunPrompt error = %v, want %v", err, wantErr)
	}
	if outcome != (RunOutcome{}) {
		t.Errorf("outcome = %+v, want zero value", outcome)
	}
	if streamCalls != 0 {
		t.Errorf("model.Chat called %d times, want 0 (hook chain must abort before any model call)", streamCalls)
	}
}

// GIVEN a sync hook that succeeds on the loop's first hook point but fails on
// the second (after one round of tool execution)
// WHEN RunPrompt runs
// THEN the tool call still executes once, but RunPrompt returns the hook error
// afterward and never issues a second model call.
func TestConversationCoreRunPromptHookFailureMidLoopAborts(t *testing.T) {
	c, _, _ := newTestCore()
	wantErr := errors.New("mid-loop hook boom")
	var hookCalls int
	c.hooks = hooks.NewRegistry()
	c.hooks.RegisterSync(countingHook{calls: &hookCalls, failAt: 2, err: wantErr})

	var streamCalls int
	c.model = stubModel{chatFn: func(context.Context, string, []prompting.Message, []tools.ToolSchema, func(string), func(string)) (ChatResult, error) {
		streamCalls++
		return ChatResult{ToolCalls: []prompting.ToolCall{{ID: "call-1", Name: "list_dir"}}}, nil
	}}
	var execCalls int
	c.execTool = func(context.Context, prompting.ToolCall) (string, *toolPin, error) {
		execCalls++
		return "ok", nil, nil
	}

	outcome, err := c.RunPrompt(context.Background(), RunOptions{Text: "hello"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunPrompt error = %v, want %v", err, wantErr)
	}
	if outcome != (RunOutcome{}) {
		t.Errorf("outcome = %+v, want zero value", outcome)
	}
	if streamCalls != 1 {
		t.Errorf("model.Chat called %d times, want 1 (loop must not continue past the mid-loop hook failure)", streamCalls)
	}
	if execCalls != 1 {
		t.Errorf("execTool called %d times, want 1", execCalls)
	}
}

// GIVEN execTool reports an interrupt (a non-nil error, e.g. the user aborted
// while an approval prompt was pending)
// WHEN RunPrompt handles that tool call
// THEN it returns RunOutcome{Interrupted: true} and a nil error — matching
// finishCycle's existing nil-on-interrupt contract — and report is invoked
// exactly once (proving finishCycle, which would report again, never runs on
// this path).
func TestConversationCoreRunPromptInterruptDuringToolApproval(t *testing.T) {
	c, _, _ := newTestCore()
	c.model = stubModel{chatFn: func(context.Context, string, []prompting.Message, []tools.ToolSchema, func(string), func(string)) (ChatResult, error) {
		return ChatResult{ToolCalls: []prompting.ToolCall{{ID: "call-1", Name: "rm"}}}, nil
	}}
	c.execTool = func(context.Context, prompting.ToolCall) (string, *toolPin, error) {
		return "", nil, errors.New("interrupted while awaiting approval")
	}

	var reported []state.RunState
	c.reportState = func(s state.RunState, _ state.Phase) { reported = append(reported, s) }

	outcome, err := c.RunPrompt(context.Background(), RunOptions{Text: "delete everything"})
	if err != nil {
		t.Fatalf("RunPrompt error = %v, want nil", err)
	}
	if !outcome.Interrupted || outcome.Response != "" {
		t.Fatalf("outcome = %+v, want Interrupted=true Response=\"\"", outcome)
	}
	// streamResponse itself reports StateWorking before the model call; the
	// interrupt path then reports StateCompleted once more. A third entry would
	// mean finishCycle also ran, which it must not on this path.
	if len(reported) != 2 || reported[len(reported)-1] != state.StateCompleted {
		t.Errorf("reported states = %v, want exactly 2 entries ending in StateCompleted", reported)
	}
}

// GIVEN the model keeps issuing tool calls past the tool-iteration budget,
// never answering with plain text
// WHEN RunPrompt's loop exhausts maxIterSetting's budget
// THEN it publishes and returns the fixed "[stopped: ...]" fallback text rather
// than leaving the caller with an empty response.
func TestConversationCoreRunPromptBudgetExhausted(t *testing.T) {
	c, events, convo := newTestCore()
	c.maxIterSetting = func() int { return 2 }
	c.model = stubModel{chatFn: func(context.Context, string, []prompting.Message, []tools.ToolSchema, func(string), func(string)) (ChatResult, error) {
		return ChatResult{ToolCalls: []prompting.ToolCall{{ID: "call-1", Name: "list_dir"}}}, nil
	}}
	c.execTool = func(context.Context, prompting.ToolCall) (string, *toolPin, error) {
		return "ok", nil, nil
	}

	outcome, err := c.RunPrompt(context.Background(), RunOptions{Text: "keep going forever"})
	if err != nil {
		t.Fatalf("RunPrompt error: %v", err)
	}
	const want = "[stopped: reached the tool-call limit for this turn without a final answer]"
	if outcome.Response != want {
		t.Fatalf("outcome.Response = %q, want %q", outcome.Response, want)
	}
	found := false
	for _, ev := range events.published {
		if ev == "AGENT_RESPONSE" {
			found = true
		}
	}
	if !found {
		t.Errorf("published events = %v, want an AGENT_RESPONSE for the fallback text", events.published)
	}
	if len(convo.recorded) != 1 || convo.recorded[0].Response != want {
		t.Errorf("recorded = %+v, want one entry with the fallback Response", convo.recorded)
	}
}
