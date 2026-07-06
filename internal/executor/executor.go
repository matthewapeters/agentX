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
}

// New builds an executor from its collaborators.
func New(p Proposer, reg Registry, g Gate, r Runner, v Verifier) *Executor {
	return &Executor{proposer: p, registry: reg, gate: g, runner: r, verify: v}
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
	switch verdict.Decision {
	case tools.Deny:
		return Outcome{Status: Denied, Reason: verdict.Reason}
	case tools.NeedsApproval:
		return Outcome{Status: NeedsApproval, Reason: verdict.Reason}
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
