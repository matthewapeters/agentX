package runtime

import (
	"context"
	"fmt"
	"strings"

	"agentx/internal/prompting/task"
	"agentx/internal/runtime/decompose"
	"agentx/internal/runtime/wavefront"
	"agentx/internal/state"
)

// runWavefrontPhase is runPlanPhase's ADR 0012 counterpart: drains an imperative
// through wavefront.Scheduler instead of the continuous engine's
// decompose.DrainPlan, selected by Settings.WavefrontEnabled (the branch lives in
// runPlanPhase itself). Returns the exact same (context, handled, error) contract
// the continuous branch does, so callers never need to know which engine ran.
// Reuses planObserver unchanged — it already satisfies scheduler.Observer, the
// same interface wavefront.Scheduler accepts, so no new Observer type exists.
func (o *Orchestrator) runWavefrontPhase(ctx context.Context, text, rootID string) (string, bool, error) {
	o.setProcessing(state.StateWorking, state.PhasePlanning)

	root := task.Record{
		ID: rootID, Goal: text, Type: task.Query, Kind: task.KindStep,
		Status: task.Proposed, Deps: []string{},
		Provenance: task.Provenance{Source: engineWavefront},
	}
	o.publish("TASK_PLAN", state.ContentTaskPlan, map[string]any{
		"root": root.ID, "goal": root.Goal, "phase": "started",
		"nodes": []map[string]any{{
			"task_id": root.ID, "goal": root.Goal, "status": string(root.Status),
			"deps": root.Deps, "kind": string(root.Kind),
			"source": root.Provenance.Source, "origin": root.Provenance.Origin,
		}},
	})

	g := task.NewGraph()
	if err := g.Add(root); err != nil {
		return "", false, err
	}
	s := wavefront.New(g, root.Goal, o.wavefrontClassifier, o.wavefrontChat, o.settings.WavefrontSynthesisPrompt,
		o.taskExec, decompose.DefaultSlots, decompose.DefaultMaxDepth,
		wavefront.WithObserver(&planObserver{o: o, root: root.ID}),
		wavefront.WithCondenser(o.outputSummarizer),
		// o.registry is guaranteed non-nil here: runWavefrontPhase only runs once
		// o.taskExec is set, which buildTaskExecutor only does after
		// o.toolsReady() (requires o.registry != nil) has passed.
		wavefront.WithValidator(o.registry),
	)
	rerr := s.Run(ctx)
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}

	nodes := g.Nodes()
	executed := 0
	for _, n := range nodes {
		if n.Kind == task.KindTask && n.Status == task.Done {
			executed++
		}
	}
	o.publish("TASK_PLAN", state.ContentTaskPlan, planSummary(root, nodes, executed, rerr))

	planCtx := wavefrontPlanContext(root.ID, nodes)
	if strings.TrimSpace(planCtx) == "" {
		return "", false, nil // nothing investigated → answer directly
	}
	return planCtx, true, nil
}

// wavefrontPlanContext renders every resolved (Done or Failed), non-root node's
// Goal/Value/Error as prompt context. Unlike the continuous engine's planContext
// (which only ever sees capturedStep — Task-leaf executions, since capturingExec
// wraps only the executor), this deliberately includes resolved KindStep nodes
// too: wavefront's Step nodes routinely carry the actual meaningful synthesized
// answer (via self-match or the fallback synthesis call), not just raw tool
// output, and excluding them would discard most of what a wavefront-run plan
// actually produced. The root itself is excluded — its resolution is the answer
// the respond call is building toward, not a finding feeding into it.
func wavefrontPlanContext(rootID string, nodes []task.Record) string {
	var findings strings.Builder
	any := false
	for _, n := range nodes {
		if n.ID == rootID {
			continue
		}
		if n.Status != task.Done && n.Status != task.Failed {
			continue
		}
		any = true
		fmt.Fprintf(&findings, "- %s → %s", n.Goal, n.Status)
		if n.Value != "" {
			fmt.Fprintf(&findings, ": %s", n.Value)
		}
		if n.Error != "" {
			fmt.Fprintf(&findings, " (%s)", n.Error)
		}
		findings.WriteByte('\n')
	}
	if !any {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[Investigation — executed steps and findings]\n")
	b.WriteString(findings.String())
	b.WriteString("Answer the request using only these findings.")
	return b.String()
}
