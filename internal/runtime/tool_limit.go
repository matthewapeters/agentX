package runtime

import (
	"context"
	"fmt"

	"agentx/internal/state"
)

// toolLimitOptions is the fixed option set offered when a turn hits its
// per-turn tool-call round-trip budget (maxToolIterations) — a simple
// continue/stop choice, not scoped (session/global) the way tool approval
// is: "keep working a while longer this turn" isn't a standing permission
// worth remembering, just a one-off decision about how deep THIS turn goes.
var toolLimitOptions = []state.ApprovalOption{
	{Label: "Continue working", Decision: "continue"},
	{Label: "Stop here", Decision: "stop"},
}

// RequestToolLimitApproval asks whether to keep working past used tool-call
// round-trips this turn, once the per-turn budget (maxToolIterations) is
// exhausted without a final answer. A "continue" decision does not persist
// anywhere — it only answers for this specific pause; the caller
// (ConversationCore.RunPrompt) resets its own iteration counter and keeps
// looping under the same budget, asking again every time it's next
// exhausted (docs/architecture/behavior/
// tool_iteration_limit_approval.feature.md).
func (o *Orchestrator) RequestToolLimitApproval(ctx context.Context, used int) (bool, error) {
	prompt := fmt.Sprintf("Reached %d tool-call round-trips this turn without a final answer. Continue working?", used)
	dec, err := o.RequestDecision(ctx, state.PhaseIterationLimit, prompt, toolLimitOptions)
	if err != nil {
		return false, err
	}
	return dec == "continue", nil
}
