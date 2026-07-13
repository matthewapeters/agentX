package runtime

import (
	"context"
	"fmt"

	"agentx/internal/state"
	"agentx/internal/tools"
)

// outputSizeOptions is the fixed option set every oversized-output recovery
// request offers (TOOL-6) — deliberately not configurable per call site, so the
// surface always shows the same five choices regardless of which tool truncated.
// "refine" (ask the agent to narrow the command itself) is Phase B and not yet
// offered.
var outputSizeOptions = []state.ApprovalOption{
	{Label: "Use the truncated result, just this once", Decision: "use_truncated_once"},
	{Label: "Always use truncated results from this tool", Decision: "use_truncated_always"},
	{Label: "Capture more, just this once", Decision: "expand_once"},
	{Label: "Always capture more from this tool", Decision: "expand_always"},
	{Label: "Abort — don't use this result", Decision: "abort"},
}

// RequestOutputSizeDecision resolves what to do with a tool result the executor
// truncated at its output_max_bytes capture safety net (res.Truncated). A
// persisted per-tool override (set by a prior "always" choice) applies directly,
// with the applied choice still stated in the returned result's Preview — never a
// silent swap, even when the human isn't re-asked. Otherwise it blocks on the
// same decision gate tool-approval and verb-continuation use
// (Orchestrator.RequestDecision), so plan leaves and the interactive cycle share
// one code path with no special-casing (TOOL-6, retracting an earlier "plan
// leaves shouldn't block" assumption — they already do, safely, for approval).
//
// It returns the result to use (unchanged, or re-captured with a larger cap) and
// ok=true when the caller should proceed with it; ok=false means "abort" (treat
// like a policy denial) — either chosen explicitly or because the request was
// interrupted, in which case err is also set so the caller can propagate it the
// same way an interrupted approval does.
func (o *Orchestrator) RequestOutputSizeDecision(ctx context.Context, d tools.Descriptor, args map[string]string, res tools.Result) (tools.Result, bool, error) {
	if ov, ok := o.outputOverrides.Get(d.ID); ok {
		return o.applyOutputOverride(ctx, d, args, res, ov, true), true, nil
	}

	dec, err := o.RequestDecision(ctx, state.PhaseOutputSize, outputSizePromptText(d, args, res), outputSizeOptions)
	if err != nil {
		return tools.Result{}, false, err // interrupted while awaiting
	}
	switch dec {
	case "use_truncated_once":
		return res, true, nil
	case "use_truncated_always":
		o.persistOutputOverride(tools.OutputOverride{Tool: d.ID, Decision: "use_truncated"})
		return res, true, nil
	case "expand_once":
		return o.expandCapture(ctx, d, args, res, o.absoluteMaxBytes(), false), true, nil
	case "expand_always":
		capBytes := o.absoluteMaxBytes()
		o.persistOutputOverride(tools.OutputOverride{Tool: d.ID, Decision: "expand", CapBytes: capBytes})
		return o.expandCapture(ctx, d, args, res, capBytes, false), true, nil
	default: // "abort", or an unrecognized decision from a malformed remote POST
		return tools.Result{}, false, nil
	}
}

// applyOutputOverride resolves a remembered per-tool decision without prompting.
// auto is always true from RequestOutputSizeDecision's one call site (kept as a
// parameter, not hardcoded, so the "remembered preference" note is scoped to the
// exact behavior it documents, not implied by the function name alone).
func (o *Orchestrator) applyOutputOverride(ctx context.Context, d tools.Descriptor, args map[string]string, res tools.Result, ov tools.OutputOverride, auto bool) tools.Result {
	if ov.Decision == "expand" {
		capBytes := ov.CapBytes
		if capBytes <= 0 {
			capBytes = o.absoluteMaxBytes()
		}
		return o.expandCapture(ctx, d, args, res, capBytes, auto)
	}
	if auto {
		res.Preview += fmt.Sprintf("\n…(remembered preference for %s: use truncated results without asking)", d.ID)
	}
	return res
}

// expandCapture re-runs the call once more with a larger byte cap (ceiling-
// clamped to absoluteMaxBytes regardless of what was requested — no interactive
// choice or remembered preference can exceed it) via an ad-hoc executor; the
// shared o.runner keeps the configured default for every other call. Degrades to
// the original (truncated) result if the artifact store or the re-run itself
// fails, rather than losing the result entirely.
func (o *Orchestrator) expandCapture(ctx context.Context, d tools.Descriptor, args map[string]string, orig tools.Result, capBytes int, auto bool) tools.Result {
	ceiling := o.absoluteMaxBytes()
	if capBytes <= 0 || capBytes > ceiling {
		capBytes = ceiling
	}
	art, err := o.store.Artifacts(o.id.ID)
	if err != nil {
		return orig
	}
	bigger := tools.NewExecutor(art, capBytes)
	res, err := bigger.Run(ctx, d, args)
	if err != nil {
		return orig
	}
	if auto {
		res.Preview += fmt.Sprintf("\n…(expanded to %d bytes — remembered preference for %s)", capBytes, d.ID)
	}
	return res
}

// persistOutputOverride records ov in memory and re-saves the full override set —
// mirrors RequestApproval's global-scope persistence (tools.SaveApprovals) for
// the same "always" shape.
func (o *Orchestrator) persistOutputOverride(ov tools.OutputOverride) {
	o.outputOverrides.Set(ov)
	_ = tools.SaveOutputOverrides(o.settings.ToolOutputOverridesPath, o.outputOverrides.All())
}

// absoluteMaxBytes is the configured ceiling, defaulting like the executor's own
// byte cap does when unset.
func (o *Orchestrator) absoluteMaxBytes() int {
	if o.settings.ToolOutputAbsoluteMaxBytes > 0 {
		return o.settings.ToolOutputAbsoluteMaxBytes
	}
	return 2097152
}

// outputSizePromptText renders the decision prompt: what was called and how much
// it captured versus its cap.
func outputSizePromptText(d tools.Descriptor, args map[string]string, res tools.Result) string {
	return fmt.Sprintf("%s output was truncated (%d bytes captured, %d lines). Use it as-is, capture more, or abort?",
		proposalText(d, args), res.Bytes, res.Lines)
}
