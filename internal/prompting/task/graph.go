package task

import (
	"errors"
	"fmt"
	"slices"
	"sort"
)

// Graph is an in-memory projection of a session's task DAG, folded from the
// append-only task event stream: a task_proposed event is Add, a task_updated event
// is Update. It is the read model the Phase-3 scheduler consumes and owns no I/O —
// the event log is the source of truth, and folding the same event sequence twice
// yields an identical Graph (deterministic replay). Family A emits one node with an
// empty deps set; a decomposer (Phase 4) emits many nodes with edges, and the same
// Graph carries the plan DAG with no schema change.
//
// Phase 1 of ADR 0008. Behavior contract:
// docs/architecture/behavior/adr/0008_task_dag_substrate.feature.md.
type Graph struct {
	nodes   map[string]Record // id -> latest record (latest event wins)
	order   []string          // ids in first-seen order, for deterministic iteration
	nextSeq int               // next Record.Seq to assign on Add (ADR 0012 amendment)
}

// Integrity errors, refused at admission so the projection stays a well-formed DAG.
var (
	// ErrDuplicateID — Add was called for an id already in the graph.
	ErrDuplicateID = errors.New("task: duplicate node id")
	// ErrUnknownNode — Update was called for an id not in the graph.
	ErrUnknownNode = errors.New("task: unknown node id")
	// ErrDanglingDep — a dep references an id not present in the graph.
	ErrDanglingDep = errors.New("task: dangling dependency")
	// ErrCycle — the edge would close a dependency cycle.
	ErrCycle = errors.New("task: dependency cycle")
)

// NewGraph returns an empty graph.
func NewGraph() *Graph {
	return &Graph{nodes: map[string]Record{}}
}

// Add admits a new node. It refuses a duplicate id, a dep referencing an unknown
// node (dangling edge), or a self-edge. Because deps must already exist — nodes are
// admitted in dependency order — a fresh node has no inbound edges and so cannot
// close a cycle on Add; the cycle check is shared with Update for uniformity.
//
// Add assigns rec.Seq itself (overwriting whatever the caller set, if anything) —
// ADR 0012 amendment: a node's growth position is graph-authoritative, never
// caller-supplied, so a serialized graph's growth order can never be gamed or left
// inconsistent by a careless caller.
func (g *Graph) Add(rec Record) error {
	if _, ok := g.nodes[rec.ID]; ok {
		return fmt.Errorf("%w: %s", ErrDuplicateID, rec.ID)
	}
	if err := g.validate(rec); err != nil {
		return err
	}
	rec.Seq = g.nextSeq
	g.nextSeq++
	g.nodes[rec.ID] = rec
	g.order = append(g.order, rec.ID)
	return nil
}

// Update supersedes an existing node with a later event (append-only, latest wins).
// The node must already exist; dep integrity is re-checked so a lifecycle event that
// re-points deps cannot introduce a dangling edge or a cycle. First-seen order is
// preserved, keeping iteration deterministic across replays.
//
// Update always re-inherits the existing node's Seq, discarding whatever Seq (if
// any) the incoming record carries — ADR 0012 amendment. Seq means "when this node
// was first admitted," not "when it was last touched"; a status-transition Update
// carrying a zero-value Record.Seq (the common case — callers are not expected to
// set it) must never silently reset a node's growth position back to 0.
func (g *Graph) Update(rec Record) error {
	existing, ok := g.nodes[rec.ID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownNode, rec.ID)
	}
	if err := g.validate(rec); err != nil {
		return err
	}
	rec.Seq = existing.Seq
	g.nodes[rec.ID] = rec
	return nil
}

// validate enforces referential integrity and acyclicity for a candidate record,
// treating rec.Deps as the authoritative deps for rec.ID (so it is correct for both
// a new node and a re-pointed existing one).
func (g *Graph) validate(rec Record) error {
	for _, d := range rec.Deps {
		if d == rec.ID {
			return fmt.Errorf("%w: %s depends on itself", ErrCycle, rec.ID)
		}
		if _, ok := g.nodes[d]; !ok {
			return fmt.Errorf("%w: %s -> %s", ErrDanglingDep, rec.ID, d)
		}
	}

	// A cycle exists iff rec.ID is reachable from one of its own deps by following
	// the depends-on relation. depsOf overrides rec.ID's stored deps with the
	// candidate's, so the check reflects the edge we are about to apply.
	depsOf := func(id string) []string {
		if id == rec.ID {
			return rec.Deps
		}
		return g.nodes[id].Deps
	}
	visited := map[string]bool{}
	var reaches func(from string) bool
	reaches = func(from string) bool {
		for _, next := range depsOf(from) {
			if next == rec.ID {
				return true
			}
			if !visited[next] {
				visited[next] = true
				if reaches(next) {
					return true
				}
			}
		}
		return false
	}
	if slices.ContainsFunc(rec.Deps, reaches) {
		return fmt.Errorf("%w: through %s", ErrCycle, rec.ID)
	}
	return nil
}

// Len is the node count.
func (g *Graph) Len() int { return len(g.order) }

// Node returns the latest record for id.
func (g *Graph) Node(id string) (Record, bool) {
	r, ok := g.nodes[id]
	return r, ok
}

// Nodes returns every node in first-seen order (deterministic).
func (g *Graph) Nodes() []Record {
	out := make([]Record, 0, len(g.order))
	for _, id := range g.order {
		out = append(out, g.nodes[id])
	}
	return out
}

// Roots returns the ids with no dependencies, in first-seen order.
func (g *Graph) Roots() []string {
	var out []string
	for _, id := range g.order {
		if len(g.nodes[id].Deps) == 0 {
			out = append(out, id)
		}
	}
	return out
}

// Dependents returns the ids whose deps include id, in first-seen order.
func (g *Graph) Dependents(id string) []string {
	var out []string
	for _, nid := range g.order {
		if slices.Contains(g.nodes[nid].Deps, id) {
			out = append(out, nid)
		}
	}
	return out
}

// Ready returns the ids eligible to run: a runnable node (proposed or ready) whose
// every dependency is done. A node with no deps is ready as soon as it is runnable,
// so roots run first. This is the read-only seam the Phase-3 scheduler consumes;
// Phase 1 never acts on it.
func (g *Graph) Ready() []string {
	var out []string
	for _, id := range g.order {
		rec := g.nodes[id]
		if rec.Status != Proposed && rec.Status != Ready {
			continue
		}
		allDone := true
		for _, d := range rec.Deps {
			if g.nodes[d].Status != Done {
				allDone = false
				break
			}
		}
		if allDone {
			out = append(out, id)
		}
	}
	return out
}

// Edges returns every dependency edge as a [prerequisite, dependent] pair, sorted
// for deterministic comparison.
func (g *Graph) Edges() [][2]string {
	var out [][2]string
	for _, id := range g.order {
		for _, d := range g.nodes[id].Deps {
			out = append(out, [2]string{d, id})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}
