package promptingsteps

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/prompting/planner"
	"agentx/internal/prompting/task"
)

type plannerWorld struct {
	parentID string
	respJSON string
	plan     planner.Plan
	err      error
}

func registerPlannerSteps(sc *godog.ScenarioContext) {
	w := &plannerWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = plannerWorld{}
		return ctx, nil
	})

	sc.Step(`^a parent task id "([^"]*)"$`, w.parentTaskID)
	sc.Step(`^a planner response with steps "([^"]*)" and "([^"]*)" where "([^"]*)" depends on "([^"]*)"$`, w.responseTwoSteps)
	sc.Step(`^a planner response with no steps$`, w.responseNoSteps)
	sc.Step(`^a planner response whose step depends on an undefined step$`, w.responseDanglingDep)
	sc.Step(`^a planner response with (\d+) dag nodes$`, w.responseNNodes)
	sc.Step(`^a planner response with a node missing both task and step$`, w.responseNeitherTaskNorStep)
	sc.Step(`^a planner response with a node carrying both task and step$`, w.responseBothTaskAndStep)
	sc.Step(`^the plan is parsed$`, w.parse)
	sc.Step(`^it yields (\d+) child records$`, w.childCount)
	sc.Step(`^a child goal for "([^"]*)" exists$`, w.childGoalExists)
	sc.Step(`^the child for "([^"]*)" depends on the child for "([^"]*)"$`, w.childDependsOn)
	sc.Step(`^every child id is namespaced under "([^"]*)"$`, w.childIDsNamespaced)
	sc.Step(`^parsing fails$`, w.parseFails)
}

func (w *plannerWorld) parentTaskID(id string) error { w.parentID = id; return nil }

// taskNode renders one valid "task" dag node.
func taskNode(id string, deps []string) string {
	depsJSON := "[]"
	if len(deps) > 0 {
		quoted := make([]string, len(deps))
		for i, d := range deps {
			quoted[i] = fmt.Sprintf("%q", d)
		}
		depsJSON = "[" + strings.Join(quoted, ",") + "]"
	}
	return fmt.Sprintf(`{"id":%q,"deps":%s,"task":{"tool":"list_dir","args":{"path":"."},"explanation":"do %s"}}`, id, depsJSON, id)
}

func (w *plannerWorld) responseTwoSteps(g1, g2, dep, on string) error {
	w.respJSON = fmt.Sprintf(`{"plan":{"name":"p","objective":"s","dag":[
		%s,
		%s]}}`, taskNode(g1, nil), taskNode(g2, []string{on}))
	_ = dep
	return nil
}

func (w *plannerWorld) responseNoSteps() error {
	w.respJSON = `{"plan":{"name":"p","objective":"s","dag":[]}}`
	return nil
}

func (w *plannerWorld) responseDanglingDep() error {
	w.respJSON = fmt.Sprintf(`{"plan":{"name":"p","objective":"s","dag":[%s]}}`, taskNode("g1", []string{"ghost"}))
	return nil
}

func (w *plannerWorld) responseNNodes(n int) error {
	nodes := make([]string, n)
	for i := range n {
		nodes[i] = taskNode(fmt.Sprintf("g%d", i+1), nil)
	}
	w.respJSON = fmt.Sprintf(`{"plan":{"name":"p","objective":"s","dag":[%s]}}`, strings.Join(nodes, ","))
	return nil
}

func (w *plannerWorld) responseNeitherTaskNorStep() error {
	w.respJSON = `{"plan":{"name":"p","objective":"s","dag":[{"id":"g1","deps":[]}]}}`
	return nil
}

func (w *plannerWorld) responseBothTaskAndStep() error {
	w.respJSON = `{"plan":{"name":"p","objective":"s","dag":[
		{"id":"g1","deps":[],
		 "task":{"tool":"list_dir","args":{"path":"."},"explanation":"x"},
		 "step":{"description":"y","deliverable":"z"}}]}}`
	return nil
}

func (w *plannerWorld) parse() error {
	w.plan, w.err = planner.Parse(w.parentID, []byte(w.respJSON))
	return nil
}

func (w *plannerWorld) childCount(n int) error {
	if w.err != nil {
		return fmt.Errorf("parse failed: %w", w.err)
	}
	if got := len(w.plan.Records); got != n {
		return fmt.Errorf("child records = %d, want %d", got, n)
	}
	return nil
}

// childForLocal maps a local step label (g1 => index 0 => parentID-1) to its record.
func (w *plannerWorld) childForLocal(local string) (task.Record, bool) {
	// The parser namespaces by 1-based step order; the canned response lists g1 then g2.
	idx := map[string]int{"g1": 1, "g2": 2}[local]
	want := fmt.Sprintf("%s-%d", w.parentID, idx)
	for _, r := range w.plan.Records {
		if r.ID == want {
			return r, true
		}
	}
	return task.Record{}, false
}

func (w *plannerWorld) childGoalExists(local string) error {
	if _, ok := w.childForLocal(local); !ok {
		return fmt.Errorf("no child record for %q", local)
	}
	return nil
}

func (w *plannerWorld) childDependsOn(local, onLocal string) error {
	child, ok := w.childForLocal(local)
	if !ok {
		return fmt.Errorf("no child record for %q", local)
	}
	on, ok := w.childForLocal(onLocal)
	if !ok {
		return fmt.Errorf("no child record for %q", onLocal)
	}
	if slices.Contains(child.Deps, on.ID) {
		return nil
	}
	return fmt.Errorf("child %q deps %v do not include %q", child.ID, child.Deps, on.ID)
}

func (w *plannerWorld) childIDsNamespaced(parent string) error {
	for _, r := range w.plan.Records {
		if !strings.HasPrefix(r.ID, parent+"-") {
			return fmt.Errorf("child id %q not namespaced under %q", r.ID, parent)
		}
	}
	return nil
}

func (w *plannerWorld) parseFails() error {
	if w.err == nil {
		return fmt.Errorf("expected parse to fail, got plan %+v", w.plan)
	}
	return nil
}
