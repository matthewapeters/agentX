// Package executor drains a recognized task record into a real, verified effect
// ([E] in cascade_classifier.md). It reuses the command stack — a proposer turns
// the task goal into a concrete tool call, the command policy gates it, the runner
// executes it — and adds the piece the single-pass design lacked: it verifies the
// effect actually happened before anyone reports success. The response never says
// "Created X" unless X demonstrably exists, which permanently retires the
// phantom-success class that started the vivid-willow thread.
//
// The executor is decoupled from the runtime via small interfaces (satisfied by
// the tools package and the orchestrator's runner), so its drain->gate->run->verify
// logic is testable with stubs.
//
// Design: docs/architecture/cascade_classifier.md (§ Execute + verify).
// Behavior contract: tests/features/executor/execute.feature.
package executor

import (
	"context"
	"maps"
	"path/filepath"
	"strings"

	"agentx/internal/prompting/task"
	"agentx/internal/tools"
)

// Proposer turns a task goal into a concrete tool call.
type Proposer interface {
	Propose(ctx context.Context, goal string) (tools.Proposal, bool)
}

// Registry resolves a proposed tool id to its descriptor.
type Registry interface {
	Lookup(id string) (tools.Descriptor, bool)
}

// Gate is the command policy: it decides whether a tool call may run.
type Gate interface {
	Evaluate(d tools.Descriptor, args map[string]string) tools.Verdict
}

// Runner executes an approved tool call.
type Runner interface {
	Run(ctx context.Context, d tools.Descriptor, args map[string]string) (tools.Result, error)
}

// Verifier confirms an executed tool call's effect is real (the file exists, the
// command exited cleanly). It is the anti-phantom-success check.
type Verifier interface {
	Verify(d tools.Descriptor, args map[string]string, res tools.Result) bool
}

// Approver decides whether a call the policy or the working-directory confinement
// flagged may proceed. Reason explains why approval is needed (e.g. "outside
// working directory"). The orchestrator backs this with its interactive approval
// gate; a nil Approver means such a call is surfaced as NeedsApproval and not run.
type Approver interface {
	Approve(ctx context.Context, d tools.Descriptor, args map[string]string, reason string) bool
}

// ApproverFunc adapts a function to the Approver interface.
type ApproverFunc func(ctx context.Context, d tools.Descriptor, args map[string]string, reason string) bool

// Approve implements Approver.
func (f ApproverFunc) Approve(ctx context.Context, d tools.Descriptor, args map[string]string, reason string) bool {
	return f(ctx, d, args, reason)
}

// OutputSizeDecider resolves what to do with a result the runner's output_max_bytes
// safety net truncated (TOOL-6): accept it, re-run with a larger cap, or abort
// (ok=false, treated like a policy denial). The orchestrator backs this with the
// same interactive decision gate tool-approval uses; a nil OutputSizeDecider means
// a truncated result is used as-is, unchanged — today's behavior before TOOL-6.
type OutputSizeDecider interface {
	DecideOutputSize(ctx context.Context, d tools.Descriptor, args map[string]string, res tools.Result) (tools.Result, bool)
}

// OutputSizeDeciderFunc adapts a function to the OutputSizeDecider interface.
type OutputSizeDeciderFunc func(ctx context.Context, d tools.Descriptor, args map[string]string, res tools.Result) (tools.Result, bool)

// DecideOutputSize implements OutputSizeDecider.
func (f OutputSizeDeciderFunc) DecideOutputSize(ctx context.Context, d tools.Descriptor, args map[string]string, res tools.Result) (tools.Result, bool) {
	return f(ctx, d, args, res)
}

// ReadGrants reports whether a resolved absolute path has a standing read grant, so a
// read outside the confinement root need not prompt again. It is how a session-scoped
// "allow reads under X" approval (persisted in working memory) is honored on later
// calls — the executor consults it before treating an out-of-root path as needing
// approval. A nil ReadGrants grants nothing.
type ReadGrants interface {
	Allows(path string) bool
}

// Status is the terminal state of a drain attempt.
type Status string

const (
	// Executed — ran and the effect was verified.
	Executed Status = "executed"
	// Phantom — the runner reported success but the effect could not be verified
	// (the exact failure vivid-willow surfaced). Never reported to the user as done.
	Phantom Status = "phantom"
	// Denied — the command policy blocked the call.
	Denied Status = "denied"
	// NeedsApproval — the call requires interactive approval before it can run.
	NeedsApproval Status = "needs_approval"
	// NoTool — no usable tool was proposed for the goal.
	NoTool Status = "no_tool"
	// Failed — the runner returned an error.
	Failed Status = "failed"
)

// Outcome is the result of draining one task record.
type Outcome struct {
	Status Status
	Result tools.Result
	Reason string
}

// CallObserver receives per-call lifecycle callbacks so tool execution is user-visible
// while it happens (ADR 0009): the resolved tool + args before the run, and the result
// after — for every attempt, including denied ones, so a blocked call is as legible as a
// successful one. Callbacks are synchronous on the Execute goroutine.
type CallObserver interface {
	// ToolCalled fires once the proposal resolved to a known tool, before gate/run.
	ToolCalled(rec task.Record, d tools.Descriptor, args map[string]string)
	// ToolFinished fires at the call's terminal point with its outcome status; res is
	// the zero Result when the call never ran (denied / needs approval).
	ToolFinished(rec task.Record, d tools.Descriptor, res tools.Result, status Status)
}

// Executor drains task records into verified tool calls.
type Executor struct {
	proposer   Proposer
	registry   Registry
	gate       Gate
	runner     Runner
	verify     Verifier
	root       string            // working directory calls are confined to ("" = no boundary)
	approve    Approver          // grants calls that need approval (policy or confinement)
	reads      ReadGrants        // standing out-of-root read grants (nil = none)
	observer   CallObserver      // per-call visibility seam (nil = silent)
	sizeDecide OutputSizeDecider // resolves output_max_bytes truncation (nil = use as-is)
}

// Option configures an Executor.
type Option func(*Executor)

// WithRoot confines execution to a working directory: a call whose file target
// resolves outside root requires approval before it runs.
func WithRoot(root string) Option {
	return func(e *Executor) { e.root = root }
}

// WithApprover supplies the interactive approval seam for flagged calls. Without
// it, a call needing approval is surfaced as NeedsApproval and not run.
func WithApprover(a Approver) Option {
	return func(e *Executor) { e.approve = a }
}

// WithReadGrants supplies standing out-of-root read grants: a path they allow is read
// without a fresh approval prompt, honoring a session-scoped "allow reads under X"
// decision. It only widens read access; it never permits a mutating call.
func WithReadGrants(g ReadGrants) Option {
	return func(e *Executor) { e.reads = g }
}

// WithCallObserver attaches the per-call visibility seam (nil is allowed = silent).
func WithCallObserver(obs CallObserver) Option {
	return func(e *Executor) { e.observer = obs }
}

// WithOutputSizeDecider supplies the oversized-output recovery seam (TOOL-6).
// Without it, a truncated result is used as-is, unchanged (pre-TOOL-6 behavior).
func WithOutputSizeDecider(d OutputSizeDecider) Option {
	return func(e *Executor) { e.sizeDecide = d }
}

// New builds an executor from its collaborators.
func New(p Proposer, reg Registry, g Gate, r Runner, v Verifier, opts ...Option) *Executor {
	e := &Executor{proposer: p, registry: reg, gate: g, runner: r, verify: v}
	for _, o := range opts {
		o(e)
	}
	return e
}

// escapesRoot reports whether the call's file target resolves outside the confinement
// root, requiring approval. No root or no path means no boundary applies. A standing
// read grant (WithReadGrants) that covers the target suppresses the escape for
// read-risk calls only — a grant widens reads, never a mutating call.
func (e *Executor) escapesRoot(d tools.Descriptor, args map[string]string) bool {
	if e.root == "" {
		return false
	}
	p := args["path"]
	if p == "" {
		return false
	}
	full := p
	if !filepath.IsAbs(p) {
		full = filepath.Join(e.root, p)
	}
	full = filepath.Clean(full)
	rel, err := filepath.Rel(e.root, full)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false // within root
	}
	if d.Risk == tools.RiskRead && e.reads != nil && e.reads.Allows(full) {
		return false // standing read grant covers this out-of-root read
	}
	return true
}

// resolvedProposal reads a pre-resolved tool call from rec.Params (ADR 0008 amendment: a
// planner-produced Task carries {"tool","args"} directly, resolved at plan-generation time
// under JSON-schema-constrained decoding — see internal/prompting/planner). ok is false for
// any record without one (Redispatch/Reify/Verify-route records, which only ever carry a
// Goal), so Execute falls back to the proposer unchanged for those. Args values are
// stringified with tools.StringifyArg because Params round-trips through JSON (session
// persistence), so a value that started as map[string]string may arrive back as
// map[string]any/map[string]interface{} — never assume the original Go type survived.
func resolvedProposal(rec task.Record) (tools.Proposal, bool) {
	toolID, ok := rec.Params["tool"].(string)
	if !ok || toolID == "" {
		return tools.Proposal{}, false
	}
	args := map[string]string{}
	switch m := rec.Params["args"].(type) {
	case map[string]string:
		maps.Copy(args, m)
	case map[string]any:
		for k, v := range m {
			args[k] = tools.StringifyArg(v)
		}
	}
	return tools.Proposal{Tool: toolID, Args: args}, true
}

// Execute drains one task record: it uses a pre-resolved tool call when the record carries
// one (a planner-produced Task), otherwise proposes a concrete tool call for the task's
// goal, gates it through policy, runs it, resolves any output_max_bytes truncation
// (TOOL-6), and verifies the effect. A denied or approval-pending call does not run; a
// truncated result declined at the size-decision seam is Denied, same as a policy
// decline; a run whose effect fails verification is reported as Phantom, never as done.
func (e *Executor) Execute(ctx context.Context, rec task.Record) Outcome {
	prop, ok := resolvedProposal(rec)
	if !ok {
		prop, ok = e.proposer.Propose(ctx, rec.Goal)
	}
	if !ok {
		return Outcome{Status: NoTool, Reason: "no tool proposed for goal"}
	}
	d, found := e.registry.Lookup(prop.Tool)
	if !found {
		return Outcome{Status: NoTool, Reason: "unknown tool " + prop.Tool}
	}

	// The call is resolved: announce it before anything runs, and report however it
	// ends (ADR 0009 — execution is visible, including denied attempts).
	if e.observer != nil {
		e.observer.ToolCalled(rec, d, prop.Args)
	}
	finish := func(out Outcome) Outcome {
		if e.observer != nil {
			e.observer.ToolFinished(rec, d, out.Result, out.Status)
		}
		return out
	}

	verdict := e.gate.Evaluate(d, prop.Args)
	if verdict.Decision == tools.Deny {
		return finish(Outcome{Status: Denied, Reason: verdict.Reason})
	}

	// A call needs approval when the policy asks for it, or when it would operate
	// outside the working directory. Confinement never silently escapes the cwd.
	needApproval, reason := verdict.Decision == tools.NeedsApproval, verdict.Reason
	if e.escapesRoot(d, prop.Args) {
		needApproval, reason = true, "outside working directory"
	}
	if needApproval {
		if e.approve == nil {
			return finish(Outcome{Status: NeedsApproval, Reason: reason})
		}
		if !e.approve.Approve(ctx, d, prop.Args, reason) {
			return finish(Outcome{Status: Denied, Reason: "declined: " + reason})
		}
	}

	res, err := e.runner.Run(ctx, d, prop.Args)
	if err != nil {
		return finish(Outcome{Status: Failed, Result: res, Reason: err.Error()})
	}
	if res.Truncated && e.sizeDecide != nil {
		newRes, ok := e.sizeDecide.DecideOutputSize(ctx, d, prop.Args, res)
		if !ok {
			return finish(Outcome{Status: Denied, Reason: "declined: output truncated"})
		}
		res = newRes
	}
	if !e.verify.Verify(d, prop.Args, res) {
		return finish(Outcome{Status: Phantom, Result: res, Reason: "effect not verified"})
	}
	return finish(Outcome{Status: Executed, Result: res})
}
