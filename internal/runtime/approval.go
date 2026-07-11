package runtime

import (
	"context"
	"sort"
	"strings"
	"time"

	"agentx/internal/state"
	"agentx/internal/tools"
)

// ToolRunner executes an approved tool descriptor. *tools.Executor satisfies it;
// tests inject a stub.
type ToolRunner interface {
	Run(ctx context.Context, d tools.Descriptor, args map[string]string) (tools.Result, error)
}

// WithProposer overrides the tool proposer (tests inject a deterministic one).
func WithProposer(p *tools.Proposer) Option {
	return func(o *Orchestrator) { o.proposer = p }
}

// WithToolRunner overrides the tool executor (tests inject a stub runner).
func WithToolRunner(r ToolRunner) Option {
	return func(o *Orchestrator) { o.runner = r }
}

// toolApprovalOptions is the fixed option set every tool-approval request
// offers — deliberately not configurable per call site, so the surface always
// shows the same three choices regardless of which tool is being proposed.
var toolApprovalOptions = []state.ApprovalOption{
	{Label: "Approve for this session", Decision: "session"},
	{Label: "Approve for all sessions", Decision: "global"},
	{Label: "Deny", Decision: "deny"},
}

// RequestApproval enqueues the proposed tool call and, once it's at the front of the
// shared decision queue, blocks until the surface resolves this specific request (or
// ctx is canceled) — never a shared channel, so a second concurrent request can never
// orphan this one. On approval it persists the chosen scope to pol and returns Allow;
// a denial returns Deny; cancellation returns the ctx error. It is the reusable
// approval seam both the single_tool cycle and the scheduler's concurrent leaves call
// (TOOL-4; ADR 0008).
func (o *Orchestrator) RequestApproval(ctx context.Context, d tools.Descriptor, args map[string]string, pol *tools.Policy) (tools.Verdict, error) {
	dec, err := o.RequestDecision(ctx, state.PhaseTool, proposalText(d, args), toolApprovalOptions)
	if err != nil {
		return tools.Verdict{}, err
	}
	switch dec {
	case "session":
		pol.Approve(tools.ScopeSession, d, args)
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

// publishToolCall emits the proposed command as a tool_call event for the surface
// (rendered as the 🔧 widget): a plain record that a tool was called, independent of
// whether it went through the approval gate (see RequestDecision/publishApprovalPrompt
// for the pending-decision display itself). It returns the event's ordinal and
// rendered text so the caller can register it as a pinnable context-history entry
// (recordTurn) — pinning is initially off (DefaultEnabled), matching the checkbox
// the context surface shows for it.
func (o *Orchestrator) publishToolCall(d tools.Descriptor, args map[string]string) (uint64, string) {
	text := proposalText(d, args)
	ord := o.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.id.ID,
		EventType:   "TOOL_CALL",
		ContentType: state.ContentToolCall,
		ToolName:    d.ID,
		Payload:     map[string]any{"text": text},
		Enabled:     state.DefaultEnabled(state.ContentToolCall),
		ModelName:   o.settings.OllamaModel,
	})
	return ord, text
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
