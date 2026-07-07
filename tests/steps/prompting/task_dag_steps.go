package promptingsteps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/prompting/task"
)

// dagWorld drives the Phase-1 task-DAG substrate scenarios. events is the ordered
// append-only stream; g is the live projection; admitErr captures the last
// Add/Update result so integrity scenarios can assert on it.
type dagWorld struct {
	events   []task.Record
	g        *task.Graph
	admitErr error
	ready    []string
}

func registerTaskDAGSteps(sc *godog.ScenarioContext) {
	w := &dagWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = dagWorld{g: task.NewGraph()}
		return ctx, nil
	})

	sc.Step(`^a proposed task "([^"]*)" with no dependencies$`, w.proposedNoDeps)
	sc.Step(`^a proposed task "([^"]*)" depending on "([^"]*)"$`, w.proposedDep1)
	sc.Step(`^a proposed task "([^"]*)" depending on "([^"]*)" and "([^"]*)"$`, w.proposedDep2)
	sc.Step(`^node "([^"]*)" transitions to "([^"]*)"$`, w.transition)
	sc.Step(`^node "([^"]*)" is updated to status "([^"]*)"$`, w.transition)
	sc.Step(`^"([^"]*)" is updated to depend on "([^"]*)"$`, w.updateDep)
	sc.Step(`^"([^"]*)" is admitted to the DAG$`, w.admit)
	sc.Step(`^a second task claims id "([^"]*)"$`, w.secondClaim)

	sc.Step(`^the task events are persisted and reloaded$`, w.persistReload)
	sc.Step(`^the DAG is rebuilt from the event log twice$`, w.rebuildTwice)
	sc.Step(`^the roots are queried$`, w.noop)
	sc.Step(`^the ready set is queried$`, w.queryReady)

	sc.Step(`^the reconstructed DAG has (\d+) nodes?$`, w.nodeCount)
	sc.Step(`^the DAG has (\d+) nodes?$`, w.nodeCount)
	sc.Step(`^node "([^"]*)" has empty deps$`, w.nodeEmptyDeps)
	sc.Step(`^the reload is byte-identical to the original$`, w.byteIdentical)
	sc.Step(`^the edge set is exactly "([^"]*)"$`, w.edgeSet)
	sc.Step(`^the roots are exactly "([^"]*)"$`, w.rootsExactly)
	sc.Step(`^both reconstructions are identical$`, w.reconstructionsIdentical)
	sc.Step(`^"([^"]*)" is not in the ready set$`, w.notReady)
	sc.Step(`^"([^"]*)" is in the ready set$`, w.isReady)
	sc.Step(`^admission fails with a dangling-dependency error$`, w.failDangling)
	sc.Step(`^admission fails with a cycle error$`, w.failCycle)
	sc.Step(`^admission fails with a duplicate-id error$`, w.failDup)
	sc.Step(`^the DAG remains acyclic$`, w.acyclic)
	sc.Step(`^node "([^"]*)" has status "([^"]*)"$`, w.nodeStatus)
	sc.Step(`^the prior "([^"]*)" event is retained in the log$`, w.priorRetained)
}

// --- record construction / admission -------------------------------------------

func (w *dagWorld) record(id string, deps ...string) task.Record {
	if deps == nil {
		deps = []string{}
	}
	return task.Record{ID: id, Goal: id, Type: task.Query, Status: task.Proposed, Deps: deps}
}

// stage adds a new node, recording both the event and the admission result.
func (w *dagWorld) stage(rec task.Record) error {
	w.admitErr = w.g.Add(rec)
	if w.admitErr == nil {
		w.events = append(w.events, rec)
	}
	return nil
}

func (w *dagWorld) proposedNoDeps(id string) error { return w.stage(w.record(id)) }
func (w *dagWorld) proposedDep1(id, d string) error { return w.stage(w.record(id, d)) }
func (w *dagWorld) proposedDep2(id, d1, d2 string) error {
	return w.stage(w.record(id, d1, d2))
}

// admit is the "When ... is admitted" trigger for records staged as pending; here
// staging already ran Add, so the error is captured — this step just asserts nothing
// further and lets the Then check w.admitErr.
func (w *dagWorld) admit(string) error { return nil }

func (w *dagWorld) secondClaim(id string) error {
	w.admitErr = w.g.Add(w.record(id))
	return nil
}

func (w *dagWorld) transition(id, status string) error {
	rec, ok := w.g.Node(id)
	if !ok {
		return fmt.Errorf("node %q not found", id)
	}
	rec.Status = task.Status(status)
	if err := w.g.Update(rec); err != nil {
		return err
	}
	w.events = append(w.events, rec)
	return nil
}

func (w *dagWorld) updateDep(id, dep string) error {
	rec, ok := w.g.Node(id)
	if !ok {
		return fmt.Errorf("node %q not found", id)
	}
	rec.Deps = []string{dep}
	w.admitErr = w.g.Update(rec)
	if w.admitErr == nil {
		w.events = append(w.events, rec)
	}
	return nil
}

// --- reload / rebuild ----------------------------------------------------------

// foldEvents replays an event stream into a fresh graph via Add (first-seen id) /
// Update (subsequent), mirroring task_proposed / task_updated.
func foldEvents(events []task.Record) (*task.Graph, error) {
	g := task.NewGraph()
	for _, ev := range events {
		if _, ok := g.Node(ev.ID); ok {
			if err := g.Update(ev); err != nil {
				return nil, err
			}
			continue
		}
		if err := g.Add(ev); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (w *dagWorld) persistReload() error {
	blob, err := json.Marshal(w.events)
	if err != nil {
		return err
	}
	var reloaded []task.Record
	if err := json.Unmarshal(blob, &reloaded); err != nil {
		return err
	}
	g, err := foldEvents(reloaded)
	if err != nil {
		return err
	}
	w.g = g
	return nil
}

func (w *dagWorld) rebuildTwice() error {
	g1, err := foldEvents(w.events)
	if err != nil {
		return err
	}
	g2, err := foldEvents(w.events)
	if err != nil {
		return err
	}
	a, _ := json.Marshal(struct {
		N []task.Record `json:"n"`
		E [][2]string   `json:"e"`
	}{g1.Nodes(), g1.Edges()})
	b, _ := json.Marshal(struct {
		N []task.Record `json:"n"`
		E [][2]string   `json:"e"`
	}{g2.Nodes(), g2.Edges()})
	if string(a) != string(b) {
		return fmt.Errorf("reconstructions differ:\n%s\n%s", a, b)
	}
	return nil
}

func (w *dagWorld) queryReady() error {
	w.ready = w.g.Ready()
	return nil
}

func (w *dagWorld) noop() error { return nil }

// --- assertions ----------------------------------------------------------------

func (w *dagWorld) nodeCount(n int) error {
	if got := w.g.Len(); got != n {
		return fmt.Errorf("node count = %d, want %d", got, n)
	}
	return nil
}

func (w *dagWorld) nodeEmptyDeps(id string) error {
	rec, ok := w.g.Node(id)
	if !ok {
		return fmt.Errorf("node %q not found", id)
	}
	if len(rec.Deps) != 0 {
		return fmt.Errorf("node %q deps = %v, want empty", id, rec.Deps)
	}
	return nil
}

func (w *dagWorld) byteIdentical() error {
	before, err := json.Marshal(w.events)
	if err != nil {
		return err
	}
	after, err := json.Marshal(w.g.Nodes())
	if err != nil {
		return err
	}
	if string(before) != string(after) {
		return fmt.Errorf("reload not byte-identical:\nbefore %s\nafter  %s", before, after)
	}
	return nil
}

func (w *dagWorld) edgeSet(want string) error {
	var got []string
	for _, e := range w.g.Edges() {
		got = append(got, fmt.Sprintf("%s->%s", e[0], e[1]))
	}
	if g, wnt := strings.Join(got, ", "), normalizeList(want); g != wnt {
		return fmt.Errorf("edge set = %q, want %q", g, wnt)
	}
	return nil
}

func (w *dagWorld) rootsExactly(want string) error {
	if g, wnt := strings.Join(w.g.Roots(), ", "), normalizeList(want); g != wnt {
		return fmt.Errorf("roots = %q, want %q", g, wnt)
	}
	return nil
}

func (w *dagWorld) reconstructionsIdentical() error { return w.rebuildTwice() }

func (w *dagWorld) notReady(id string) error {
	if containsStr(w.ready, id) {
		return fmt.Errorf("%q unexpectedly ready: %v", id, w.ready)
	}
	return nil
}

func (w *dagWorld) isReady(id string) error {
	if !containsStr(w.ready, id) {
		return fmt.Errorf("%q not ready, ready set = %v", id, w.ready)
	}
	return nil
}

func (w *dagWorld) failDangling() error { return wantErr(w.admitErr, task.ErrDanglingDep) }
func (w *dagWorld) failCycle() error    { return wantErr(w.admitErr, task.ErrCycle) }
func (w *dagWorld) failDup() error      { return wantErr(w.admitErr, task.ErrDuplicateID) }

func (w *dagWorld) acyclic() error {
	// Every edge must reference an existing node and the graph must still fold clean.
	if _, err := foldEvents(w.events); err != nil {
		return fmt.Errorf("graph no longer folds cleanly: %w", err)
	}
	return nil
}

func (w *dagWorld) nodeStatus(id, want string) error {
	rec, ok := w.g.Node(id)
	if !ok {
		return fmt.Errorf("node %q not found", id)
	}
	if got := string(rec.Status); got != want {
		return fmt.Errorf("node %q status = %q, want %q", id, got, want)
	}
	return nil
}

func (w *dagWorld) priorRetained(status string) error {
	for _, ev := range w.events {
		if string(ev.Status) == status {
			return nil
		}
	}
	return fmt.Errorf("no %q event retained in log of %d events", status, len(w.events))
}

// --- helpers -------------------------------------------------------------------

func normalizeList(s string) string {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, ", ")
}

func containsStr(ss []string, v string) bool {
	return slices.Contains(ss, v)
}

func wantErr(got, want error) error {
	if got == nil {
		return fmt.Errorf("expected error %v, got nil", want)
	}
	if !errors.Is(got, want) {
		return fmt.Errorf("error = %v, want kind %v", got, want)
	}
	return nil
}
