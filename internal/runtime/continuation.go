package runtime

import (
	"context"

	"agentx/internal/planfindings"
	"agentx/internal/prompting/continuation"
	"agentx/internal/state"
)

// verbDecision is the user's response to an unrecognized stated-continuation verb.
// Two independent axes — allow-or-deny, once-or-always — kept as one enum (matching
// ApprovalDecision's shape) rather than two bools, since exactly one of the four ever
// applies to a given request.
type verbDecision int

const (
	// VerbDenyOnce: don't continue this time; ask again if this verb recurs.
	VerbDenyOnce verbDecision = iota
	// VerbAllowOnce: continue this time only; the verb is not persisted either way.
	VerbAllowOnce
	// VerbAllowAlways: continue, and append the verb to the allow-list so future
	// occurrences auto-continue without asking.
	VerbAllowAlways
	// VerbDenyAlways: don't continue, and append the verb to the deny-list so future
	// occurrences are silently ignored (not treated as a continuation trigger, and
	// never asked about again).
	VerbDenyAlways
)

// verbApprovalOptions is the fixed option set every verb-continuation request
// offers — deliberately not configurable per call site, so the surface always
// shows the same four choices regardless of which verb is being asked about.
var verbApprovalOptions = []state.ApprovalOption{
	{Label: "Yes, just this once", Decision: "allow_once"},
	{Label: "Yes, always", Decision: "allow_always"},
	{Label: "No, not now", Decision: "deny_once"},
	{Label: "No, never", Decision: "deny_always"},
}

// RequestVerbApproval enqueues an unrecognized stated-continuation verb and, once
// it's at the front of the shared decision queue, blocks until the surface resolves
// this specific request (or ctx is canceled) — the same per-request-channel,
// FIFO-queue shape RequestApproval uses, so a second concurrent verb prompt can
// never orphan this one. On an "always" decision it persists the verb to the
// corresponding externalized list so the system learns it without needing every
// possible verb predicted in advance.
func (o *Orchestrator) RequestVerbApproval(ctx context.Context, verb, sentence string) (verbDecision, error) {
	dec, err := o.RequestDecision(ctx, state.PhaseVerb, sentence, verbApprovalOptions)
	if err != nil {
		return VerbDenyOnce, err
	}
	switch dec {
	case "allow_once":
		return VerbAllowOnce, nil
	case "allow_always":
		_ = continuation.AppendVerb(o.settings.ContinuationVerbsAllowedPath, verb)
		return VerbAllowAlways, nil
	case "deny_always":
		_ = continuation.AppendVerb(o.settings.ContinuationVerbsDeniedPath, verb)
		return VerbDenyAlways, nil
	default:
		return VerbDenyOnce, nil
	}
}

// maybeContinuePlan checks the model's own synthesized response for a stated
// continuation intent ("Let me examine the source code...") and, if warranted, runs
// one more bounded decomposition round — grounded in what the FIRST round already
// found, not just its own — before the turn finalizes. This is the fix for a
// response that correctly recognizes its own investigation is incomplete, says so,
// and then silently ends anyway (sessions clever-raven-3, amber-quartz:
// `state.StateCompleted` landed right after the stated "Let me..." with nothing
// further ever happening).
//
// Returns the response to finalize: unchanged when no continuation is warranted (no
// trigger phrase, a deny-listed verb, or the user declines), or the re-synthesized
// answer folding in both rounds' findings when one runs. A single bounded round —
// this never recurses into itself again, even if the continuation's OWN response
// also ends with a stated intent, so a chain of "let me"s can't spiral (tidy-cove's
// anti-spiral precedent, applied here at the turn level).
//
// Always succeeds from the caller's point of view: by the time this runs, resp has
// ALREADY been generated successfully, so any failure partway through the
// continuation attempt (the verb-approval prompt interrupted, the continuation's own
// decompose erroring, re-synthesis failing) just abandons the attempt and returns the
// original resp unchanged — it must never retroactively fail a turn that already
// produced a good answer.
func (o *Orchestrator) maybeContinuePlan(ctx context.Context, route, userText, rootID, planCtx, resp string, doThink, ephemeral bool) string {
	verb, sentence, ok := continuation.Detect(resp)
	if !ok {
		return resp
	}

	allowed, _ := continuation.LoadVerbs(o.settings.ContinuationVerbsAllowedPath)
	denied, _ := continuation.LoadVerbs(o.settings.ContinuationVerbsDeniedPath)

	switch {
	case denied[verb]:
		return resp // recognized, deliberately not a trigger — no prompt, no continuation
	case allowed[verb]:
		// proceed to the continuation round below
	default:
		dec, err := o.RequestVerbApproval(ctx, verb, sentence)
		if err != nil || (dec != VerbAllowOnce && dec != VerbAllowAlways) {
			return resp // declined, or interrupted while the prompt was showing
		}
	}

	contCtx := planfindings.WithSource(ctx, func() string { return planCtx })
	extraCtx, handled, perr := o.runPlanPhase(contCtx, sentence, rootID+"-cont")
	if perr != nil || !handled {
		return resp // interrupted, or the continuation itself investigated nothing new
	}

	combined := planCtx + extraCtx
	msgs := o.withContext(o.assembler.AssembleWithThinking(userText+combined, o.thinkingPrompt(doThink), route))
	fallback := o.withContext(o.assembler.Assemble(userText + combined))
	newResp, _, err := o.streamResponse(ctx, msgs, fallback, doThink, ephemeral)
	if err != nil {
		return resp // re-synthesis failed: keep the original (already-streamed) answer
	}
	return newResp
}
