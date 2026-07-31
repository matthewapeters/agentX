package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"agentx/internal/prompting"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// buildTools constructs the registry, policy, executor, and output-overrides
// collaborators the native tool-calling loop and plan_task both depend on.
// Called under o.mu during Start when tools are enabled.
func (o *Orchestrator) buildTools() error {
	if o.registry == nil {
		o.registry = tools.DefaultRegistry()
	}
	if o.policy == nil {
		blacklist, err := tools.LoadBlacklist(o.settings.ToolBlacklistPath)
		if err != nil {
			return err
		}
		o.policy = tools.NewPolicy(blacklist...)
		approvals, err := tools.LoadApprovals(o.settings.ToolApprovalsPath)
		if err != nil {
			return err
		}
		o.policy.LoadGlobal(approvals)
	}
	if o.runner == nil {
		art, err := o.store.Artifacts(o.id.ID)
		if err != nil {
			return fmt.Errorf("tool artifacts: %w", err)
		}
		o.runner = tools.NewExecutor(art, o.settings.ToolOutputMaxBytes)
	}
	if o.outputOverrides == nil {
		overrides, err := tools.LoadOutputOverrides(o.settings.ToolOutputOverridesPath)
		if err != nil {
			return err
		}
		o.outputOverrides = tools.NewOutputOverrides(overrides)
	}
	return nil
}

// toolsReady reports whether native tool-calling can advertise/execute tools.
func (o *Orchestrator) toolsReady() bool {
	return o.settings.ToolsEnabled && o.runner != nil && o.policy != nil && o.registry != nil
}

// availableToolSchemas returns every tool schema this turn should advertise to
// the model: the subprocess/builtin catalog (filtered by ToolReadOnly, same as
// the old single_tool cycle's tier gate), plus plan_task when the decomposition
// substrate is wired.
func (o *Orchestrator) availableToolSchemas() []tools.ToolSchema {
	var out []tools.ToolSchema
	if o.toolsReady() {
		out = append(out, o.registry.ToolSchemas(o.settings.ToolReadOnly)...)
	}
	if o.planReady() {
		out = append(out, planTaskSchema())
	}
	return out
}

// maxToolIterations bounds how many native tool-call round-trips one turn may
// run before the loop stops and answers with whatever it has.
func (o *Orchestrator) maxToolIterations() int {
	if o.settings.MaxToolIterationsPerTurn > 0 {
		return o.settings.MaxToolIterationsPerTurn
	}
	return 25
}

// toolPin carries the ordinals and rendered text of the tool_call/tool_result
// events one native tool call published, so recordTurn can register them as
// toggleable context-history entries (initially disabled, like their
// checkbox) alongside the turn's user/assistant elements. An entry with
// ordinal 0 was never published (e.g. the approval-gated path publishes no
// separate 🔧 tool_call widget — the approval request/decision audit trail
// stands in for it) and is skipped by recordTurn.
type toolPin struct {
	callOrdinal   uint64
	callText      string
	resultOrdinal uint64
	resultText    string
}

// runNativeToolCall executes one model-issued native tool call: looks it up in
// the registry, evaluates policy (prompting for approval when required), runs
// it, and publishes tool_call/tool_result. Returns the tool-result text to
// fold back into the turn's messages and the pinnable ordinals for that
// call/result. A non-nil error means the user interrupted while awaiting
// approval or an output-size decision.
func (o *Orchestrator) runNativeToolCall(ctx context.Context, call prompting.ToolCall) (string, *toolPin, error) {
	o.setProcessing(state.StateWorking, state.PhaseTool)

	d, found := o.registry.Lookup(call.Name)
	if !found {
		return fmt.Sprintf("[tool %q is not available]", call.Name), nil, nil
	}
	args := make(map[string]string, len(call.Arguments))
	for k, v := range call.Arguments {
		args[k] = tools.StringifyArg(v)
	}

	pin := &toolPin{}
	var verdict tools.Verdict
	switch {
	case o.settings.ToolReadOnly && d.Risk != tools.RiskRead:
		verdict = tools.Verdict{Decision: tools.Deny, Reason: "read_only"}
		pin.callOrdinal, pin.callText = o.publishToolCall(d, args)
	default:
		verdict = o.policy.Evaluate(d, args)
		if verdict.Decision == tools.NeedsApproval {
			v, err := o.RequestApproval(ctx, d, args, o.policy)
			if err != nil {
				return "", nil, err // interrupted while awaiting
			}
			verdict = v
		} else {
			pin.callOrdinal, pin.callText = o.publishToolCall(d, args)
		}
	}

	if verdict.Decision == tools.Deny {
		pin.resultOrdinal, pin.resultText = o.publishToolResult(tools.Result{ToolID: d.ID, Status: "denied", Exit: -1, Preview: "denied: " + verdict.Reason}, args)
		return toolDeniedContext(d, verdict.Reason), pin, nil
	}

	res, err := o.runner.Run(ctx, d, args)
	if err != nil {
		res = tools.Result{ToolID: d.ID, Status: "error", Exit: -1, Preview: err.Error(), Stderr: err.Error()}
	}
	if err == nil && res.Truncated {
		newRes, ok, derr := o.RequestOutputSizeDecision(ctx, d, args, res)
		if derr != nil {
			return "", pin, derr // interrupted while awaiting
		}
		if !ok {
			deniedRes := tools.Result{ToolID: d.ID, Status: "denied", Exit: -1, Preview: "denied: output truncated, not used"}
			pin.resultOrdinal, pin.resultText = o.publishToolResult(deniedRes, args)
			return toolDeniedContext(d, "output truncated, not used"), pin, nil
		}
		res = newRes
	}
	pin.resultOrdinal, pin.resultText = o.publishToolResult(res, args)
	return toolResultContext(d, res), pin, nil
}

// streamResponse streams the answer for the assembled messages, advertising
// toolSchemas for native tool-calling. When doThink is set it reasons first
// (PhaseThinking → PhaseRespond on the first content delta) and, if the
// thinking budget elapses before any content, cancels and re-asks with
// fallback (no thinking).
//
// The live answer streams as transient agent_delta events (for the chat window's
// typing effect); when it finishes with text content, the complete answer is
// published once as a durable agent_response (a tool-call-only response, with
// empty Content, publishes nothing here — its visibility is the
// tool_call/tool_result events runNativeToolCall/plan_task publish instead). It
// returns the full result (for the loop to inspect ToolCalls and fold history),
// that complete event's ordinal (0 when nothing was answered), and the model
// error for finishCycle to map.
func (o *Orchestrator) streamResponse(ctx context.Context, messages, fallback []prompting.Message, toolSchemas []tools.ToolSchema, doThink, ephemeral bool) (ChatResult, uint64, error) {
	prePhase := state.PhaseRespond
	if doThink {
		prePhase = state.PhaseThinking
	}
	o.setProcessing(state.StateWorking, prePhase)

	var onThink func(string)
	if doThink {
		onThink = func(t string) {
			o.publishEv("THINKING", state.ContentThinking, map[string]any{"text": t}, ephemeral)
		}
	}
	var respondStarted atomic.Bool
	onDelta := func(delta string) {
		if doThink && respondStarted.CompareAndSwap(false, true) {
			o.setProcessing(state.StateWorking, state.PhaseRespond)
		}
		o.publishEv("AGENT_DELTA", state.ContentAgentDelta, map[string]any{"text": delta}, ephemeral)
	}

	respondCtx := ctx
	if doThink && o.settings.ThinkingBudget > 0 {
		var cancel context.CancelFunc
		respondCtx, cancel = context.WithCancel(ctx)
		timer := time.AfterFunc(o.settings.ThinkingBudget, func() {
			if !respondStarted.Load() {
				cancel()
			}
		})
		defer timer.Stop()
		defer cancel()
	}

	result, err := o.model.Chat(respondCtx, o.modelName(), messages, toolSchemas, onDelta, onThink)

	// Thinking budget exceeded (child ctx canceled, parent live, no content yet):
	// answer directly without thinking.
	if errors.Is(err, context.Canceled) && ctx.Err() == nil && !respondStarted.Load() && fallback != nil {
		o.publishEv("THINKING", state.ContentThinking, map[string]any{"text": "\n…(thinking budget reached — answering directly)"}, ephemeral)
		o.setProcessing(state.StateWorking, state.PhaseRespond)
		result, err = o.model.Chat(ctx, o.modelName(), fallback, toolSchemas, onDelta, nil)
	}

	// Publish the complete answer as one durable agent_response (the canonical
	// conversation element) on success or a user interrupt that kept partial text.
	// A hard error publishes its own error agent_response via finishCycle.
	var respOrd uint64
	if result.Content != "" && (err == nil || errors.Is(err, context.Canceled)) {
		respOrd = o.publishEv("AGENT_RESPONSE", state.ContentAgentResponse, map[string]any{"text": result.Content}, ephemeral)
	}
	return result, respOrd, err
}

// finishCycle maps the terminal model error to processing-state: nil and a user
// interrupt complete cleanly; any other error is recorded and fails the cycle.
func (o *Orchestrator) finishCycle(err error) error {
	switch {
	case err == nil:
		o.setProcessing(state.StateCompleted, state.PhaseNone)
		return nil
	case errors.Is(err, context.Canceled):
		o.setProcessing(state.StateCompleted, state.PhaseNone)
		return nil
	default:
		o.publish("ERROR", state.ContentAgentResponse, map[string]any{"text": err.Error()})
		o.setProcessing(state.StateFailed, state.PhaseNone)
		return err
	}
}

// publishToolResult emits a tool_result event (rendered as the 📋 result widget;
// persisted as the audit record). The model sees the full captured result text
// (res.Preview — never a truncated excerpt of it, see tools.Result's doc
// comment) plus a ref it can use with read_output to re-read a specific window
// — unless the context surface enables this element, folding it into
// subsequent turns too, or pins it to working memory (see
// Orchestrator.PinToolEvent). args is the tool's resolved argument map, carried
// on the payload so a pin can later re-run the exact same call ("live"). It
// returns the event's ordinal and rendered text.
func (o *Orchestrator) publishToolResult(res tools.Result, args map[string]string) (uint64, string) {
	text := toolResultText(res)
	ord := o.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.id.ID,
		EventType:   "TOOL_RESULT",
		ContentType: state.ContentToolResult,
		ToolName:    res.ToolID,
		Payload: map[string]any{
			"text":    text,
			"status":  res.Status,
			"exit":    res.Exit,
			"ref":     res.Ref,
			"bytes":   res.Bytes,
			"lines":   res.Lines,
			"command": res.Command,
			"args":    args,
		},
		Enabled:   state.DefaultEnabled(state.ContentToolResult),
		ModelName: o.settings.OllamaModel,
	})
	return ord, text
}

func toolResultText(res tools.Result) string {
	head := fmt.Sprintf("[%s exit=%d, %d lines]", res.Status, res.Exit, res.Lines)
	if res.Preview == "" {
		return head
	}
	return head + "\n" + res.Preview
}

// toolResultContext renders the tool outcome as the content of a native
// tool-result message: the full captured result text (never a truncated
// excerpt of it — see tools.Result), plus a ref for re-reading a specific
// window via read_output.
func toolResultContext(d tools.Descriptor, res tools.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[tool %s result — status %s, exit %d, %d lines]\n%s",
		d.ID, res.Status, res.Exit, res.Lines, res.Preview)
	if res.Ref != "" {
		fmt.Fprintf(&b, "\n(artifact ref: %s; use read_output to re-read a specific window)", res.Ref)
	}
	return b.String()
}

func toolDeniedContext(d tools.Descriptor, reason string) string {
	return fmt.Sprintf("[tool %s was not permitted: %s]", d.ID, reason)
}
