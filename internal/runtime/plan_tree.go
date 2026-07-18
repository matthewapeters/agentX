package runtime

import (
	"sync"
	"time"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/session"
	"agentx/internal/tools"
)

// planTreeRegistry builds the durable, fully-nested PlanTree (ADR 0009 §9b) for each
// root currently draining in this session and rewrites it to disk on every mutation.
// It is fed from the same callback sites already firing for the streamed task_plan/
// task_node/tool_call/tool_result events (planObserver, taskToolPublisher) — richer
// data is available here than what's projected onto the bus (e.g. a Task's resolved
// tool id, a leaf's result ref), so this is not a re-derivation of the wire payloads.
//
// ownerOf maps a leaf task id to its root, so taskToolPublisher — which only ever
// sees one task.Record at a time, with no root pointer — can find the tree to update.
// Best-effort throughout: a write failure never disturbs the plan itself (mirrors
// artifactStore()'s degrade-gracefully precedent) — this is a post-session review
// artifact, not load-bearing for execution.
type planTreeRegistry struct {
	mu      sync.Mutex
	trees   map[string]*session.PlanTree
	ownerOf map[string]string
}

func newPlanTreeRegistry() *planTreeRegistry {
	return &planTreeRegistry{
		trees:   map[string]*session.PlanTree{},
		ownerOf: map[string]string{},
	}
}

func (r *planTreeRegistry) ensureTree(root, goal string) *session.PlanTree {
	t, ok := r.trees[root]
	if !ok {
		t = &session.PlanTree{RootID: root, Goal: goal, Nodes: map[string]*session.PlanTreeNode{}}
		r.trees[root] = t
	}
	return t
}

func ensureNode(t *session.PlanTree, id string) *session.PlanTreeNode {
	n, ok := t.Nodes[id]
	if !ok {
		n = &session.PlanTreeNode{ID: id}
		t.Nodes[id] = n
	}
	return n
}

func (r *planTreeRegistry) dispatched(root string, rec task.Record, store *session.Store, sessionID string) {
	r.mu.Lock()
	t := r.ensureTree(root, "")
	if rec.ID == root {
		t.Goal = rec.Goal
	}
	n := ensureNode(t, rec.ID)
	n.Goal, n.Kind, n.Status = rec.Goal, string(rec.Kind), "dispatched"
	n.Source, n.Origin = rec.Provenance.Source, rec.Provenance.Origin
	if rec.Kind == task.KindTask {
		n.Type = string(rec.Type)
	}
	n.DispatchedAt = time.Now().UnixMilli()
	// A node owns itself for tool-event lookups: covers not just ordinary Task leaves
	// (already set by decomposed(), redundant-but-harmless here) but also a root Step
	// that degrades to executing directly as a Task (scheduler's retry-then-degrade) —
	// the only dispatch event it ever gets before a tool_call/tool_result tagged with
	// its own id could arrive.
	r.ownerOf[rec.ID] = root
	r.mu.Unlock()
	r.persist(root, store, sessionID)
}

func (r *planTreeRegistry) decomposed(root string, parent task.Record, children []task.Record, store *session.Store, sessionID string) {
	r.mu.Lock()
	t := r.ensureTree(root, "")
	p := ensureNode(t, parent.ID)
	p.Status = "decomposed"
	p.Source, p.Origin = parent.Provenance.Source, parent.Provenance.Origin
	kids := make([]string, 0, len(children))
	for _, c := range children {
		n := ensureNode(t, c.ID)
		n.Goal, n.Kind, n.Status, n.Deps = c.Goal, string(c.Kind), "pending", c.Deps
		n.Source, n.Origin = c.Provenance.Source, c.Provenance.Origin
		if c.Kind == task.KindTask {
			n.Type = string(c.Type)
		}
		kids = append(kids, c.ID)
		r.ownerOf[c.ID] = root
	}
	p.Children = kids
	r.mu.Unlock()
	r.persist(root, store, sessionID)
}

func (r *planTreeRegistry) completed(root, id string, status task.Status, value, errText string, store *session.Store, sessionID string) {
	r.mu.Lock()
	t := r.ensureTree(root, "")
	n := ensureNode(t, id)
	n.Status = string(status)
	n.Value = value
	n.Error = errText
	n.CompletedAt = time.Now().UnixMilli()
	r.mu.Unlock()
	r.persist(root, store, sessionID)
}

// node returns a snapshot of one node's durable goal/value/error, for
// PinPlanNode (ADR 0012 amendment, surface-visibility follow-up) — the
// authoritative server-side source of what a Step's resolved Value actually is,
// mirroring how PinToolEvent re-reads its source from o.History() rather than
// trusting whatever text the client last rendered.
func (r *planTreeRegistry) node(root, id string) (goal, value, errText string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.trees[root]
	if !ok {
		return "", "", "", false
	}
	n, ok := t.Nodes[id]
	if !ok {
		return "", "", "", false
	}
	return n.Goal, n.Value, n.Error, true
}

// converged records that parentID's classify response wired an additional
// dependency edge onto existingID instead of creating a fresh child (ADR 0012
// §3). existingID must already have its own node (it was found via
// findExistingNode, i.e. already added to the graph) — ensureNode here is
// defensive, not expected to create a new entry in practice.
func (r *planTreeRegistry) converged(root, parentID, existingID string, store *session.Store, sessionID string) {
	r.mu.Lock()
	t := r.ensureTree(root, "")
	p := ensureNode(t, parentID)
	p.ConvergesTo = append(p.ConvergesTo, existingID)
	ensureNode(t, existingID)
	r.mu.Unlock()
	r.persist(root, store, sessionID)
}

func (r *planTreeRegistry) toolCalled(rec task.Record, d tools.Descriptor, args map[string]string, store *session.Store, sessionID string) {
	r.mu.Lock()
	root, ok := r.ownerOf[rec.ID]
	if !ok {
		r.mu.Unlock()
		return
	}
	t := r.trees[root]
	n := ensureNode(t, rec.ID)
	n.Command = proposalText(d, args)
	r.mu.Unlock()
	r.persist(root, store, sessionID)
}

func (r *planTreeRegistry) toolFinished(rec task.Record, res tools.Result, status executor.Status, store *session.Store, sessionID string) {
	r.mu.Lock()
	root, ok := r.ownerOf[rec.ID]
	if !ok {
		r.mu.Unlock()
		return
	}
	t := r.trees[root]
	n := ensureNode(t, rec.ID)
	n.ResultText = toolResultText(res)
	n.ResultOutcome = string(status)
	n.ResultRef = res.Ref
	r.mu.Unlock()
	r.persist(root, store, sessionID)
}

// persist rewrites the root's tree to disk. Called after every mutation above
// (outside the mutation's own lock) — plans are small, so a full rewrite per event is
// cheap; a failure here (missing store, disk error) is swallowed, matching the same
// degrade-gracefully posture as artifactStore().
func (r *planTreeRegistry) persist(root string, store *session.Store, sessionID string) {
	if store == nil {
		return
	}
	r.mu.Lock()
	t, ok := r.trees[root]
	if ok {
		t.UpdatedAt = time.Now().UnixMilli()
	}
	r.mu.Unlock()
	if !ok {
		return
	}
	plans, err := store.Plans(sessionID)
	if err != nil {
		return
	}
	_ = plans.Write(root, t)
}
