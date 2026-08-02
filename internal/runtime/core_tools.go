package runtime

import (
	"context"
	"fmt"

	"agentx/internal/prompting"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// toolsReady reports whether native tool-calling can advertise/execute tools —
// the ConversationCore-native equivalent of Orchestrator.toolsReady
// (tool_cycle.go). Deliberately not unified with Orchestrator's copy: the
// classifier pipeline (classifier_pipeline.go, disconnected from the main loop
// but still present) calls Orchestrator's directly, and coupling that unrelated
// path to Core's construction order isn't worth it for a four-line check.
func (c *ConversationCore) toolsReady() bool {
	return c.toolsEnabled() && c.runner != nil && c.policy != nil && c.registry != nil
}

// toolSchemas returns the generic tool catalog this turn should advertise,
// filtered by the live toolReadOnly setting. Does not include plan_task — that
// remains an Orchestrator-only concern layered on by Orchestrator's
// availableToolSchemas (ADR 0013 §"Explicitly not decided here", Open Question
// 4). RunPrompt reaches the combined (generic + plan_task) list through the
// toolSchemasFn closure, not this method directly.
func (c *ConversationCore) toolSchemas() []tools.ToolSchema {
	if !c.toolsReady() {
		return nil
	}
	return c.registry.ToolSchemas(c.toolReadOnly())
}

// runNativeToolCall executes one model-issued native tool call: looks it up in
// the registry, evaluates policy (prompting for approval when required), runs
// it, and publishes tool_call/tool_result. Returns the tool-result text to fold
// back into the turn's messages and the pinnable ordinals for that call/result.
// A non-nil error means the user interrupted while awaiting approval or an
// output-size decision. Moved from Orchestrator (tool_cycle.go) verbatim in
// control flow — o.gate/o.publishToolCall/o.publishToolResult reaches are
// replaced by c.approvals/c.publishToolCall/c.publishToolResult (below).
func (c *ConversationCore) runNativeToolCall(ctx context.Context, call prompting.ToolCall) (string, *toolPin, error) {
	c.report(state.StateWorking, state.PhaseTool)

	d, found := c.registry.Lookup(call.Name)
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
	case c.toolReadOnly() && d.Risk != tools.RiskRead:
		verdict = tools.Verdict{Decision: tools.Deny, Reason: "read_only"}
		pin.callOrdinal, pin.callText = c.publishToolCall(d, args)
	default:
		verdict = c.policy.Evaluate(d, args)
		if verdict.Decision == tools.NeedsApproval {
			v, err := c.approvals.RequestApproval(ctx, d, args, c.policy)
			if err != nil {
				return "", nil, err // interrupted while awaiting
			}
			verdict = v
		} else {
			pin.callOrdinal, pin.callText = c.publishToolCall(d, args)
		}
	}

	if verdict.Decision == tools.Deny {
		pin.resultOrdinal, pin.resultText = c.publishToolResult(tools.Result{ToolID: d.ID, Status: "denied", Exit: -1, Preview: "denied: " + verdict.Reason}, args)
		return toolDeniedContext(d, verdict.Reason), pin, nil
	}

	res, err := c.runner.Run(ctx, d, args)
	if err != nil {
		res = tools.Result{ToolID: d.ID, Status: "error", Exit: -1, Preview: err.Error(), Stderr: err.Error()}
	}
	if err == nil && res.Truncated {
		newRes, ok, derr := c.approvals.RequestOutputSizeDecision(ctx, d, args, res)
		if derr != nil {
			return "", pin, derr // interrupted while awaiting
		}
		if !ok {
			deniedRes := tools.Result{ToolID: d.ID, Status: "denied", Exit: -1, Preview: "denied: output truncated, not used"}
			pin.resultOrdinal, pin.resultText = c.publishToolResult(deniedRes, args)
			return toolDeniedContext(d, "output truncated, not used"), pin, nil
		}
		res = newRes
	}
	pin.resultOrdinal, pin.resultText = c.publishToolResult(res, args)
	return toolResultContext(d, res), pin, nil
}

// publishToolCall/publishToolResult mirror Orchestrator's pre-extraction helpers
// of the same name (formerly approval.go/tool_cycle.go), moved onto
// ConversationCore via EventSink.PublishTool.
func (c *ConversationCore) publishToolCall(d tools.Descriptor, args map[string]string) (uint64, string) {
	text := proposalText(d, args)
	ord := c.events.PublishTool(ToolEvent{
		EventType:   "TOOL_CALL",
		ContentType: state.ContentToolCall,
		ToolName:    d.ID,
		Payload:     map[string]any{"text": text},
	})
	return ord, text
}

func (c *ConversationCore) publishToolResult(res tools.Result, args map[string]string) (uint64, string) {
	text := toolResultText(res)
	ord := c.events.PublishTool(ToolEvent{
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
	})
	return ord, text
}
