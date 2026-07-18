package wavefront

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/scheduler"
	"agentx/internal/tools"
)

// recObserver records lifecycle callbacks in call order (main-loop only, so no
// locking needed) and implements both scheduler.Observer and the optional
// scheduler.ConvergenceObserver — the ADR 0012 amendment's surface-visibility fix
// depends on wavefront actually calling both.
type recObserver struct {
	dispatched []string
	decomposed []string // "parent→child1,child2"
	completed  []string // "id=status:value/error"
	converged  []string // "parent→existing"
}

func (r *recObserver) NodeDispatched(rec task.Record, depth int) {
	r.dispatched = append(r.dispatched, rec.ID)
}

func (r *recObserver) NodeDecomposed(parent task.Record, children []task.Record) {
	ids := make([]string, len(children))
	for i, c := range children {
		ids[i] = c.ID
	}
	r.decomposed = append(r.decomposed, fmt.Sprintf("%s→%s", parent.ID, strings.Join(ids, ",")))
}

func (r *recObserver) NodeCompleted(id string, status task.Status, value, errText string) {
	r.completed = append(r.completed, fmt.Sprintf("%s=%s:%s%s", id, status, value, errText))
}

func (r *recObserver) NodeConverged(parentID string, existing task.Record) {
	r.converged = append(r.converged, fmt.Sprintf("%s→%s", parentID, existing.ID))
}

type stubClassifierFn func(ctx context.Context, wm, question string) (Result, error)

func (f stubClassifierFn) Classify(ctx context.Context, wm, question string) (Result, error) {
	return f(ctx, wm, question)
}

type stubExecutor struct{ out executor.Outcome }

func (e stubExecutor) Execute(context.Context, task.Record) executor.Outcome { return e.out }

func runAndWait(t *testing.T, s *Scheduler) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not terminate")
		return nil
	}
}

func newGraph(t *testing.T, rootGoal string) *task.Graph {
	t.Helper()
	g := task.NewGraph()
	if err := g.Add(task.Record{ID: "root", Goal: rootGoal, Kind: task.KindStep, Status: task.Proposed, Deps: []string{}}); err != nil {
		t.Fatalf("Add root: %v", err)
	}
	return g
}

func failSynthesis(t *testing.T) Chat {
	return func(context.Context, string, string, json.RawMessage) (string, error) {
		t.Fatal("synthesis should not be called in this test")
		return "", nil
	}
}

// TestSelfMatchResolvesDirectly: a Know whose name echoes the dispatching node's
// own goal (normalized) resolves it directly via registerOrConvergeKnow's
// findExistingNode scan — no separate self-match code path, no children spawned.
func TestSelfMatchResolvesDirectly(t *testing.T) {
	g := newGraph(t, "what language is this project?")
	classify := stubClassifierFn(func(_ context.Context, _, question string) (Result, error) {
		return Result{Knows: []Know{{Name: question, Value: "Go"}}}, nil
	})
	s := New(g, "what language is this project?", classify, failSynthesis(t), "", stubExecutor{}, 4, 3)

	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	root, _ := g.Node("root")
	if root.Status != task.Done || root.Value != "Go" {
		t.Fatalf("root = %+v, want Done/Go", root)
	}
	if len(root.Deps) != 0 {
		t.Errorf("root.Deps = %v, want empty (self-resolved, no children)", root.Deps)
	}
}

// TestCommandNeedExecutesAndUnblocksParent: a command-valued Need executes via the
// injected Executor; its Value feeds the parent's eventual synthesis call once the
// parent has no self-match of its own.
func TestCommandNeedExecutesAndUnblocksParent(t *testing.T) {
	g := newGraph(t, "what does this project do?")
	var classifyCalls int
	classify := stubClassifierFn(func(_ context.Context, _, question string) (Result, error) {
		classifyCalls++
		if question != "what does this project do?" {
			t.Fatalf("unexpected classify question: %q", question)
		}
		return Result{Needs: []Need{
			{Name: "contents of README.md", Command: &Command{Tool: "read_file", Args: map[string]string{"path": "README.md"}}},
		}}, nil
	})
	exec := stubExecutor{out: executor.Outcome{Status: executor.Executed, Result: tools.Result{Preview: "a demo project"}}}
	synth := func(_ context.Context, _, usr string, _ json.RawMessage) (string, error) {
		if !strings.Contains(usr, "a demo project") {
			t.Errorf("synthesis user prompt missing the resolved finding: %q", usr)
		}
		return "This project is a demo.", nil
	}
	s := New(g, "what does this project do?", classify, synth, "", exec, 4, 3)
	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if classifyCalls != 1 {
		t.Errorf("classify called %d times, want exactly 1", classifyCalls)
	}
	root, _ := g.Node("root")
	if root.Status != task.Done || root.Value != "This project is a demo." {
		t.Fatalf("root = %+v, want Done via synthesis", root)
	}
	child, ok := g.Node("root-1")
	if !ok {
		t.Fatal("command-Need child node not found")
	}
	if child.Kind != task.KindTask || child.Status != task.Done || child.Value != "a demo project" {
		t.Errorf("child = %+v, want KindTask/Done/'a demo project'", child)
	}
}

// TestOpenNeedSpawnsChildClassifiedInTurn: an open-value Need with no existing
// match becomes a new KindStep child, classified recursively under the same rules.
func TestOpenNeedSpawnsChildClassifiedInTurn(t *testing.T) {
	g := newGraph(t, "review the project")
	classify := stubClassifierFn(func(_ context.Context, _, question string) (Result, error) {
		switch question {
		case "review the project":
			return Result{Needs: []Need{{Name: "what language is used"}}}, nil
		case "what language is used":
			return Result{Knows: []Know{{Name: question, Value: "Go"}}}, nil
		}
		t.Fatalf("unexpected classify question: %q", question)
		return Result{}, nil
	})
	synth := func(_ context.Context, _, usr string, _ json.RawMessage) (string, error) {
		if !strings.Contains(usr, "Go") {
			t.Errorf("synthesis prompt missing resolved child fact: %q", usr)
		}
		return "This project uses Go.", nil
	}
	s := New(g, "review the project", classify, synth, "", stubExecutor{}, 4, 3)
	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	root, _ := g.Node("root")
	if root.Status != task.Done || root.Value != "This project uses Go." {
		t.Fatalf("root = %+v", root)
	}
	child, ok := g.Node("root-1")
	if !ok || child.Kind != task.KindStep || child.Status != task.Done || child.Value != "Go" {
		t.Fatalf("child = %+v, want KindStep/Done/Go", child)
	}
}

// TestDuplicateNeedsInSameResponseConvergeOnOneChild: two Needs naming the
// identical (normalized) question in the SAME classify response wire to one
// child, not two.
func TestDuplicateNeedsInSameResponseConvergeOnOneChild(t *testing.T) {
	g := newGraph(t, "review the project")
	var childClassifyCalls int
	classify := stubClassifierFn(func(_ context.Context, _, question string) (Result, error) {
		switch question {
		case "review the project":
			return Result{Needs: []Need{{Name: "project language"}, {Name: "project language"}}}, nil
		case "project language":
			childClassifyCalls++
			return Result{Knows: []Know{{Name: question, Value: "Go"}}}, nil
		}
		t.Fatalf("unexpected question: %q", question)
		return Result{}, nil
	})
	synth := func(context.Context, string, string, json.RawMessage) (string, error) { return "done", nil }
	s := New(g, "review the project", classify, synth, "", stubExecutor{}, 4, 3)
	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if childClassifyCalls != 1 {
		t.Errorf("child classified %d times, want exactly 1 (converged)", childClassifyCalls)
	}
	root, _ := g.Node("root")
	if len(root.Deps) != 1 {
		t.Errorf("root.Deps = %v, want exactly 1 (converged, not duplicated)", root.Deps)
	}
}

// TestCrossBranchConvergence: two independent branches, dispatched and resolved
// separately, both proposing the identical open-value Need converge onto one
// child — deterministic regardless of goroutine timing, since the merge step that
// decides convergence is always single-threaded.
func TestCrossBranchConvergence(t *testing.T) {
	g := newGraph(t, "review the project")
	var langClassifyCalls int
	classify := stubClassifierFn(func(_ context.Context, _, question string) (Result, error) {
		switch question {
		case "review the project":
			return Result{Needs: []Need{{Name: "branch A"}, {Name: "branch B"}}}, nil
		case "branch A", "branch B":
			return Result{Needs: []Need{{Name: "project language"}}}, nil
		case "project language":
			langClassifyCalls++
			return Result{Knows: []Know{{Name: question, Value: "Go"}}}, nil
		}
		t.Fatalf("unexpected question: %q", question)
		return Result{}, nil
	})
	synth := func(context.Context, string, string, json.RawMessage) (string, error) { return "done", nil }
	s := New(g, "review the project", classify, synth, "", stubExecutor{}, 4, 3)
	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if langClassifyCalls != 1 {
		t.Errorf("'project language' classified %d times, want exactly 1 (converged across branches)", langClassifyCalls)
	}
}

// TestObserverSeesDecomposeAndConverge is the ADR 0012 amendment's regression: before
// it, wavefront never called NodeDecomposed at all, so no node past the root ever
// nested in the output/context plan widget — this locks in that every freshly
// created child is reported via NodeDecomposed (the same callback the continuous
// engine uses, so the existing widget needs no new event type), and a Need that
// converges onto an already-existing node is reported separately via
// NodeConverged, never folded into NodeDecomposed's children list.
func TestObserverSeesDecomposeAndConverge(t *testing.T) {
	g := newGraph(t, "review the project")
	classify := stubClassifierFn(func(_ context.Context, _, question string) (Result, error) {
		switch question {
		case "review the project":
			return Result{Needs: []Need{{Name: "branch A"}, {Name: "branch B"}}}, nil
		case "branch A":
			return Result{Needs: []Need{{Name: "project language"}}}, nil
		case "branch B":
			return Result{Needs: []Need{{Name: "project language"}}}, nil
		case "project language":
			return Result{Knows: []Know{{Name: question, Value: "Go"}}}, nil
		}
		t.Fatalf("unexpected question: %q", question)
		return Result{}, nil
	})
	synth := func(context.Context, string, string, json.RawMessage) (string, error) { return "done", nil }
	obs := &recObserver{}
	s := New(g, "review the project", classify, synth, "", stubExecutor{}, 4, 3, WithObserver(obs))
	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !slices.Contains(obs.decomposed, "root→root-1,root-2") {
		t.Errorf("root's fresh children never reported via NodeDecomposed: %v", obs.decomposed)
	}
	// Exactly one of branch A/branch B created "project language" fresh; the other
	// converged onto it — never both, and never as a second NodeDecomposed entry.
	freshLang := containsSuffix(obs.decomposed, "-1")
	if !freshLang {
		t.Errorf("neither branch reported a fresh 'project language' child via NodeDecomposed: %v", obs.decomposed)
	}
	if len(obs.converged) != 1 {
		t.Fatalf("converged = %v, want exactly one NodeConverged call", obs.converged)
	}
}

func containsSuffix(ss []string, suffix string) bool {
	for _, s := range ss {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

var _ scheduler.ConvergenceObserver = (*recObserver)(nil)

// TestCycleGuardSkipsSelfReferentialNeed: a Need naming its own parent's goal
// verbatim (totAlX's documented self-echo failure) is skipped, not wired — caught
// here by the explicit childID==parentID short-circuit.
func TestCycleGuardSkipsSelfReferentialNeed(t *testing.T) {
	g := newGraph(t, "review the project")
	classify := stubClassifierFn(func(_ context.Context, _, question string) (Result, error) {
		if question == "review the project" {
			return Result{Needs: []Need{{Name: "review the project"}}}, nil
		}
		t.Fatalf("unexpected question: %q", question)
		return Result{}, nil
	})
	synth := func(context.Context, string, string, json.RawMessage) (string, error) {
		return "synthesized answer", nil
	}
	s := New(g, "review the project", classify, synth, "", stubExecutor{}, 4, 3)
	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	root, _ := g.Node("root")
	if len(root.Deps) != 0 {
		t.Errorf("root.Deps = %v, want empty (self-referential Need skipped)", root.Deps)
	}
	if root.Status != task.Done {
		t.Errorf("root.Status = %s, want Done (via synthesis fallback, not stalled)", root.Status)
	}
}

// TestCycleGuardSkipsNeedReferencingAncestor: a deeper case than immediate
// self-reference — a child's own Need names its ANCESTOR's goal, not its own.
// Caught by task.Graph's existing, already-tested cycle detection (graph.Update
// returning ErrCycle), not the explicit self-reference short-circuit.
func TestCycleGuardSkipsNeedReferencingAncestor(t *testing.T) {
	g := newGraph(t, "review the project")
	classify := stubClassifierFn(func(_ context.Context, _, question string) (Result, error) {
		switch question {
		case "review the project":
			return Result{Needs: []Need{{Name: "branch A"}}}, nil
		case "branch A":
			return Result{Needs: []Need{{Name: "review the project"}}}, nil
		}
		t.Fatalf("unexpected question: %q", question)
		return Result{}, nil
	})
	synth := func(context.Context, string, string, json.RawMessage) (string, error) { return "synthesized", nil }
	s := New(g, "review the project", classify, synth, "", stubExecutor{}, 4, 3)
	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	branchA, ok := g.Node("root-1")
	if !ok {
		t.Fatal("branch A node not found")
	}
	if len(branchA.Deps) != 0 {
		t.Errorf("branch A Deps = %v, want empty (ancestor-cycle Need rejected)", branchA.Deps)
	}
	if branchA.Status != task.Done {
		t.Errorf("branch A Status = %s, want Done (resolved via synthesis, not stalled)", branchA.Status)
	}
}

// TestSynthesisErrorFailsNode: a node with no self-match and no Needs at all goes
// straight to synthesis; a failing chat call resolves it Failed with the error
// text as Error, never a silent stall.
func TestSynthesisErrorFailsNode(t *testing.T) {
	g := newGraph(t, "unanswerable question")
	classify := stubClassifierFn(func(context.Context, string, string) (Result, error) {
		return Result{}, nil
	})
	synth := func(context.Context, string, string, json.RawMessage) (string, error) {
		return "", errors.New("model unavailable")
	}
	s := New(g, "unanswerable question", classify, synth, "", stubExecutor{}, 4, 3)
	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	root, _ := g.Node("root")
	if root.Status != task.Failed {
		t.Fatalf("root.Status = %s, want Failed", root.Status)
	}
	if !strings.Contains(root.Error, "model unavailable") {
		t.Errorf("root.Error = %q, want it to contain the synthesis error", root.Error)
	}
}

// TestDepthCapAbstains: a KindStep at max depth fails to Ask rather than
// classifying further, exactly like the continuous engine's bounded recursion.
func TestDepthCapAbstains(t *testing.T) {
	g := newGraph(t, "infinitely deep question")
	classify := stubClassifierFn(func(_ context.Context, _, question string) (Result, error) {
		return Result{Needs: []Need{{Name: question + "+"}}}, nil
	})
	s := New(g, "infinitely deep question", classify, failSynthesis(t), "", stubExecutor{}, 4, 2)
	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	deepest, ok := g.Node("root-1-1")
	if !ok {
		t.Fatal("expected the depth-2 node to exist")
	}
	if deepest.Status != task.Abstained {
		t.Fatalf("deepest node Status = %s, want Abstained (bounded recursion)", deepest.Status)
	}
}

// TestConcurrentClassifyDispatchIsRaceFree exercises several concurrent classify
// dispatches (more branches than slots, so some queue) — run with -race. Proves
// §3's "WM rendered on the main loop only" holds under real goroutine scheduling.
func TestConcurrentClassifyDispatchIsRaceFree(t *testing.T) {
	g := newGraph(t, "root question")
	classify := stubClassifierFn(func(_ context.Context, _, question string) (Result, error) {
		if question == "root question" {
			needs := make([]Need, 5)
			for i := range needs {
				needs[i] = Need{Name: fmt.Sprintf("branch %d", i)}
			}
			return Result{Needs: needs}, nil
		}
		return Result{Knows: []Know{{Name: question, Value: "ok"}}}, nil
	})
	synth := func(context.Context, string, string, json.RawMessage) (string, error) { return "done", nil }
	s := New(g, "root question", classify, synth, "", stubExecutor{}, 4, 3)
	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	root, _ := g.Node("root")
	if root.Status != task.Done {
		t.Fatalf("root.Status = %s, want Done", root.Status)
	}
}

// TestCtxCancelStops proves a caller can bound Run via context without a timer
// inside, mirroring scheduler.TestCtxCancelStops.
func TestCtxCancelStops(t *testing.T) {
	g := newGraph(t, "q")
	classify := stubClassifierFn(func(context.Context, string, string) (Result, error) {
		return Result{Knows: []Know{{Name: "q", Value: "v"}}}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := New(g, "q", classify, failSynthesis(t), "", stubExecutor{}, 4, 3)
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected err %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run ignored ctx cancellation")
	}
}
