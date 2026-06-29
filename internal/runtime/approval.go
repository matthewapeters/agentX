package runtime

import (
	"context"
	"sort"
	"strings"
	"sync"
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

// approvalGate carries a single in-flight approval decision from the surface back
// to the paused prompt cycle.
type approvalGate struct {
	mu sync.Mutex
	ch chan ApprovalDecision
}

func (g *approvalGate) arm() chan ApprovalDecision {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ch = make(chan ApprovalDecision, 1)
	return g.ch
}

func (g *approvalGate) disarm() {
	g.mu.Lock()
	g.ch = nil
	g.mu.Unlock()
}

func (g *approvalGate) deliver(d ApprovalDecision) bool {
	g.mu.Lock()
	ch := g.ch
	g.mu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- d:
		return true
	default:
		return false
	}
}

// Resolve delivers a surface approval decision to a pending RequestApproval:
// "session" or "global" approve, anything else denies. It is a no-op when no
// request is pending.
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

// RequestApproval publishes the proposed tool call, moves the cycle to
// awaiting_input, and blocks until the surface resolves the request (or ctx is
// canceled). On approval it persists the chosen scope to pol and returns Allow; a
// denial returns Deny; cancellation returns the ctx error. It is the reusable
// approval seam the single_tool cycle calls (TOOL-4).
func (o *Orchestrator) RequestApproval(ctx context.Context, d tools.Descriptor, args map[string]string, pol *tools.Policy) (tools.Verdict, error) {
	ch := o.gate.arm()
	defer o.gate.disarm()

	o.publishToolCall(d, args)
	o.setProcessing(state.StateAwaitingInput, state.PhaseTool)

	select {
	case dec := <-ch:
		switch dec {
		case DecisionSession:
			pol.Approve(tools.ScopeSession, d, args)
			return tools.Verdict{Decision: tools.Allow}, nil
		case DecisionGlobal:
			pol.Approve(tools.ScopeGlobal, d, args)
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
