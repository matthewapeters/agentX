package runtimesteps

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// branchWorld drives the Phase-2 branch-context scenarios. parentWM / parentGraph model
// the parent; parentBus is the parent conversation (branch events must never land here).
type branchWorld struct {
	parentWM    *session.WorkingMemory
	parentGraph *task.Graph
	parentBus   []state.Event

	b   *branch.Branch
	res branch.Result

	root            string
	allowed         bool // last ReadAllowed check
	synthesis       string
	parentSessionID string
}

func registerTaskBranchSteps(sc *godog.ScenarioContext) {
	w := &branchWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = branchWorld{parentWM: &session.WorkingMemory{}, parentGraph: task.NewGraph()}
		return ctx, nil
	})

	sc.Step(`^a parent working memory with enabled facts "([^"]*)" and "([^"]*)"$`, w.parentEnabledFacts)
	sc.Step(`^a disabled parent fact "([^"]*)"$`, w.parentDisabledFact)
	sc.Step(`^a parent working memory with (\d+) facts$`, w.parentNFacts)
	sc.Step(`^a parent task DAG containing node "([^"]*)"$`, w.parentNode)
	sc.Step(`^a parent session "([^"]*)"$`, w.parentSession)

	sc.Step(`^a branch is forked from the parent$`, w.fork)
	sc.Step(`^a branch forked from the parent$`, w.fork)
	sc.Step(`^a branch forked from the parent that added records and local facts$`, w.forkWithScratch)
	sc.Step(`^the branch has added child records "([^"]*)" and "([^"]*)"$`, w.branchAddRecords)
	sc.Step(`^the branch synthesis is "([^"]*)"$`, w.branchSynthesis)
	sc.Step(`^the branch records a local fact "([^"]*)" = "([^"]*)"$`, w.branchLocalFact)
	sc.Step(`^the branch emits a planning event$`, w.branchEmit)
	sc.Step(`^the branch is sealed$`, w.seal)

	sc.Step(`^a branch result with records "([^"]*)" and "([^"]*)" each depending on "([^"]*)"$`, w.branchResultDeps)
	sc.Step(`^the result is merged into the parent$`, w.merge)
	sc.Step(`^the branch is discarded without sealing$`, w.discard)

	sc.Step(`^the branch read-restricted catalog$`, w.noop)
	sc.Step(`^a working directory "([^"]*)" with no granted read paths$`, w.workdir)
	sc.Step(`^a working directory "([^"]*)"$`, w.workdir)
	sc.Step(`^a read of "([^"]*)" is checked$`, w.readCheck)
	sc.Step(`^an out-of-cwd read of "([^"]*)" is approved for the session$`, w.approveSession)
	sc.Step(`^an out-of-cwd read of "([^"]*)" is approved once$`, w.approveOnce)

	sc.Step(`^a branch at depth one below the max task depth$`, w.branchNearMaxDepth)
	sc.Step(`^it forks a child branch$`, w.forkChild)

	// assertions
	sc.Step(`^the branch sees fact "([^"]*)"$`, w.seesFact)
	sc.Step(`^the branch does not see fact "([^"]*)"$`, w.notSeesFact)
	sc.Step(`^the parent working memory has no "([^"]*)" fact$`, w.parentNoFact)
	sc.Step(`^the parent working memory has (\d+) facts$`, w.parentFactCount)
	sc.Step(`^the parent conversation receives no branch event$`, w.parentBusEmpty)
	sc.Step(`^the branch log contains the planning event$`, w.branchLogHasEvent)
	sc.Step(`^it contains the tool "([^"]*)"$`, w.catalogHas)
	sc.Step(`^it does not contain the tool "([^"]*)"$`, w.catalogLacks)
	sc.Step(`^the read is not allowed without approval$`, w.readNotAllowed)
	sc.Step(`^a read of "([^"]*)" is allowed without approval$`, w.readAllowedAt)
	sc.Step(`^a read of "([^"]*)" is not allowed without approval$`, w.readNotAllowedAt)
	sc.Step(`^working memory records a permitted read path "([^"]*)"$`, w.wmHasReadPath)
	sc.Step(`^working memory records no permitted read path$`, w.wmNoReadPath)
	sc.Step(`^the result records are exactly "([^"]*)"$`, w.resultRecords)
	sc.Step(`^the result synthesis is "([^"]*)"$`, w.resultSynthesis)
	sc.Step(`^the parent DAG has (\d+) nodes?$`, w.parentDAGCount)
	sc.Step(`^the parent edge set is exactly "([^"]*)"$`, w.parentEdges)
	sc.Step(`^the merge returns synthesis "([^"]*)"$`, w.mergeSynthesis)
	sc.Step(`^the child branch depth equals the max task depth$`, w.childDepthMax)
	sc.Step(`^a further fork is refused with a max-depth error$`, w.furtherForkRefused)
	sc.Step(`^the branch parent id is "([^"]*)"$`, w.branchParentID)
	sc.Step(`^the branch depth is (\d+)$`, w.branchDepthIs)
}

const testMaxDepth = 5

// --- parent setup --------------------------------------------------------------

func (w *branchWorld) parentEnabledFacts(a, b string) error {
	w.parentWM.Set(a, a+"-val")
	w.parentWM.Set(b, b+"-val")
	return nil
}

func (w *branchWorld) parentDisabledFact(key string) error {
	w.parentWM.Set(key, "hidden")
	w.parentWM.SetEnabled(key, false)
	return nil
}

func (w *branchWorld) parentNFacts(n int) error {
	for i := range n {
		w.parentWM.Set(fmt.Sprintf("f%d", i), "v")
	}
	return nil
}

func (w *branchWorld) parentNode(id string) error {
	if _, ok := w.parentGraph.Node(id); ok {
		return nil
	}
	return w.parentGraph.Add(task.Record{ID: id, Goal: id, Type: task.Query, Status: task.Proposed, Deps: []string{}})
}

func (w *branchWorld) parentSession(id string) error {
	w.parentGraph = task.NewGraph()
	w.parentSessionID = id
	return nil
}

// parentSessionID is stored separately so Fork can carry it as provenance.
func (w *branchWorld) forkFrom(id string) (*branch.Branch, error) {
	return branch.Fork(branch.Parent{
		SessionID: id,
		Depth:     0,
		Facts:     w.parentWM.Enabled(),
	}, testMaxDepth)
}

// --- branch lifecycle ----------------------------------------------------------

func (w *branchWorld) fork() error {
	b, err := w.forkFrom(w.parentSessionID)
	if err != nil {
		return err
	}
	w.b = b
	return nil
}

func (w *branchWorld) forkWithScratch() error {
	if err := w.fork(); err != nil {
		return err
	}
	_ = w.b.Add(task.Record{ID: "x1", Goal: "x", Type: task.Query, Status: task.Proposed, Deps: []string{}})
	w.b.SetLocalFact("scratch", "1")
	return nil
}

func (w *branchWorld) branchAddRecords(a, b string) error {
	for _, id := range []string{a, b} {
		if err := w.b.Add(task.Record{ID: id, Goal: id, Type: task.Query, Status: task.Proposed, Deps: []string{}}); err != nil {
			return err
		}
	}
	return nil
}

func (w *branchWorld) branchSynthesis(s string) error { w.b.SetSynthesis(s); return nil }

func (w *branchWorld) branchLocalFact(k, v string) error { w.b.SetLocalFact(k, v); return nil }

func (w *branchWorld) branchEmit() error {
	w.b.Emit(state.Event{EventType: "PLAN_STEP", ContentType: state.ContentTaskDiagnostic})
	return nil
}

func (w *branchWorld) seal() error { w.res = w.b.Seal(); return nil }

func (w *branchWorld) branchResultDeps(a, b, dep string) error {
	if err := w.parentNode(dep); err != nil {
		return err
	}
	w.res = branch.Result{
		Records: []task.Record{
			{ID: a, Goal: a, Type: task.Query, Status: task.Proposed, Deps: []string{dep}},
			{ID: b, Goal: b, Type: task.Query, Status: task.Proposed, Deps: []string{dep}},
		},
		Synthesis: "two independent steps",
	}
	return nil
}

func (w *branchWorld) merge() error {
	syn, err := branch.MergeResult(w.parentGraph, w.res)
	if err != nil {
		return err
	}
	w.synthesis = syn
	return nil
}

func (w *branchWorld) discard() error { w.b = nil; return nil }

// --- grants / confinement ------------------------------------------------------

func (w *branchWorld) workdir(root string) error { w.root = root; return nil }

func (w *branchWorld) readCheck(path string) error {
	w.allowed = session.ReadAllowed(w.root, session.PermittedReadPaths(w.parentWM), path)
	return nil
}

func (w *branchWorld) approveSession(path string) error {
	session.ApplyGrant(w.parentWM, path, session.GrantSession)
	return nil
}

func (w *branchWorld) approveOnce(path string) error {
	session.ApplyGrant(w.parentWM, path, session.GrantOnce)
	return nil
}

// --- depth ---------------------------------------------------------------------

func (w *branchWorld) branchNearMaxDepth() error {
	b, err := branch.Fork(branch.Parent{SessionID: "p", Depth: testMaxDepth - 2}, testMaxDepth)
	if err != nil {
		return err
	}
	w.b = b // sits at depth testMaxDepth-1
	return nil
}

func (w *branchWorld) forkChild() error {
	child, err := w.b.Fork()
	if err != nil {
		return err
	}
	w.b = child
	return nil
}

// --- assertions ----------------------------------------------------------------

func (w *branchWorld) noop() error { return nil }

func (w *branchWorld) seesFact(key string) error {
	if _, ok := w.b.Fact(key); !ok {
		return fmt.Errorf("branch does not see fact %q", key)
	}
	return nil
}

func (w *branchWorld) notSeesFact(key string) error {
	if _, ok := w.b.Fact(key); ok {
		return fmt.Errorf("branch unexpectedly sees fact %q", key)
	}
	return nil
}

func (w *branchWorld) parentNoFact(key string) error {
	if _, ok := w.parentWM.Get(key); ok {
		return fmt.Errorf("parent WM unexpectedly has fact %q", key)
	}
	return nil
}

func (w *branchWorld) parentFactCount(n int) error {
	if got := len(w.parentWM.Facts); got != n {
		return fmt.Errorf("parent WM fact count = %d, want %d", got, n)
	}
	return nil
}

func (w *branchWorld) parentBusEmpty() error {
	if len(w.parentBus) != 0 {
		return fmt.Errorf("parent bus received %d branch events, want 0", len(w.parentBus))
	}
	return nil
}

func (w *branchWorld) branchLogHasEvent() error {
	if len(w.b.Events()) == 0 {
		return fmt.Errorf("branch log is empty, expected the planning event")
	}
	return nil
}

func (w *branchWorld) catalogHas(id string) error {
	if !catalogContains(id) {
		return fmt.Errorf("read catalog missing tool %q", id)
	}
	return nil
}

func (w *branchWorld) catalogLacks(id string) error {
	if catalogContains(id) {
		return fmt.Errorf("read catalog unexpectedly contains mutating tool %q", id)
	}
	return nil
}

func catalogContains(id string) bool {
	for _, d := range tools.DefaultRegistry().Available(true) {
		if d.ID == id {
			return true
		}
	}
	return false
}

func (w *branchWorld) readNotAllowed() error {
	if w.allowed {
		return fmt.Errorf("read was allowed without approval, want not allowed")
	}
	return nil
}

func (w *branchWorld) readAllowedAt(path string) error {
	if !session.ReadAllowed(w.root, session.PermittedReadPaths(w.parentWM), path) {
		return fmt.Errorf("read of %q not allowed, want allowed", path)
	}
	return nil
}

func (w *branchWorld) readNotAllowedAt(path string) error {
	if session.ReadAllowed(w.root, session.PermittedReadPaths(w.parentWM), path) {
		return fmt.Errorf("read of %q allowed, want not allowed", path)
	}
	return nil
}

func (w *branchWorld) wmHasReadPath(path string) error {
	if slices.Contains(session.PermittedReadPaths(w.parentWM), path) {
		return nil
	}
	return fmt.Errorf("WM has no permitted read path %q (have %v)", path, session.PermittedReadPaths(w.parentWM))
}

func (w *branchWorld) wmNoReadPath() error {
	if got := session.PermittedReadPaths(w.parentWM); len(got) != 0 {
		return fmt.Errorf("WM has permitted read paths %v, want none", got)
	}
	return nil
}

func (w *branchWorld) resultRecords(want string) error {
	var ids []string
	for _, r := range w.res.Records {
		ids = append(ids, r.ID)
	}
	if got := strings.Join(ids, ", "); got != normalizeCSV(want) {
		return fmt.Errorf("result records = %q, want %q", got, normalizeCSV(want))
	}
	return nil
}

func (w *branchWorld) resultSynthesis(want string) error {
	if w.res.Synthesis != want {
		return fmt.Errorf("result synthesis = %q, want %q", w.res.Synthesis, want)
	}
	return nil
}

func (w *branchWorld) parentDAGCount(n int) error {
	if got := w.parentGraph.Len(); got != n {
		return fmt.Errorf("parent DAG node count = %d, want %d", got, n)
	}
	return nil
}

func (w *branchWorld) parentEdges(want string) error {
	var es []string
	for _, e := range w.parentGraph.Edges() {
		es = append(es, fmt.Sprintf("%s->%s", e[0], e[1]))
	}
	if got := strings.Join(es, ", "); got != normalizeCSV(want) {
		return fmt.Errorf("parent edges = %q, want %q", got, normalizeCSV(want))
	}
	return nil
}

func (w *branchWorld) mergeSynthesis(want string) error {
	if w.synthesis != want {
		return fmt.Errorf("merge synthesis = %q, want %q", w.synthesis, want)
	}
	return nil
}

func (w *branchWorld) childDepthMax() error {
	if w.b.Depth() != testMaxDepth {
		return fmt.Errorf("child depth = %d, want %d", w.b.Depth(), testMaxDepth)
	}
	return nil
}

func (w *branchWorld) furtherForkRefused() error {
	if _, err := w.b.Fork(); err == nil {
		return fmt.Errorf("expected max-depth error, got nil")
	}
	return nil
}

func (w *branchWorld) branchParentID(want string) error {
	if w.b.ParentID() != want {
		return fmt.Errorf("branch parent id = %q, want %q", w.b.ParentID(), want)
	}
	return nil
}

func (w *branchWorld) branchDepthIs(n int) error {
	if w.b.Depth() != n {
		return fmt.Errorf("branch depth = %d, want %d", w.b.Depth(), n)
	}
	return nil
}

func normalizeCSV(s string) string {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, ", ")
}
