package runtimesteps

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/cucumber/godog"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
	"agentx/internal/runtime/scheduler"
)

// --- stub collaborators (goroutine-safe) ---------------------------------------

type stubOracle struct {
	mu     sync.Mutex
	atomic map[string]bool // id -> atomic verdict; absent = non-atomic
}

func (o *stubOracle) Atomic(_ context.Context, rec task.Record) (bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.atomic[rec.ID], nil
}

type stubExecutor struct {
	mu      sync.Mutex
	outcome map[string]executor.Status // id -> status; absent = Executed
	called  []string
}

func (e *stubExecutor) Execute(_ context.Context, rec task.Record) executor.Outcome {
	e.mu.Lock()
	e.called = append(e.called, rec.ID)
	st, ok := e.outcome[rec.ID]
	e.mu.Unlock()
	if !ok {
		st = executor.Executed
	}
	return executor.Outcome{Status: st}
}

type stubDecomposer struct {
	mu       sync.Mutex
	children map[string][]task.Record // parent id -> child records
	called   []string
}

func (d *stubDecomposer) Decompose(_ context.Context, rec task.Record) (branch.Result, error) {
	d.mu.Lock()
	d.called = append(d.called, rec.ID)
	ch := d.children[rec.ID]
	d.mu.Unlock()
	return branch.Result{Records: ch, Synthesis: "syn"}, nil
}

// --- world ---------------------------------------------------------------------

type schedWorld struct {
	graph      *task.Graph
	oracle     *stubOracle
	decomposer *stubDecomposer
	exec       *stubExecutor
	sched      *scheduler.Scheduler
	slots      int
	maxDepth   int
	completed  bool
}

func registerTaskSchedulerSteps(sc *godog.ScenarioContext) {
	w := &schedWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = schedWorld{
			graph:      task.NewGraph(),
			oracle:     &stubOracle{atomic: map[string]bool{}},
			decomposer: &stubDecomposer{children: map[string][]task.Record{}},
			exec:       &stubExecutor{outcome: map[string]executor.Status{}},
			slots:      4,
			maxDepth:   10,
		}
		return ctx, nil
	})

	sc.Step(`^a plan of atomic leaves "([^"]*)", "([^"]*)", "([^"]*)" with no dependencies$`, w.threeAtomic)
	sc.Step(`^atomic leaves "([^"]*)" and "([^"]*)" with no dependencies$`, w.twoAtomic)
	sc.Step(`^an atomic leaf "([^"]*)" depending on "([^"]*)" and "([^"]*)"$`, w.atomicDep)
	sc.Step(`^an atomic leaf "([^"]*)" with no dependencies$`, w.oneAtomic)
	sc.Step(`^a non-atomic node "([^"]*)"(?: with no dependencies)?$`, w.nonAtomic)
	sc.Step(`^the decomposer expands "([^"]*)" into atomic leaves "([^"]*)" and "([^"]*)"$`, w.expandInto)
	sc.Step(`^the executor reports every leaf executed$`, w.noopSched)
	sc.Step(`^the executor reports "([^"]*)" failed$`, w.execFails)
	sc.Step(`^a slot budget of (\d+)$`, w.slotBudget)
	sc.Step(`^a plan of (\d+) atomic leaves with no dependencies$`, w.nAtomic)
	sc.Step(`^a max task depth of (\d+)$`, w.setMaxDepth)

	sc.Step(`^the scheduler runs$`, w.run)

	sc.Step(`^"([^"]*)", "([^"]*)", "([^"]*)" each reach status "([^"]*)"$`, w.threeStatus)
	sc.Step(`^"([^"]*)" and "([^"]*)" are dispatched before "([^"]*)"$`, w.dispatchedBefore)
	sc.Step(`^"([^"]*)" is dispatched before "([^"]*)" and "([^"]*)"$`, w.oneDispatchedBeforeTwo)
	sc.Step(`^"([^"]*)" and "([^"]*)" each reach status "([^"]*)"$`, w.twoStatus)
	sc.Step(`^"([^"]*)" reaches status "([^"]*)"$`, w.oneStatus)
	sc.Step(`^"([^"]*)" does not reach status "([^"]*)"$`, w.notStatus)
	sc.Step(`^the decomposer is never invoked$`, w.decomposerNever)
	sc.Step(`^the executor is never invoked for "([^"]*)"$`, w.execNeverFor)
	sc.Step(`^at most (\d+) workers were in flight at once$`, w.peakAtMost)
	sc.Step(`^"([^"]*)" and "([^"]*)" were in flight together$`, w.inFlightTogether)
	sc.Step(`^all (\d+) leaves reach status "([^"]*)"$`, w.allNStatus)
	sc.Step(`^the run completes$`, w.runCompletes)
	sc.Step(`^the scheduler terminates$`, w.runCompletes)
	sc.Step(`^"([^"]*)" is not decomposed$`, w.notDecomposed)
	sc.Step(`^no node is left runnable$`, w.noneRunnable)
	sc.Step(`^the dispatch order is exactly "([^"]*)"$`, w.dispatchOrderExactly)
}

// --- record helpers ------------------------------------------------------------

func rec(id string, deps ...string) task.Record {
	if deps == nil {
		deps = []string{}
	}
	return task.Record{ID: id, Goal: id, Type: task.Query, Status: task.Proposed, Deps: deps}
}

func (w *schedWorld) addAtomic(id string, deps ...string) error {
	w.oracle.atomic[id] = true
	return w.graph.Add(rec(id, deps...))
}

// --- given ---------------------------------------------------------------------

func (w *schedWorld) threeAtomic(a, b, c string) error {
	for _, id := range []string{a, b, c} {
		if err := w.addAtomic(id); err != nil {
			return err
		}
	}
	return nil
}

func (w *schedWorld) twoAtomic(a, b string) error {
	for _, id := range []string{a, b} {
		if err := w.addAtomic(id); err != nil {
			return err
		}
	}
	return nil
}

func (w *schedWorld) atomicDep(id, d1, d2 string) error { return w.addAtomic(id, d1, d2) }

func (w *schedWorld) oneAtomic(id string) error { return w.addAtomic(id) }

func (w *schedWorld) nonAtomic(id string) error {
	w.oracle.atomic[id] = false
	return w.graph.Add(rec(id))
}

func (w *schedWorld) expandInto(parent, g1, g2 string) error {
	w.oracle.atomic[g1] = true
	w.oracle.atomic[g2] = true
	w.decomposer.children[parent] = []task.Record{rec(g1), rec(g2)}
	return nil
}

func (w *schedWorld) execFails(id string) error {
	w.exec.outcome[id] = executor.Failed
	return nil
}

func (w *schedWorld) slotBudget(n int) error { w.slots = n; return nil }

func (w *schedWorld) nAtomic(n int) error {
	for i := 1; i <= n; i++ {
		if err := w.addAtomic(fmt.Sprintf("n%d", i)); err != nil {
			return err
		}
	}
	return nil
}


func (w *schedWorld) setMaxDepth(n int) error { w.maxDepth = n; return nil }

// --- when ----------------------------------------------------------------------

func (w *schedWorld) run() error {
	// Run synchronously: the scheduler is provably terminating (each node is dispatched at
	// most once, else ErrStalled), and no stub blocks, so no timer/watchdog is needed.
	w.sched = scheduler.New(w.graph, w.oracle, w.decomposer, w.exec, w.slots, w.maxDepth)
	if err := w.sched.Run(context.Background()); err != nil {
		return fmt.Errorf("scheduler.Run: %w", err)
	}
	w.completed = true
	return nil
}

func (w *schedWorld) noopSched() error { return nil }

// --- then ----------------------------------------------------------------------

func (w *schedWorld) status(id string) string {
	r, ok := w.graph.Node(id)
	if !ok {
		return "<absent>"
	}
	return string(r.Status)
}

func (w *schedWorld) oneStatus(id, want string) error {
	if got := w.status(id); got != want {
		return fmt.Errorf("node %q status = %q, want %q", id, got, want)
	}
	return nil
}

func (w *schedWorld) twoStatus(a, b, want string) error {
	return firstErr(w.oneStatus(a, want), w.oneStatus(b, want))
}

func (w *schedWorld) threeStatus(a, b, c, want string) error {
	return firstErr(w.oneStatus(a, want), w.oneStatus(b, want), w.oneStatus(c, want))
}

func (w *schedWorld) notStatus(id, unwanted string) error {
	if got := w.status(id); got == unwanted {
		return fmt.Errorf("node %q reached status %q, want it not to", id, unwanted)
	}
	return nil
}

func (w *schedWorld) allNStatus(n int, want string) error {
	for i := 1; i <= n; i++ {
		if err := w.oneStatus(fmt.Sprintf("n%d", i), want); err != nil {
			return err
		}
	}
	return nil
}

func (w *schedWorld) dispatchedBefore(a, b, c string) error {
	return firstErr(w.before(a, c), w.before(b, c))
}

func (w *schedWorld) oneDispatchedBeforeTwo(a, b, c string) error {
	return firstErr(w.before(a, b), w.before(a, c))
}

func (w *schedWorld) before(x, y string) error {
	order := w.sched.DispatchOrder()
	ix, iy := slices.Index(order, x), slices.Index(order, y)
	if ix < 0 {
		return fmt.Errorf("%q was never dispatched (order %v)", x, order)
	}
	if iy >= 0 && ix > iy {
		return fmt.Errorf("%q dispatched after %q (order %v)", x, y, order)
	}
	return nil
}

func (w *schedWorld) decomposerNever() error {
	if len(w.decomposer.called) != 0 {
		return fmt.Errorf("decomposer was invoked for %v, want never", w.decomposer.called)
	}
	return nil
}

func (w *schedWorld) execNeverFor(id string) error {
	w.exec.mu.Lock()
	defer w.exec.mu.Unlock()
	if slices.Contains(w.exec.called, id) {
		return fmt.Errorf("executor was invoked for %q, want never", id)
	}
	return nil
}

func (w *schedWorld) peakAtMost(n int) error {
	if got := w.sched.Peak(); got > n {
		return fmt.Errorf("peak in-flight = %d, want at most %d", got, n)
	}
	return nil
}

// inFlightTogether asserts both nodes were dispatched and the scheduler held at least two
// workers in flight at once. With only these two nodes ready initially, a peak of >=2 means
// the atomic leaf's execution and the sibling's decomposition overlapped (interleaving),
// proven structurally without a blocking stub or timer.
func (w *schedWorld) inFlightTogether(a, b string) error {
	order := w.sched.DispatchOrder()
	if !slices.Contains(order, a) || !slices.Contains(order, b) {
		return fmt.Errorf("both %q and %q must be dispatched (order %v)", a, b, order)
	}
	if got := w.sched.Peak(); got < 2 {
		return fmt.Errorf("peak in-flight = %d, want >= 2 (no overlap)", got)
	}
	return nil
}

func (w *schedWorld) runCompletes() error {
	if !w.completed {
		return fmt.Errorf("scheduler run did not complete")
	}
	return nil
}

func (w *schedWorld) notDecomposed(id string) error {
	if slices.Contains(w.decomposer.called, id) {
		return fmt.Errorf("node %q was decomposed, want not", id)
	}
	return nil
}

func (w *schedWorld) noneRunnable() error {
	if r := w.graph.Ready(); len(r) != 0 {
		return fmt.Errorf("nodes still runnable: %v", r)
	}
	return nil
}

func (w *schedWorld) dispatchOrderExactly(want string) error {
	got := strings.Join(w.sched.DispatchOrder(), ", ")
	if got != normalizeCSV(want) {
		return fmt.Errorf("dispatch order = %q, want %q", got, normalizeCSV(want))
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
