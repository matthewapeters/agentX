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

// ApprovalDecision is the user's response to a tool-approval request.
type ApprovalDecision int

const (
	// DecisionDeny blocks the proposed command.
	DecisionDeny ApprovalDecision = iota
	// DecisionSession approves it for the current session.
	DecisionSession
	// DecisionGlobal approves it for all sessions.
	DecisionGlobal
)

// approvalPayload is one pending tool call awaiting a user decision — the Req type
// parameter for the tool-approval gate (see gate.go).
type approvalPayload struct {
	descriptor tools.Descriptor
	args       map[string]string
}

// approvalGate is the tool-approval queue: a gate of approvalPayload requests,
// resolved with an ApprovalDecision.
type approvalGate = gate[approvalPayload, ApprovalDecision]

// Resolve delivers a surface approval decision to the currently-shown request: "session"
// or "global" approve, anything else denies. It is a no-op when no request is pending.
func (o *Orchestrator) Resolve(decision string) {
	switch decision {
	case "session":
		o.gate.deliver(DecisionSession)
	case "global":
		o.gate.deliver(DecisionGlobal)
	default:
		o.gate.deliver(DecisionDeny)
	}
}

// RequestApproval enqueues the proposed tool call and, once it's at the front of the
// queue, publishes it and moves the cycle to awaiting_input; it blocks until the surface
// resolves this specific request (or ctx is canceled) — never a shared channel, so a
// second concurrent request can never orphan this one. On approval it persists the chosen
// scope to pol and returns Allow; a denial returns Deny; cancellation returns the ctx
// error and — if this request was already showing — advances the queue to the next one,
// so a canceled/interrupted request never leaves the surface stuck displaying it forever.
// It is the reusable approval seam both the single_tool cycle and the scheduler's
// concurrent leaves call (TOOL-4; ADR 0008).
func (o *Orchestrator) RequestApproval(ctx context.Context, d tools.Descriptor, args map[string]string, pol *tools.Policy) (tools.Verdict, error) {
	req := newPendingRequest[approvalPayload, ApprovalDecision](approvalPayload{descriptor: d, args: args})
	shown := o.gate.enqueue(req)
	if shown {
		o.publishToolCall(d, args)
		o.setProcessing(state.StateAwaitingInput, state.PhaseTool)
	}
	defer func() {
		if next, ok := o.gate.dequeue(req); ok {
			o.publishToolCall(next.payload.descriptor, next.payload.args)
			o.setProcessing(state.StateAwaitingInput, state.PhaseTool)
		}
	}()

	select {
	case dec := <-req.resp:
		switch dec {
		case DecisionSession:
			pol.Approve(tools.ScopeSession, d, args)
			return tools.Verdict{Decision: tools.Allow}, nil
		case DecisionGlobal:
			pol.Approve(tools.ScopeGlobal, d, args)
			// Persist the global whitelist so the approval survives restarts.
			if err := tools.SaveApprovals(o.settings.ToolApprovalsPath, pol.GlobalApprovals()); err != nil {
				return tools.Verdict{}, err
			}
			return tools.Verdict{Decision: tools.Allow}, nil
		default:
			return tools.Verdict{Decision: tools.Deny, Reason: "user_denied"}, nil
		}
	case <-ctx.Done():
		return tools.Verdict{}, ctx.Err()
	}
}

// publishToolCall emits the proposed command as a tool_call event for the surface
// (rendered as the 🔧 widget the approval affordance attaches to).
func (o *Orchestrator) publishToolCall(d tools.Descriptor, args map[string]string) {
	o.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.id.ID,
		EventType:   "TOOL_CALL",
		ContentType: state.ContentToolCall,
		ToolName:    d.ID,
		Payload:     map[string]any{"text": proposalText(d, args)},
		ModelName:   o.settings.OllamaModel,
	})
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
