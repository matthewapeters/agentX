package runtime

import (
	"context"
	"fmt"

	"agentx/internal/prompting"
)

// runPrompt is now a thin delegator to ConversationCore.RunPrompt (ADR 0013
// Phase 2) — the loop body itself lives in core_loop.go. What stays here is
// exactly what the Phase 2 behavior doc
// (docs/architecture/behavior/adr/0013_conversationcore_runprompt.feature.md)
// scopes as session-lifecycle/session-store concerns that don't fit any of the
// three ConversationCore seams: the o.mu/started/accepting readiness gate, and
// refreshLiveFacts (reads/writes live working-memory facts via o.store
// directly). This method's external signature and error-return contract are
// unchanged for its two callers.
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

	_, err := o.core.RunPrompt(ctx, RunOptions{
		Text:             text,
		RecordUserPrompt: recordUserPrompt,
		Ephemeral:        ephemeral,
	})
	return err
}

// runToolOrPlan dispatches one native tool call: plan_task is intercepted
// before the generic registry lookup (it isn't a subprocess/argv Descriptor)
// and is not individually pinnable (its result is a synthesized findings
// summary, not a single tool_call/tool_result pair); everything else goes
// through the normal tool policy/execution path. Bound onto ConversationCore's
// execTool field by buildCore — see core_loop.go.
func (o *Orchestrator) runToolOrPlan(ctx context.Context, call prompting.ToolCall) (string, *toolPin, error) {
	if call.Name == planTaskToolName {
		text, err := o.runPlanTaskTool(ctx, planTaskGoal(call.Arguments))
		return text, nil, err
	}
	return o.core.runNativeToolCall(ctx, call)
}
