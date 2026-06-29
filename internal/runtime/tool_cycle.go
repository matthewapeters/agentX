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

// buildTools constructs the registry, policy, executor, and proposer for the
// single_tool cycle. Called under o.mu during Start when tools are enabled.
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
		o.runner = tools.NewExecutor(art, 0, o.settings.ToolOutputMaxBytes)
	}
	if o.proposer == nil {
		chat := func(ctx context.Context, msgs []prompting.Message) (string, error) {
			return o.model.Chat(ctx, o.settings.OllamaModel, msgs, func(string) {}, nil)
		}
		o.proposer = tools.NewProposer(o.settings.ToolCatalog, o.settings.ClassificationRetries, chat)
	}
	return nil
}

// toolsReady reports whether the single_tool cycle can run.
func (o *Orchestrator) toolsReady() bool {
	return o.settings.ToolsEnabled && o.proposer != nil && o.runner != nil &&
		o.policy != nil && o.registry != nil
}

// runToolPhase proposes one tool call, evaluates policy (prompting for approval
// when required), executes the approved call, and publishes tool_call/tool_result.
// It returns a context block to fold into the respond turn and whether a tool was
// handled (false => answer directly). A non-nil error means the user interrupted
// while awaiting approval.
func (o *Orchestrator) runToolPhase(ctx context.Context, text string) (string, bool, error) {
	o.setProcessing(state.StateWorking, state.PhaseTool)

	prop, ok := o.proposer.Propose(ctx, text)
	if !ok {
		return "", false, nil // no tool chosen → answer directly
	}
	d, found := o.registry.Lookup(prop.Tool)
	if !found {
		return "", false, nil // unknown/hallucinated tool → answer directly
	}

	var verdict tools.Verdict
	switch {
	case o.settings.ToolReadOnly && d.Risk != tools.RiskRead:
		verdict = tools.Verdict{Decision: tools.Deny, Reason: "read_only"}
		o.publishToolCall(d, prop.Args)
	default:
		verdict = o.policy.Evaluate(d, prop.Args)
		if verdict.Decision == tools.NeedsApproval {
			v, err := o.RequestApproval(ctx, d, prop.Args, o.policy)
			if err != nil {
				return "", false, err // interrupted while awaiting
			}
			verdict = v
		} else {
			o.publishToolCall(d, prop.Args)
		}
	}

	if verdict.Decision == tools.Deny {
		o.publishToolResult(tools.Result{ToolID: d.ID, Status: "denied", Exit: -1, Preview: "denied: " + verdict.Reason})
		return toolDeniedContext(d, verdict.Reason), true, nil
	}

	res, err := o.runner.Run(ctx, d, prop.Args)
	if err != nil {
		res = tools.Result{ToolID: d.ID, Status: "error", Exit: -1, Preview: err.Error(), Stderr: err.Error()}
	}
	o.publishToolResult(res)
	return toolResultContext(d, res), true, nil
}

// streamResponse streams the answer for the assembled messages. When doThink is
// set it reasons first (PhaseThinking → PhaseRespond on the first content delta)
// and, if the thinking budget elapses before any content, cancels and re-asks with
// fallback (no thinking). It returns the full assembled answer (for conversation
// history) and the model error for finishCycle to map.
func (o *Orchestrator) streamResponse(ctx context.Context, messages, fallback []prompting.Message, doThink bool) (string, error) {
	prePhase := state.PhaseRespond
	if doThink {
		prePhase = state.PhaseThinking
	}
	o.setProcessing(state.StateWorking, prePhase)

	var onThink func(string)
	if doThink {
		onThink = func(t string) {
			o.publish("THINKING", state.ContentThinking, map[string]any{"text": t})
		}
	}
	var respondStarted atomic.Bool
	onDelta := func(delta string) {
		if doThink && respondStarted.CompareAndSwap(false, true) {
			o.setProcessing(state.StateWorking, state.PhaseRespond)
		}
		o.publish("AGENT_CONTENT", state.ContentAgentResponse, map[string]any{"text": delta})
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

	resp, err := o.model.Chat(respondCtx, o.settings.OllamaModel, messages, onDelta, onThink)

	// Thinking budget exceeded (child ctx canceled, parent live, no content yet):
	// answer directly without thinking.
	if errors.Is(err, context.Canceled) && ctx.Err() == nil && !respondStarted.Load() && fallback != nil {
		o.publish("THINKING", state.ContentThinking, map[string]any{"text": "\n…(thinking budget reached — answering directly)"})
		o.setProcessing(state.StateWorking, state.PhaseRespond)
		resp, err = o.model.Chat(ctx, o.settings.OllamaModel, fallback, onDelta, nil)
	}
	return resp, err
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
// persisted as the audit record). The model sees the preview + ref, not the full
// artifact.
func (o *Orchestrator) publishToolResult(res tools.Result) {
	o.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.id.ID,
		EventType:   "TOOL_RESULT",
		ContentType: state.ContentToolResult,
		ToolName:    res.ToolID,
		Payload: map[string]any{
			"text":    toolResultText(res),
			"status":  res.Status,
			"exit":    res.Exit,
			"ref":     res.Ref,
			"bytes":   res.Bytes,
			"lines":   res.Lines,
			"command": res.Command,
		},
		ModelName: o.settings.OllamaModel,
	})
}

func toolResultText(res tools.Result) string {
	head := fmt.Sprintf("[%s exit=%d, %d lines]", res.Status, res.Exit, res.Lines)
	if res.Preview == "" {
		return head
	}
	return head + "\n" + res.Preview
}

// toolResultContext renders the tool outcome folded into the respond turn (preview
// + ref only — never the full artifact).
func toolResultContext(d tools.Descriptor, res tools.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n\n[tool %s result — status %s, exit %d, %d lines]\n%s",
		d.ID, res.Status, res.Exit, res.Lines, res.Preview)
	if res.Ref != "" {
		fmt.Fprintf(&b, "\n(full output ref: %s; use read_output to page)", res.Ref)
	}
	return b.String()
}

func toolDeniedContext(d tools.Descriptor, reason string) string {
	return fmt.Sprintf("\n\n[tool %s was not permitted: %s]", d.ID, reason)
}
