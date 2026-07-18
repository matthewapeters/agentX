// Package task is the durable task record the classifier emits and a future
// executor drains — the storage-layer fix for the vivid-willow failure: an action
// the model commits to must survive outside the conversation, in a form a separate
// pass can execute. A record is DAG-node-shaped (deps, provenance) from day one so
// today's single task grows into Family B's plan DAG with no redesign.
//
// The emitter derives a record from the action classifier's decision alone, so this
// package depends only on internal/llm/fanout — not the pipeline.
//
// Design: docs/architecture/task_record.md, cascade_classifier.md (§ task record).
// Behavior contract: tests/features/prompting/task_record.feature.
package task

import "agentx/internal/llm/fanout"

// Type selects the executor path and policy gate.
type Type string

const (
	Artifact Type = "artifact"
	Command  Type = "command"
	Query    Type = "query"
)

// Kind selects whether a DAG node decomposes (Step) or executes (Task) — ADR 0008's
// typed-node amendment. Every node — root included — carries one; there is no third kind
// and no bare/untyped node. Declared by the planner at generation time, never inferred by
// the scheduler, so a tool-call loop is structurally impossible: a Task has no decompose
// operation to call.
type Kind string

const (
	// KindStep is a plan node: Decompose() only, never executes directly.
	KindStep Kind = "step"
	// KindTask is a tool node: Execute()/Results() only, never decomposes.
	KindTask Kind = "task"
)

// Status is the task lifecycle state. proposed/abstained are the classifier-side
// states; ready/running/done/failed are added when the executor ([E]) lands.
type Status string

const (
	// Proposed — freshly classified, awaiting reconciliation/execution.
	Proposed Status = "proposed"
	// Abstained — classification was not confident enough (scattered vote); the
	// uncertainty is recorded durably so the caller asks rather than guesses.
	Abstained Status = "abstained"
	// Ready — reconciled for execution with every dep done; eligible to drain.
	Ready Status = "ready"
	// InProgress — the executor drained it and issued the tool call.
	InProgress Status = "in_progress"
	// Done — executed and the effect verified; only now may success be reported.
	Done Status = "done"
	// Failed — execution errored or the effect could not be verified.
	Failed Status = "failed"
	// Denied — the command was blocked by policy or declined by the user; distinct
	// from Failed (a genuine execution error) so plan reporting never conflates a
	// deliberate decision with a bug (RCA: session nimble-pebble-2 — task-565-1's
	// denied git_status call was indistinguishable from a crash in the plan's
	// terminal report).
	Denied Status = "denied"
	// Cancelled — superseded by a later turn or explicitly withdrawn.
	Cancelled Status = "cancelled"
)

// Provenance records how a task was classified, so a decision is answerable from
// the event log.
type Provenance struct {
	// Source is overloaded by convention across two unrelated call sites: the
	// action classifier stamps "turn" | "response" | "reconciler" (FromAction,
	// below); the two decomposition engines instead stamp which one produced the
	// node — "wavefront" (internal/runtime/wavefront/merge.go) or "planner"
	// (internal/prompting/planner/planner.go), empty for either engine's own
	// plan root. Never both meanings on the same record in practice, since a
	// decomposition child is never also an action-classifier output.
	Source     string         `json:"source"`
	Escalated  bool           `json:"escalated"`
	Confidence float64        `json:"confidence"`
	Spread     map[string]int `json:"spread,omitempty"`
	// Origin names the classification move that produced this node, distinct
	// from Source (which engine) and Kind (execution mechanics: does this node
	// run a command, or get classified/decomposed further). One of "know" /
	// "need" (wavefront) or "step" / "action" (the continuous engine's
	// planner), empty for either engine's own plan root. This is the field that
	// makes a plan's solving *strategy* legible from its serialized form: a plan
	// built entirely from step/action nodes is a chain-of-thought decomposition;
	// one built from know/need nodes (especially with ConvergesTo edges) is a
	// tree-of-thought search that branched and converged (ADR 0012).
	Origin string `json:"origin,omitempty"`
}

// Record is a durable, DAG-node-shaped task. Its current state is the latest event
// for its ID; deps are empty in Family A and an edge list in Family B.
type Record struct {
	ID         string         `json:"id"`
	Goal       string         `json:"goal"`
	Type       Type           `json:"type"`
	Kind       Kind           `json:"kind"`
	Status     Status         `json:"status"`
	Params     map[string]any `json:"params,omitempty"`
	Deps       []string       `json:"deps"`
	Provenance Provenance     `json:"provenance"`
	// Value is the node's resolved fact or answer text, set once Status becomes
	// Done. Makes the graph a complete, serializable record of what was learned —
	// not just what was attempted — with no separate structure to keep in sync
	// (ADR 0012 amendment). Engine-agnostic: populated by whichever engine's
	// processing actually produced a resolved value for this node, under the same
	// rule either way.
	Value string `json:"value,omitempty"`
	// Error is the failure reason, set once Status becomes Failed. Persisted
	// alongside Value so a failed node's graph entry shows *why*, not just *that*
	// (ADR 0012 amendment).
	Error string `json:"error,omitempty"`
	// Seq is the node's position in the graph's growth — assigned once by
	// Graph.Add at admission and never touched by Graph.Update, independent of
	// Deps depth or dispatch concurrency (ADR 0012 amendment). Never set by a
	// caller directly; Graph owns it.
	Seq int `json:"seq"`
}

// FromAction turns an action-classifier decision into a proposed task record.
//
// It returns ok=false when the turn produced no action (verdict "none"/""), so a
// pure-conversation turn emits nothing. An abstained decision still emits a record
// (status "abstained") so the uncertainty survives and the caller can ask. escalated
// is the cascade's Tier-1→Tier-2 escalation flag, carried into provenance.
//
// Kind defaults to KindTask: this record is normally drained directly (Redispatch/
// Reify/Verify), never decomposed. The one call site that instead routes it through the
// scheduler as a plan root (the Decompose reconcile route) overrides Kind to KindStep —
// the reconciler has already judged the goal multi-step by the time it gets there.
func FromAction(id, goal string, action fanout.Decision, escalated bool) (Record, bool) {
	rec := Record{
		ID:   id,
		Goal: goal,
		Type: Type(action.Verdict),
		Kind: KindTask,
		Deps: []string{},
		Provenance: Provenance{
			Source:     "turn",
			Escalated:  escalated,
			Confidence: action.Confidence,
			Spread:     action.Spread,
		},
	}

	if action.Abstained {
		rec.Status = Abstained
		return rec, true
	}
	if action.Verdict == "" || action.Verdict == "none" {
		return Record{}, false
	}
	rec.Status = Proposed
	return rec, true
}
