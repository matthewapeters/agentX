package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PlanTree is the durable, fully-nested snapshot of one decomposition plan — the
// queryable "what ran, in what order, with what result refs" companion to the
// append-only event log (ADR 0009 §9b). Rewritten in full on every mutation; plans
// are small, so this is cheap. Deliberately resumability-friendly (durable node
// states + result refs + full tree shape) without any reconstruction logic here —
// that remains a separate, later feature.
type PlanTree struct {
	RootID    string                   `json:"root_id"`
	Goal      string                   `json:"goal"`
	Nodes     map[string]*PlanTreeNode `json:"nodes"`
	UpdatedAt int64                    `json:"updated_at"`
}

// PlanTreeNode is one node's durable state: its own fields (kind/status/timing) plus
// explicit tree edges. Children is decomposition containment (this node's own kids,
// in order); Deps is the sibling "waits-on" set captured at the moment of decompose —
// kept distinct from Children because the scheduler's own Record.Deps conflates
// "structural children" with "sibling wait-on" once it appends childIDs onto the
// parent post-join.
type PlanTreeNode struct {
	ID     string `json:"id"`
	Goal   string `json:"goal"`
	Kind   string `json:"kind"`           // "step" | "task"
	Status string `json:"status"`         // dispatched | decomposed | done | failed | abstained | cancelled
	Type   string `json:"type,omitempty"` // task.Type, Task nodes only

	Deps     []string `json:"deps,omitempty"`     // sibling waits-on
	Children []string `json:"children,omitempty"` // decomposition containment, ordered

	DispatchedAt int64 `json:"dispatched_at,omitempty"`
	CompletedAt  int64 `json:"completed_at,omitempty"`

	Command       string `json:"command,omitempty"`
	ResultText    string `json:"result_text,omitempty"`
	ResultOutcome string `json:"result_outcome,omitempty"`
	ResultRef     string `json:"result_ref,omitempty"`
}

// Plans persists decomposition plan snapshots under a session's plans/ directory —
// the queryable post-session-review companion to the event log (ADR 0009 §9b).
type Plans struct {
	dir string
}

// Plans returns the plan store for a session id, creating its directory.
func (s *Store) Plans(id string) (*Plans, error) {
	dir := filepath.Join(s.Dir(id), "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create plans dir: %w", err)
	}
	return &Plans{dir: dir}, nil
}

// Write materializes tree to plans/<rootID>.json, replacing it atomically (a
// temp-file-then-rename) so a concurrent reader never observes a torn write — this
// rewrites on every plan mutation, so partial writes would otherwise be common.
func (p *Plans) Write(rootID string, tree *PlanTree) error {
	data, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan tree: %w", err)
	}
	dst := filepath.Join(p.dir, rootID+".json")
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write plan tree: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("replace plan tree: %w", err)
	}
	return nil
}
