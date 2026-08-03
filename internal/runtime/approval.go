package runtime

import (
	"context"
	"sort"
	"strings"

	"agentx/internal/state"
	"agentx/internal/tools"
)

// ToolRunner executes an approved tool descriptor. *tools.Executor satisfies it;
// tests inject a stub.
type ToolRunner interface {
	Run(ctx context.Context, d tools.Descriptor, args map[string]string) (tools.Result, error)
}

// WithToolRunner overrides the tool executor (tests inject a stub runner).
func WithToolRunner(r ToolRunner) Option {
	return func(o *Orchestrator) { o.runner = r }
}

// toolApprovalOptions is the option set a tool-approval request outside any
// plan offers (root == "" — the single_tool cycle never has a plan to scope
// an approval to).
var toolApprovalOptions = []state.ApprovalOption{
	{Label: "Approve for this session", Decision: "session"},
	{Label: "Approve for all sessions", Decision: "global"},
	{Label: "Deny", Decision: "deny"},
}

// toolApprovalOptionsPlan is offered instead whenever root != "" — a call
// made while a plan is draining, so the user can additionally scope an
// approval to that plan only (docs/architecture/behavior/
// tool_policy_plan_scoped_approval.feature.md).
var toolApprovalOptionsPlan = []state.ApprovalOption{
	{Label: "Approve for this session", Decision: "session"},
	{Label: "Approve for this plan", Decision: "plan"},
	{Label: "Approve for all sessions", Decision: "global"},
	{Label: "Deny", Decision: "deny"},
}

// toolApprovalOptionsFor picks the option set for a proposed call: the plan
// option only appears when the call is actually part of a plan.
func toolApprovalOptionsFor(root string) []state.ApprovalOption {
	if root == "" {
		return toolApprovalOptions
	}
	return toolApprovalOptionsPlan
}

// RequestApproval enqueues the proposed tool call and, once it's at the front of the
// shared decision queue, blocks until the surface resolves this specific request (or
// ctx is canceled) — never a shared channel, so a second concurrent request can never
// orphan this one. On approval it persists the chosen scope to pol and returns Allow;
// a denial returns Deny; cancellation returns the ctx error. It is the reusable
// approval seam both the single_tool cycle and the scheduler's concurrent leaves call
// (TOOL-4; ADR 0008). root is the plan this call belongs to, or "" outside any plan —
// it gates whether the plan-scoped option is offered and, if chosen, which plan the
// approval is recorded against.
func (o *Orchestrator) RequestApproval(ctx context.Context, d tools.Descriptor, args map[string]string, pol *tools.Policy, root string) (tools.Verdict, error) {
	dec, err := o.RequestDecision(ctx, state.PhaseTool, proposalText(d, args), toolApprovalOptionsFor(root))
	if err != nil {
		return tools.Verdict{}, err
	}
	switch dec {
	case "session":
		pol.Approve(tools.ScopeSession, d, args)
		return tools.Verdict{Decision: tools.Allow}, nil
	case "plan":
		pol.ApprovePlan(root, d, args)
		return tools.Verdict{Decision: tools.Allow}, nil
	case "global":
		pol.Approve(tools.ScopeGlobal, d, args)
		// Persist the global whitelist so the approval survives restarts.
		if err := tools.SaveApprovals(o.settings.ToolApprovalsPath, pol.GlobalApprovals()); err != nil {
			return tools.Verdict{}, err
		}
		return tools.Verdict{Decision: tools.Allow}, nil
	default:
		return tools.Verdict{Decision: tools.Deny, Reason: "user_denied"}, nil
	}
}

// proposalText renders the proposed command for display/audit: the rendered argv
// when available, otherwise the tool id with its sorted arguments.
func proposalText(d tools.Descriptor, args map[string]string) string {
	if len(d.Argv) > 0 {
		if argv, err := d.BuildArgv(args); err == nil {
			return strings.Join(argv, " ")
		}
	}
	parts := []string{d.ID}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := args[k]
		if len(v) > 60 {
			v = v[:60] + "…"
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " ")
}
