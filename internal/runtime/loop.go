package runtime

import (
	"context"
	"fmt"

	"agentx/internal/prompting"
	"agentx/internal/runtime/hooks"
	"agentx/internal/state"
)

// runPrompt drives one prompt cycle as a flat loop: submit → LLM (advertising
// native tool schemas) → detect tool calls vs a chat response → execute
// tools/fold results back → loop, until the model answers with plain text or
// the per-turn tool-iteration budget is hit. Hooks run at two points — right
// after the prompt is recorded, and after each round of tool execution — as a
// serial sync chain followed by a fire-and-forget async fan-out; both are
// no-ops today (o.hooks is an empty Registry until a future hook ships and is
// named in Settings.HooksConfigPath).
//
// When recordUserPrompt is false the user message is still sent to the model
// (so instructions + prompt reach the LLM) but no user_prompt event is
// published — used for the bootstrap prompt.
func (o *Orchestrator) runPrompt(ctx context.Context, text string, recordUserPrompt, ephemeral bool) error {
	o.mu.Lock()
	ready := o.started && o.accepting
	o.mu.Unlock()
	if !ready {
		return fmt.Errorf("orchestrator not accepting prompts")
	}
	o.refreshLiveFacts(ctx)

	var userOrd uint64
	if recordUserPrompt {
		userOrd = o.publishEv("USER_PROMPT", state.ContentUserPrompt, map[string]any{"text": text}, ephemeral)
	}

	// Think once, before the first model call — not on every tool-round-trip
	// iteration, which would mean reasoning before every single tool result too.
	doThink := o.settings.ThinkingEnabled
	turn := &hooks.Turn{
		Prompt:   text,
		Messages: o.withContext(o.assembler.AssembleWithThinking(text, o.thinkingPrompt(doThink), "")),
	}
	fallback := o.withContext(o.assembler.Assemble(text))

	if err := o.hooks.RunSync(ctx, turn); err != nil {
		o.setProcessing(state.StateFailed, state.PhaseNone)
		return err
	}
	o.hooks.RunAsync(ctx, *turn)

	toolSchemas := o.availableToolSchemas()
	maxIter := o.maxToolIterations()
	var lastResp string
	var respOrd uint64
	var err error
	var pins []*toolPin

	for i := 0; i < maxIter; i++ {
		var result ChatResult
		result, respOrd, err = o.streamResponse(ctx, turn.Messages, fallback, toolSchemas, doThink && i == 0, ephemeral)
		if err != nil {
			o.recordTurn(err, recordUserPrompt, userOrd, text, respOrd, "", pins)
			return o.finishCycle(err)
		}
		lastResp = result.Content
		if len(result.ToolCalls) == 0 {
			break // a chat response: the turn is done
		}

		turn.Messages = append(turn.Messages, prompting.Message{
			Role: "assistant", Content: result.Content, ToolCalls: result.ToolCalls,
		})
		for _, call := range result.ToolCalls {
			resultText, pin, terr := o.runToolOrPlan(ctx, call)
			if terr != nil {
				// Interrupted while awaiting approval: end the cycle cleanly.
				o.setProcessing(state.StateCompleted, state.PhaseNone)
				return nil
			}
			if pin != nil {
				pins = append(pins, pin)
			}
			turn.Messages = append(turn.Messages, prompting.Message{
				Role: "tool", Content: resultText, ToolCallID: call.ID,
			})
		}

		if err = o.hooks.RunSync(ctx, turn); err != nil {
			o.setProcessing(state.StateFailed, state.PhaseNone)
			return err
		}
		o.hooks.RunAsync(ctx, *turn)
	}

	if lastResp == "" && err == nil {
		// The tool-iteration budget was hit without ever reaching a text answer —
		// never leave the user with total silence.
		lastResp = "[stopped: reached the tool-call limit for this turn without a final answer]"
		respOrd = o.publishEv("AGENT_RESPONSE", state.ContentAgentResponse, map[string]any{"text": lastResp}, ephemeral)
	}
	o.recordTurn(err, recordUserPrompt, userOrd, text, respOrd, lastResp, pins)
	return o.finishCycle(err)
}

// runToolOrPlan dispatches one native tool call: plan_task is intercepted
// before the generic registry lookup (it isn't a subprocess/argv Descriptor)
// and is not individually pinnable (its result is a synthesized findings
// summary, not a single tool_call/tool_result pair); everything else goes
// through the normal tool policy/execution path.
func (o *Orchestrator) runToolOrPlan(ctx context.Context, call prompting.ToolCall) (string, *toolPin, error) {
	if call.Name == planTaskToolName {
		text, err := o.runPlanTaskTool(ctx, planTaskGoal(call.Arguments))
		return text, nil, err
	}
	return o.runNativeToolCall(ctx, call)
}
