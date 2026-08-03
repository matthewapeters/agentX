package runtime

import (
	"context"
	"strings"
	"time"

	"agentx/internal/prompting"
	"agentx/internal/runtime/hooks"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// RunOptions is one RunPrompt call's per-call inputs — ADR 0013 Phase 2.
type RunOptions struct {
	Text string
	// RecordUserPrompt gates whether a USER_PROMPT event is published and the
	// user's text is recorded to history — false for the bootstrap prompt (see
	// runPrompt's existing doc comment in loop.go).
	RecordUserPrompt bool
	// Ephemeral marks published events as engaging the session but not part of the
	// user's conversation — still reaches the chat surface, omitted by read-only
	// observers like the context viewer.
	Ephemeral bool
}

// RunOutcome is what one RunPrompt call produced. A hard error is returned
// separately (RunPrompt's second return value), mirroring finishCycle's existing
// nil-on-success-or-interrupt split — Interrupted and a non-nil error never both
// describe the same call.
type RunOutcome struct {
	// Response is the final answer text, "" if the turn was interrupted.
	Response string
	// Interrupted reports a user interrupt during tool approval ended the turn
	// cleanly (not a failure) — RunPrompt returns a nil error alongside it.
	Interrupted bool
}

// ConversationCore is the prompt/tool/hook loop, extracted from Orchestrator per
// ADR 0013. Phase 2 moved runPrompt's own body here; Phase 3 moved
// runNativeToolCall/streamResponse/finishCycle/maxToolIterations natively onto
// Core too (core_tools.go, core_respond.go). toolSchemasFn and execTool remain
// permanent injected closures — both are entangled with plan_task, which ADR
// 0013 §"Explicitly not decided here" keeps off Core for good (Open Question 4).
// Everything else Settings-derived is read through a closure, never snapshotted
// at construction — several of the underlying fields are live-reloadable without
// a restart (Orchestrator.applyLiveSettings), and a one-time snapshot would
// silently stop tracking a live edit. See the Phase 3 behavior doc for the
// specific bug this corrects (Phase 2 had snapshotted thinkingEnabled).
type ConversationCore struct {
	assembler *prompting.Assembler
	hooks     *hooks.Registry
	events    EventSink
	convo     ContextStore
	approvals ApprovalSeeker

	registry *tools.Registry
	policy   *tools.Policy
	runner   ToolRunner

	model Model
	// modelName is a one-time snapshot, not a closure — safe because a Provider/
	// model change is a restart-required config key (Orchestrator.SetConfig's
	// restartRequiredKeys), and a restart rebuilds Core from scratch via Start ->
	// buildCore.
	modelName string

	// Settings read fresh on every call — see the type doc comment for why these
	// are closures rather than fields. thinkingEnabled/thinkingPromptText back
	// thinkingPrompt/RunPrompt's doThink; thinkingBudget backs streamResponse's
	// timeout; toolsEnabled backs toolsReady/toolSchemas; maxIterSetting backs
	// maxToolIterations. There is deliberately no toolReadOnly — read-only mode
	// was removed; approval-gating (Policy.Evaluate/RequestApproval) is the sole
	// execution gate now (docs/architecture/behavior/
	// tool_policy_read_only_removal.feature.md).
	thinkingEnabled    func() bool
	thinkingPromptText func() string
	thinkingBudget     func() time.Duration
	toolsEnabled       func() bool
	maxIterSetting     func() int

	// reportState mirrors Orchestrator.setProcessing's signature. Nil-safe
	// (report no-ops when unset) — deliberately a closure, not a fourth named
	// interface; see the Phase 2 behavior doc
	// (docs/architecture/behavior/adr/0013_conversationcore_runprompt.feature.md)
	// for why a committed ProcessingReporter interface is premature while ADR
	// 0013 Open Question 3 stays open.
	reportState func(state.RunState, state.Phase)

	// execTool runs one model-issued tool call, including plan_task interception
	// (bound to Orchestrator.runToolOrPlan). Permanent — see type doc comment.
	execTool func(ctx context.Context, call prompting.ToolCall) (string, *toolPin, error)

	// toolSchemasFn returns the full advertised tool list, including plan_task
	// when ready (bound to Orchestrator.availableToolSchemas, itself a thin
	// wrapper over Core's own native toolSchemas() plus plan_task — see
	// core_tools.go). Permanent — see type doc comment.
	toolSchemasFn func() []tools.ToolSchema
}

// report calls reportState if set; a nil reportState is a valid "no processing-
// state reporting" configuration (e.g. a future nested loop with nowhere to
// report to), not an error.
func (c *ConversationCore) report(s state.RunState, p state.Phase) {
	if c.reportState != nil {
		c.reportState(s, p)
	}
}

// thinkingPrompt returns the thinking guidance to fold into the respond system
// prompt, or "" when not thinking. Empty configured guidance uses the default —
// the same behavior as Orchestrator.thinkingPrompt (orchestrator.go:1196), moved
// here verbatim since it is pure and only reads Core's own config.
func (c *ConversationCore) thinkingPrompt(doThink bool) string {
	if !doThink {
		return ""
	}
	if p := strings.TrimSpace(c.thinkingPromptText()); p != "" {
		return p
	}
	return prompting.DefaultThinkingPrompt
}

// RunPrompt drives one prompt cycle as a flat loop: submit → LLM (advertising
// native tool schemas) → detect tool calls vs a chat response → execute
// tools/fold results back → loop, until the model answers with plain text.
// Hitting the per-turn tool-iteration budget (maxToolIterations) no longer
// hard-stops the turn — it pauses for a continue/stop decision
// (RequestToolLimitApproval); continuing resets the budget window and keeps
// looping under the same cap, asking again each time it's next exhausted
// (docs/architecture/behavior/tool_iteration_limit_approval.feature.md).
// This is runPrompt's body (internal/runtime/loop.go, pre-Phase-2), moved
// verbatim in control flow — direct o.bus/o.store/o.gate/o.mu/o.history
// reaches are replaced by c.events/c.convo and the injected closures above.
// The o.mu/started/accepting readiness check and refreshLiveFacts are
// deliberately NOT here — see the Phase 2 behavior doc for why they stay as
// Orchestrator-side pre-flight steps.
func (c *ConversationCore) RunPrompt(ctx context.Context, opts RunOptions) (RunOutcome, error) {
	var userOrd uint64
	if opts.RecordUserPrompt {
		userOrd = c.events.Publish("USER_PROMPT", state.ContentUserPrompt, map[string]any{"text": opts.Text}, opts.Ephemeral)
	}

	// Think once, before the first model call — not on every tool-round-trip
	// iteration, which would mean reasoning before every single tool result too.
	doThink := c.thinkingEnabled()
	turn := &hooks.Turn{
		Prompt:   opts.Text,
		Messages: c.convo.Augment(c.assembler.AssembleWithThinking(opts.Text, c.thinkingPrompt(doThink), "")),
	}
	fallback := c.convo.Augment(c.assembler.Assemble(opts.Text))

	if err := c.hooks.RunSync(ctx, turn); err != nil {
		c.report(state.StateFailed, state.PhaseNone)
		return RunOutcome{}, err
	}
	c.hooks.RunAsync(ctx, *turn)

	toolSchemas := c.toolSchemasFn()
	maxIter := c.maxToolIterations()
	var lastResp string
	var respOrd uint64
	var err error
	var pins []*toolPin

	// iter counts round-trips within the CURRENT budget window, reset to 0
	// every time the user approves continuing past maxIter — totalIter never
	// resets, purely for the prompt text ("reached N round-trips" reporting
	// the true depth of a turn that's been extended one or more times), and
	// firstCall (not iter) gates thinking, so a continuation reset never
	// re-triggers reasoning that should only happen once at the very start
	// of the turn (docs/architecture/behavior/
	// tool_iteration_limit_approval.feature.md).
	iter := 0
	totalIter := 0
	firstCall := true
	declinedToContinue := false

	for {
		if iter >= maxIter {
			cont, cerr := c.approvals.RequestToolLimitApproval(ctx, totalIter)
			if cerr != nil {
				// Interrupted while awaiting the continue/stop decision: end the
				// cycle cleanly, the same posture as an interrupted tool approval.
				c.report(state.StateCompleted, state.PhaseNone)
				return RunOutcome{Interrupted: true}, nil
			}
			if !cont {
				declinedToContinue = true
				break
			}
			iter = 0
		}
		iter++
		totalIter++

		var result ChatResult
		result, respOrd, err = c.streamResponse(ctx, turn.Messages, fallback, toolSchemas, doThink && firstCall, opts.Ephemeral)
		firstCall = false
		if err != nil {
			c.convo.Record(TurnRecord{Err: err, Record: opts.RecordUserPrompt, UserOrd: userOrd, UserText: opts.Text, RespOrd: respOrd, Pins: pins})
			return RunOutcome{}, c.finishCycle(err)
		}
		lastResp = result.Content
		if len(result.ToolCalls) == 0 {
			break // a chat response: the turn is done
		}

		turn.Messages = append(turn.Messages, prompting.Message{
			Role: "assistant", Content: result.Content, ToolCalls: result.ToolCalls,
		})
		for _, call := range result.ToolCalls {
			resultText, pin, terr := c.execTool(ctx, call)
			if terr != nil {
				// Interrupted while awaiting approval: end the cycle cleanly.
				c.report(state.StateCompleted, state.PhaseNone)
				return RunOutcome{Interrupted: true}, nil
			}
			if pin != nil {
				pins = append(pins, pin)
			}
			turn.Messages = append(turn.Messages, prompting.Message{
				Role: "tool", Content: resultText, ToolCallID: call.ID,
			})
		}

		if err = c.hooks.RunSync(ctx, turn); err != nil {
			c.report(state.StateFailed, state.PhaseNone)
			return RunOutcome{}, err
		}
		c.hooks.RunAsync(ctx, *turn)
	}

	if lastResp == "" && err == nil {
		// The tool-iteration budget was hit and the user declined to continue
		// (or, in principle, some other path left both empty) — never leave
		// the user with total silence.
		if declinedToContinue {
			lastResp = "[stopped: declined to continue past the tool-call limit for this turn]"
		} else {
			lastResp = "[stopped: reached the tool-call limit for this turn without a final answer]"
		}
		respOrd = c.events.Publish("AGENT_RESPONSE", state.ContentAgentResponse, map[string]any{"text": lastResp}, opts.Ephemeral)
	}
	c.convo.Record(TurnRecord{Err: err, Record: opts.RecordUserPrompt, UserOrd: userOrd, UserText: opts.Text, RespOrd: respOrd, Response: lastResp, Pins: pins})
	return RunOutcome{Response: lastResp}, c.finishCycle(err)
}

// buildCore constructs o.core from pieces Start has already built (o.assembler,
// o.hooks, o.registry/o.policy/o.runner, o.model) plus closures for
// Settings-derived values (never snapshotted — see the type doc comment) and the
// two permanent closures (execTool, toolSchemasFn). o satisfies EventSink,
// ContextStore, and ApprovalSeeker itself (Phase 1's Publish/Augment/Record;
// RequestApproval/RequestOutputSizeDecision unchanged since Phase 1). Called
// under o.mu during Start, after o.hooks is built.
func (o *Orchestrator) buildCore() {
	o.core = &ConversationCore{
		assembler: o.assembler,
		hooks:     o.hooks,
		events:    o,
		convo:     o,
		approvals: o,

		registry: o.registry,
		policy:   o.policy,
		runner:   o.runner,

		model:     o.model,
		modelName: o.modelName(),

		thinkingEnabled:    func() bool { return o.settings.ThinkingEnabled },
		thinkingPromptText: func() string { return o.settings.ThinkingPrompt },
		thinkingBudget:     func() time.Duration { return o.settings.ThinkingBudget },
		toolsEnabled:       func() bool { return o.settings.ToolsEnabled },
		maxIterSetting:     func() int { return o.settings.MaxToolIterationsPerTurn },

		reportState:   o.setProcessing,
		execTool:      o.runToolOrPlan,
		toolSchemasFn: o.availableToolSchemas,
	}
}
