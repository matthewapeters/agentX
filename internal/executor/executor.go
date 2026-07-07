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

// Executor drains task records into verified tool calls.
type Executor struct {
	proposer Proposer
	registry Registry
	gate     Gate
	runner   Runner
	verify   Verifier
	root     string     // working directory calls are confined to ("" = no boundary)
	approve  Approver   // grants calls that need approval (policy or confinement)
	reads    ReadGrants // standing out-of-root read grants (nil = none)
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

// Execute drains one task record: it proposes a concrete tool call for the task's
// goal, gates it through policy, runs it, and verifies the effect. A denied or
// approval-pending call does not run; a run whose effect fails verification is
// reported as Phantom, never as done.
func (e *Executor) Execute(ctx context.Context, rec task.Record) Outcome {
	prop, ok := e.proposer.Propose(ctx, rec.Goal)
	if !ok {
		return Outcome{Status: NoTool, Reason: "no tool proposed for goal"}
	}
	d, found := e.registry.Lookup(prop.Tool)
	if !found {
		return Outcome{Status: NoTool, Reason: "unknown tool " + prop.Tool}
	}

	verdict := e.gate.Evaluate(d, prop.Args)
	if verdict.Decision == tools.Deny {
		return Outcome{Status: Denied, Reason: verdict.Reason}
	}

	// A call needs approval when the policy asks for it, or when it would operate
	// outside the working directory. Confinement never silently escapes the cwd.
	needApproval, reason := verdict.Decision == tools.NeedsApproval, verdict.Reason
	if e.escapesRoot(d, prop.Args) {
		needApproval, reason = true, "outside working directory"
	}
	if needApproval {
		if e.approve == nil {
			return Outcome{Status: NeedsApproval, Reason: reason}
		}
		if !e.approve.Approve(ctx, d, prop.Args, reason) {
			return Outcome{Status: Denied, Reason: "declined: " + reason}
		}
	}

	res, err := e.runner.Run(ctx, d, prop.Args)
	if err != nil {
		return Outcome{Status: Failed, Result: res, Reason: err.Error()}
	}
	if !e.verify.Verify(d, prop.Args, res) {
		return Outcome{Status: Phantom, Result: res, Reason: "effect not verified"}
	}
	return Outcome{Status: Executed, Result: res}
}
