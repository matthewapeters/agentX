package wavefront

import (
	"fmt"
	"slices"
	"strings"

	"agentx/internal/prompting/task"
	"agentx/internal/runtime/scheduler"
)

// applyClassify processes a completed classify response for node id: folds Knows
// (this is also the self-match mechanism — see findExistingNode, no separate check
// exists), registers/converges Needs, and decides id's own fate — resolved now, or
// awaiting its (possibly just-grown) deps. Main-loop only.
//
// Surface visibility (ADR 0012 amendment, 2026-07-17): every freshly created child
// is reported to the observer via the same NodeDecomposed callback the continuous
// engine uses, so the output/context plan widget's nested-box recursion — which
// only ever descends into a node's recorded Children — actually has children to
// descend into for a wavefront plan. Before this, wavefront never called
// NodeDecomposed at all, so no node past the root ever rendered: a real, previously
// unnoticed gap, not a design choice. A converged Need (wired onto an
// already-existing node instead of a fresh one) is reported separately via
// NodeConverged, since it is not this node's decomposition containment — see
// ConvergenceObserver's doc comment.
func (s *Scheduler) applyClassify(id string, depth map[string]int, result Result, awaiting map[string]bool) {
	for _, know := range result.Knows {
		if s.registerOrConvergeKnow(know) == id {
			// Resolved via a Know matching id's own goal (verbatim, normalized) —
			// the same findExistingNode scan that handles every other convergence.
			// Needs from the same response are discarded: a node that just
			// resolved has nothing left to wait on (ADR 0012 §4).
			return
		}
	}

	childIdx := 0
	var fresh []task.Record
	for _, need := range result.Needs {
		childID, isNew := s.registerOrConvergeNeed(id, depth[id]+1, need, depth, &childIdx)
		if childID == "" {
			continue
		}
		child, ok := s.graph.Node(childID)
		if !ok {
			continue
		}
		if isNew {
			fresh = append(fresh, child)
		} else if co, ok := s.observer.(scheduler.ConvergenceObserver); ok {
			co.NodeConverged(id, child)
		}
	}
	if len(fresh) > 0 && s.observer != nil {
		if rec, ok := s.graph.Node(id); ok {
			s.observer.NodeDecomposed(rec, fresh)
		}
	}

	rec, ok := s.graph.Node(id)
	if !ok {
		return
	}
	if match, ok := findExistingNode(s.graph, rec.Goal); ok && match.Status == task.Done && match.ID != id {
		s.setStatus(id, task.Done, match.Value, "")
		return
	}
	awaiting[id] = true
}

// registerOrConvergeKnow folds one Know into the graph and returns the id of the
// node it resolved or created (ADR 0012 §4):
//   - a match on an already-Done node is a no-op (already known);
//   - a match on an open node resolves it now with this value — this is also how a
//     dispatching node resolves itself, when a Know's name echoes its own goal;
//   - no match creates a new, already-Done, free-standing node (the "incidental
//     synthesis" case — a fact nobody was explicitly asking about, preserved for
//     future dispatches to find via the same mechanism, matching totAlX's
//     unconditional add_known()).
//
// Main-loop only.
func (s *Scheduler) registerOrConvergeKnow(know Know) string {
	if existing, ok := findExistingNode(s.graph, know.Name); ok {
		if existing.Status != task.Done {
			s.setStatus(existing.ID, task.Done, know.Value, "")
		}
		return existing.ID
	}
	id := s.freshID(know.Name)
	rec := task.Record{
		ID: id, Goal: know.Name, Type: task.Query, Kind: task.KindStep,
		Status: task.Done, Value: know.Value,
		Provenance: task.Provenance{Source: "wavefront", Origin: "know"},
	}
	if err := s.graph.Add(rec); err != nil {
		return "" // defensively skip a genuinely unrepresentable fact rather than crash
	}
	return id
}

// registerOrConvergeNeed wires need onto parentID's Deps — either a brand-new
// child (command-valued Needs always, deferred dedup per the ADR 0012 addendum; or
// open-value Needs with no existing match) or an edge onto an existing node found
// via findExistingNode (open-value Needs only). A convergence that would close a
// cycle (the existing node is one of parentID's own ancestors) is rejected by
// task.Graph's existing, already-tested validate — this one Need is simply
// skipped, never wired, and does not fail the parent (ADR 0012 §5; mirrors
// totAlX's "reject rather than wiring the edge, continue"). Main-loop only.
//
// Returns the wired child's id (empty when nothing was wired — an unrepresentable
// Need, a self-reference, an already-present edge, or a rejected cycle) and
// whether that child was freshly created (isNew) as opposed to an existing node
// the edge converged onto — the caller (applyClassify) uses this to decide between
// reporting NodeDecomposed (fresh) and NodeConverged (converged), so a wavefront
// plan's surface rendering can tell "my own new child" apart from "a reference to
// work already in flight elsewhere" (ADR 0012 amendment, surface-visibility
// follow-up).
func (s *Scheduler) registerOrConvergeNeed(parentID string, childDepth int, need Need, depth map[string]int, childIdx *int) (string, bool) {
	var childID string
	isNew := false
	if need.Command != nil {
		childID = s.nextChildID(parentID, childIdx)
		if err := s.graph.Add(newCommandRecord(childID, need)); err != nil {
			return "", false
		}
		depth[childID] = childDepth
		isNew = true
	} else {
		if existing, ok := findExistingNode(s.graph, need.Name); ok {
			childID = existing.ID
		} else {
			childID = s.nextChildID(parentID, childIdx)
			if err := s.graph.Add(newQuestionRecord(childID, need.Name)); err != nil {
				return "", false
			}
			depth[childID] = childDepth
			isNew = true
		}
	}

	if childID == "" || childID == parentID {
		return "", false // an empty/unrepresentable Need, or a Need naming its own parent
	}
	parent, ok := s.graph.Node(parentID)
	if !ok || slices.Contains(parent.Deps, childID) {
		return "", false // already a dep — nothing to add (this is also the
		// convergence case: two Needs in the same response naming the same thing
		// wire to it once, and only the first is reported)
	}
	parent.Deps = append(parent.Deps, childID)
	if err := s.graph.Update(parent); err != nil {
		// task.ErrCycle (converging onto parentID's own ancestor) or another
		// integrity error — skip this one Need rather than failing the whole node.
		return "", false
	}
	return childID, isNew
}

// newCommandRecord builds a task.KindTask child record for a command-valued Need,
// matching planner.Parse's exact task-node shape ({"tool","args"} under Params) so
// the same executor/catalog path handles both engines' resolved calls identically.
func newCommandRecord(id string, need Need) task.Record {
	args := make(map[string]any, len(need.Command.Args))
	for k, v := range need.Command.Args {
		args[k] = v
	}
	return task.Record{
		ID: id, Goal: need.Name, Type: task.Query, Kind: task.KindTask,
		Status: task.Proposed, Deps: []string{},
		Params:     map[string]any{"tool": need.Command.Tool, "args": args},
		Provenance: task.Provenance{Source: "wavefront", Origin: "need"},
	}
}

// newQuestionRecord builds a task.KindStep child record for an open-value Need —
// a further question, classified in turn under the same rules and depth cap.
func newQuestionRecord(id, goal string) task.Record {
	return task.Record{
		ID: id, Goal: goal, Type: task.Query, Kind: task.KindStep,
		Status: task.Proposed, Deps: []string{},
		Provenance: task.Provenance{Source: "wavefront", Origin: "need"},
	}
}

// nextChildID mints a session-unique, namespaced child id (parentID-N), scoped to
// one classify response's own Needs — mirrors planner.Parse's exact namespacing
// convention.
func (s *Scheduler) nextChildID(parentID string, idx *int) string {
	*idx++
	return fmt.Sprintf("%s-%d", parentID, *idx)
}

// freshID mints an id for a free-standing node created directly from an
// unprompted Know (§ registerOrConvergeKnow) — namespaced under the graph's own
// growth counter (Seq-adjacent but independent, since Seq is graph-assigned only
// on Add) rather than under any particular parent, since an incidental fact has no
// single owning node.
func (s *Scheduler) freshID(name string) string {
	return fmt.Sprintf("wf-%d-%s", len(s.graph.Nodes()), sanitizeID(name))
}

// sanitizeID collapses name into a short, id-safe fragment — cosmetic only (the
// graph's real identity guarantee is Add's duplicate-id rejection, not this), so a
// crude truncation/replacement is sufficient.
func sanitizeID(name string) string {
	name = strings.ToLower(strings.Join(strings.Fields(name), "-"))
	if len(name) > 24 {
		name = name[:24]
	}
	if name == "" {
		return "fact"
	}
	return name
}

// normalize is the shared discipline behind every convergence decision in this
// package: trim, collapse internal whitespace, casefold. A compliant model rarely
// echoes text back byte-for-byte, so exact-match-after-normalization is the
// deliberately conservative choice (never fuzzy/semantic similarity) — a missed
// convergence costs only efficiency, but a wrong one would silently answer the
// wrong question (ADR 0012 §3's false-positive asymmetry).
func normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// findExistingNode scans the graph for a node whose normalized Goal matches name —
// the single mechanism behind both Know registration and Need convergence.
// Main-loop only.
func findExistingNode(g *task.Graph, name string) (task.Record, bool) {
	norm := normalize(name)
	if norm == "" {
		return task.Record{}, false
	}
	for _, rec := range g.Nodes() {
		if normalize(rec.Goal) == norm {
			return rec, true
		}
	}
	return task.Record{}, false
}

// renderWM renders the current working-memory snapshot for a classify/synthesis
// prompt: every Done node's Goal/Value as a known fact, plus the goals of every
// still-open node as "THINGS TO BE CONSIDERED" — the prompt-level convergence hint
// (ADR 0012 §3's preventable regime; merge-time existence checks are the
// unconditional backstop regardless of whether this hint lands in time).
// Main-loop only — never called from a worker goroutine (ADR 0012 §3).
func (s *Scheduler) renderWM() string {
	var facts, open strings.Builder
	for _, rec := range s.graph.Nodes() {
		switch rec.Status {
		case task.Done:
			fmt.Fprintf(&facts, "%s: %s\n", rec.Goal, rec.Value)
		case task.Proposed, task.Ready, task.InProgress:
			fmt.Fprintf(&open, "- %s\n", rec.Goal)
		}
	}
	var b strings.Builder
	b.WriteString(facts.String())
	if open.Len() > 0 {
		b.WriteString("\nTHINGS TO BE CONSIDERED (already open elsewhere in this investigation — reuse the name exactly if this is the same question):\n")
		b.WriteString(open.String())
	}
	return b.String()
}
