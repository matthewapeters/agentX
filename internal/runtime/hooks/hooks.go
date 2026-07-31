// Package hooks is the extension seam for the main prompt/response loop: a
// synchronous chain that may observe and mutate a turn in place, and an
// asynchronous fan-out that observes a snapshot and runs orthogonal to the
// loop (indexers, summarizers, telemetry). Intent evaluation and
// decomposition/planning strategies are expected to register here in a future
// pass — this package ships the framework only; no hooks are registered yet
// (Orchestrator.Start builds an empty Registry).
package hooks

import (
	"context"

	"agentx/internal/prompting"
)

// Turn is the mutable per-turn state hooks observe and (sync hooks) may
// modify. Messages uses prompting.Message directly (the same shape the model
// call itself consumes) rather than a parallel type — internal/runtime/hooks
// is a runtime subpackage with the same import allowances as its siblings
// decompose/wavefront, which already depend on internal/prompting.
type Turn struct {
	// Prompt is the user's current input text, unset on the post-tool-execution
	// hook point (there is no new user prompt at that point in the loop).
	Prompt string
	// Messages is the full message history for this turn's next model call,
	// including any tool call/result round-trips already appended this turn.
	Messages []prompting.Message
}

// SyncHook runs serially, in registration order, against the live turn. It
// may mutate turn in place; a returned error aborts the remaining sync chain
// for this invocation (the loop surfaces it like any other turn-level error).
type SyncHook interface {
	Run(ctx context.Context, turn *Turn) error
}

// AsyncHook runs orthogonal to the loop, against a value-copy snapshot of the
// turn — Go's value semantics on Turn/Message give the copy-not-reference
// guarantee: an async hook cannot affect the turn that triggered it, only
// write to external state (a session note, an index, a summary) for a later
// turn to pick up.
type AsyncHook interface {
	Run(ctx context.Context, turn Turn)
}

// Registry holds the ordered set of hooks invoked at the loop's two
// hook-invocation points (after a new user prompt, and after tool
// execution).
type Registry struct {
	sync  []SyncHook
	async []AsyncHook
}

// NewRegistry returns an empty Registry. An empty Registry's RunSync/RunAsync
// are no-ops, so callers never need to nil-check it.
func NewRegistry() *Registry {
	return &Registry{}
}

// RegisterSync appends a hook to the synchronous chain, run in registration
// order.
func (r *Registry) RegisterSync(h SyncHook) { r.sync = append(r.sync, h) }

// RegisterAsync appends a hook to the asynchronous fan-out.
func (r *Registry) RegisterAsync(h AsyncHook) { r.async = append(r.async, h) }

// RunSync invokes every sync hook in registration order against turn. The
// first error aborts the remaining chain and is returned.
func (r *Registry) RunSync(ctx context.Context, turn *Turn) error {
	for _, h := range r.sync {
		if err := h.Run(ctx, turn); err != nil {
			return err
		}
	}
	return nil
}

// RunAsync dispatches every async hook against a value copy of turn, each in
// its own goroutine, and returns immediately without waiting — the loop must
// never block on orthogonal work.
func (r *Registry) RunAsync(ctx context.Context, turn Turn) {
	for _, h := range r.async {
		go h.Run(ctx, turn)
	}
}

// spawnDepthKey is the context key tracking how many nested loop-spawns led
// to the current call, so a hook that spawns a recursive loop instance (a
// sub-agent) cannot recurse unboundedly.
type spawnDepthKey struct{}

// SpawnDepth reports the current loop-spawn nesting depth (0 at the top-level
// loop).
func SpawnDepth(ctx context.Context) int {
	d, _ := ctx.Value(spawnDepthKey{}).(int)
	return d
}

// CanSpawn reports whether a hook may spawn another recursive loop instance
// without exceeding max. Sync hooks spawning a blocking sub-loop and async
// hooks spawning a backgrounded one must both check this before spawning —
// async spawns in particular are not serialized against each other, so
// nothing else enforces the cap for them.
func CanSpawn(ctx context.Context, max int) bool {
	return SpawnDepth(ctx) < max
}

// NextSpawnContext returns a context for a hook-spawned recursive loop
// instance, with its spawn depth incremented by one. A hook must derive the
// spawned loop's context from this, not the raw incoming ctx, or CanSpawn's
// guardrail never advances.
func NextSpawnContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, spawnDepthKey{}, SpawnDepth(ctx)+1)
}
