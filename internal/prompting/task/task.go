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

// Status is the task lifecycle state. proposed/abstained are the classifier-side
// states; ready/running/done/failed are added when the executor ([E]) lands.
type Status string

const (
	// Proposed — freshly classified, awaiting reconciliation/execution.
	Proposed Status = "proposed"
	// Abstained — classification was not confident enough (scattered vote); the
	// uncertainty is recorded durably so the caller asks rather than guesses.
	Abstained Status = "abstained"
)

// Provenance records how a task was classified, so a decision is answerable from
// the event log.
type Provenance struct {
	Source     string         `json:"source"` // turn | response | reconciler
	Escalated  bool           `json:"escalated"`
	Confidence float64        `json:"confidence"`
	Spread     map[string]int `json:"spread,omitempty"`
}

// Record is a durable, DAG-node-shaped task. Its current state is the latest event
// for its ID; deps are empty in Family A and an edge list in Family B.
type Record struct {
	ID         string         `json:"id"`
	Goal       string         `json:"goal"`
	Type       Type           `json:"type"`
	Status     Status         `json:"status"`
	Params     map[string]any `json:"params,omitempty"`
	Deps       []string       `json:"deps"`
	Provenance Provenance     `json:"provenance"`
}

// FromAction turns an action-classifier decision into a proposed task record.
//
// It returns ok=false when the turn produced no action (verdict "none"/""), so a
// pure-conversation turn emits nothing. An abstained decision still emits a record
// (status "abstained") so the uncertainty survives and the caller can ask. escalated
// is the cascade's Tier-1→Tier-2 escalation flag, carried into provenance.
func FromAction(id, goal string, action fanout.Decision, escalated bool) (Record, bool) {
	rec := Record{
		ID:   id,
		Goal: goal,
		Type: Type(action.Verdict),
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
