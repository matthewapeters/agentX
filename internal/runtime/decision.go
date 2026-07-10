package runtime

import (
	"context"

	"agentx/internal/state"
)

// approvalUIRequest is the Req type for the single, cross-kind decision gate:
// the generic prompt+options every interactive-decision request shows the
// surface, regardless of what it represents underneath — tool-execution
// approval, verb-continuation approval, or any future decision kind.
type approvalUIRequest struct {
	prompt  string
	options []state.ApprovalOption
}

// decisionGate is the shared, cross-kind serialized queue every interactive
// decision funnels into — replacing the former separate approvalGate/
// verbApprovalGate so concurrent requests of DIFFERENT kinds now serialize
// against each other too (previously only guaranteed within one kind).
type decisionGate = gate[approvalUIRequest, string]

// Resolve delivers a surface decision string to the currently-shown request
// on the shared decision gate. A no-op when nothing is pending. The surface
// never needs to know which kind of decision is currently showing — it just
// echoes back the Decision string of whichever option the user picked.
func (o *Orchestrator) Resolve(decision string) {
	o.gate.deliver(decision)
}

// RequestDecision is the generic seam every interactive-decision request
// funnels through: enqueue, publish-if-front (announce the prompt+options and
// move the cycle to awaiting_input) once at the front of the queue, block on
// this request's own channel (or ctx), publish the audit decision event on a
// successful resolution, and advance the queue on return. Callers translate
// the returned raw decision string into their own local enum and apply any
// kind-specific persistence (e.g. policy approval, verb allow/deny lists).
//
// An interrupted/canceled request never "decided" anything, so it does not
// get an audit line — only the ctx.Done() branch skips publishApprovalDecision.
func (o *Orchestrator) RequestDecision(ctx context.Context, phase state.Phase, prompt string, options []state.ApprovalOption) (string, error) {
	req := newPendingRequest[approvalUIRequest, string](approvalUIRequest{prompt: prompt, options: options})
	shown := o.gate.enqueue(req)
	if shown {
		o.publishApprovalPrompt(prompt, options)
		o.setProcessing(state.StateAwaitingInput, phase)
	}
	defer func() {
		if next, ok := o.gate.dequeue(req); ok {
			o.publishApprovalPrompt(next.payload.prompt, next.payload.options)
			o.setProcessing(state.StateAwaitingInput, phase)
		}
	}()

	select {
	case dec := <-req.resp:
		o.publishApprovalDecision(prompt, chosenLabel(options, dec), dec)
		return dec, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// chosenLabel looks up the display label for a decision string among a
// request's own options, falling back to the raw decision string if it
// somehow doesn't match any option (e.g. a malformed remote-surface POST).
func chosenLabel(options []state.ApprovalOption, decision string) string {
	for _, opt := range options {
		if opt.Decision == decision {
			return opt.Label
		}
	}
	return decision
}

// publishApprovalPrompt emits {prompt, options} as an approval_request event —
// rendered by the surface both as the swapped-in input-panel widget and as a
// lightweight scrollback record of what was asked. Replaces the former
// per-kind publishToolCall (the pending-decision display use, not
// classifier_pipeline.go's unrelated post-approval ContentToolCall log) and
// publishVerbPrompt.
func (o *Orchestrator) publishApprovalPrompt(prompt string, options []state.ApprovalOption) {
	o.publish("APPROVAL_REQUEST", state.ContentApprovalRequest, map[string]any{
		"prompt": prompt, "options": options,
	})
}

// publishApprovalDecision emits the audit record of what was chosen — prompt +
// chosen option's Label AS TEXT (never the raw decision key) — via
// ContentApprovalDecision (DefaultEnabled=false): persisted, excluded from
// context.
func (o *Orchestrator) publishApprovalDecision(prompt, chosenLabelText, decision string) {
	o.publish("APPROVAL_DECISION", state.ContentApprovalDecision, map[string]any{
		"prompt": prompt, "chosen_label": chosenLabelText, "decision": decision,
	})
}
