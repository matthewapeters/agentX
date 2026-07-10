// Package planfindings threads one decomposition plan's live, accumulating findings
// (what earlier sibling/dependency leaves in THIS plan have already discovered)
// through to the decomposer and tool proposer — the fix for a recurring gap
// (mellow-meadow, lively-raven, quiet-cove, amber-quartz): a Task's args and a
// Step's children were generated from the parent goal alone, with zero visibility
// into what an earlier step in the SAME plan actually found. A real tree call would
// run successfully, and moments later a sibling's tool call would still get resolved
// as a hallucinated `cat src/main.py` in a Go project — the proposer never looked.
//
// This is carried via context.Context, not a struct field, because both the
// decomposer (internal/runtime/decompose.Decomposer) and the tool proposer
// (internal/tools.Proposer) are shared, session-lifetime singletons that can serve
// more than one plan drain over a session's life — including concurrently, since
// runDecomposition is deliberately backgrounded so a turn is never blocked. A shared
// mutable field would race or cross-contaminate between two plans; ctx already flows
// through every relevant call (Decompose, Execute, Propose) and scopes correctly to
// exactly the one drain that attached it.
package planfindings

import "context"

type ctxKey struct{}

// Source returns the current plan's findings-so-far as a context-ready string, or ""
// if nothing has completed yet. Called fresh on each read, so later calls during the
// same drain see strictly more than earlier ones as the scheduler completes leaves.
type Source func() string

// WithSource attaches a plan's live findings source to ctx, for everything the
// scheduler dispatches during that plan's drain to read via From.
func WithSource(ctx context.Context, src Source) context.Context {
	return context.WithValue(ctx, ctxKey{}, src)
}

// From reads the current findings snapshot from ctx. Returns "" when no source is
// attached (outside a plan drain — the single_tool cycle, or a decomposer/proposer
// called directly in a test) or when the plan hasn't completed any leaves yet.
func From(ctx context.Context) string {
	src, _ := ctx.Value(ctxKey{}).(Source)
	if src == nil {
		return ""
	}
	return src()
}
