package runtime

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"agentx/internal/prompting"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// maxToolIterations bounds how many native tool-call round-trips one turn may
// run before the loop stops and answers with whatever it has. Moved from
// Orchestrator (tool_cycle.go) verbatim; maxIterSetting reads the live setting
// fresh (not currently a live-reloadable config key, but kept as a closure for
// consistency with every other Settings-derived value on Core — see
// ConversationCore's type doc comment).
func (c *ConversationCore) maxToolIterations() int {
	if n := c.maxIterSetting(); n > 0 {
		return n
	}
	return 25
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
// error for finishCycle to map. Moved from Orchestrator (tool_cycle.go)
// verbatim in control flow — o.setProcessing/o.publishEv/o.model/o.modelName()
// reaches are replaced by c.report/c.events.Publish/c.model/c.modelName.
func (c *ConversationCore) streamResponse(ctx context.Context, messages, fallback []prompting.Message, toolSchemas []tools.ToolSchema, doThink, ephemeral bool) (ChatResult, uint64, error) {
	prePhase := state.PhaseRespond
	if doThink {
		prePhase = state.PhaseThinking
	}
	c.report(state.StateWorking, prePhase)

	var onThink func(string)
	if doThink {
		onThink = func(t string) {
			c.events.Publish("THINKING", state.ContentThinking, map[string]any{"text": t}, ephemeral)
		}
	}
	var respondStarted atomic.Bool
	onDelta := func(delta string) {
		if doThink && respondStarted.CompareAndSwap(false, true) {
			c.report(state.StateWorking, state.PhaseRespond)
		}
		c.events.Publish("AGENT_DELTA", state.ContentAgentDelta, map[string]any{"text": delta}, ephemeral)
	}

	respondCtx := ctx
	if doThink {
		if budget := c.thinkingBudget(); budget > 0 {
			var cancel context.CancelFunc
			respondCtx, cancel = context.WithCancel(ctx)
			timer := time.AfterFunc(budget, func() {
				if !respondStarted.Load() {
					cancel()
				}
			})
			defer timer.Stop()
			defer cancel()
		}
	}

	result, err := c.model.Chat(respondCtx, c.modelName, messages, toolSchemas, onDelta, onThink)

	// Thinking budget exceeded (child ctx canceled, parent live, no content yet):
	// answer directly without thinking.
	if errors.Is(err, context.Canceled) && ctx.Err() == nil && !respondStarted.Load() && fallback != nil {
		c.events.Publish("THINKING", state.ContentThinking, map[string]any{"text": "\n…(thinking budget reached — answering directly)"}, ephemeral)
		c.report(state.StateWorking, state.PhaseRespond)
		result, err = c.model.Chat(ctx, c.modelName, fallback, toolSchemas, onDelta, nil)
	}

	// Publish the complete answer as one durable agent_response (the canonical
	// conversation element) on success or a user interrupt that kept partial text.
	// A hard error publishes its own error agent_response via finishCycle.
	var respOrd uint64
	if result.Content != "" && (err == nil || errors.Is(err, context.Canceled)) {
		respOrd = c.events.Publish("AGENT_RESPONSE", state.ContentAgentResponse, map[string]any{"text": result.Content}, ephemeral)
	}
	return result, respOrd, err
}

// finishCycle maps the terminal model error to processing-state: nil and a user
// interrupt complete cleanly; any other error is recorded and fails the cycle.
// Moved from Orchestrator (tool_cycle.go) verbatim.
func (c *ConversationCore) finishCycle(err error) error {
	switch {
	case err == nil:
		c.report(state.StateCompleted, state.PhaseNone)
		return nil
	case errors.Is(err, context.Canceled):
		c.report(state.StateCompleted, state.PhaseNone)
		return nil
	default:
		c.events.Publish("ERROR", state.ContentAgentResponse, map[string]any{"text": err.Error()}, false)
		c.report(state.StateFailed, state.PhaseNone)
		return err
	}
}
