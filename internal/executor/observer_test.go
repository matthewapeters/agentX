package executor

import (
	"context"
	"testing"

	"agentx/internal/prompting/task"
	"agentx/internal/tools"
)

type stubProposer struct{ prop tools.Proposal }

func (s stubProposer) Propose(context.Context, string) (tools.Proposal, bool) { return s.prop, true }

type stubRegistry struct{ d tools.Descriptor }

func (s stubRegistry) Lookup(string) (tools.Descriptor, bool) { return s.d, true }

type stubGate struct{ v tools.Verdict }

func (s stubGate) Evaluate(tools.Descriptor, map[string]string) tools.Verdict { return s.v }

type stubRunner struct{ res tools.Result }

func (s stubRunner) Run(context.Context, tools.Descriptor, map[string]string) (tools.Result, error) {
	return s.res, nil
}

// recCallObserver records the call lifecycle in order.
type recCallObserver struct{ events []string }

func (r *recCallObserver) ToolCalled(rec task.Record, d tools.Descriptor, _ map[string]string) {
	r.events = append(r.events, "called:"+d.ID+":"+rec.ID)
}
func (r *recCallObserver) ToolFinished(rec task.Record, d tools.Descriptor, _ tools.Result, st Status) {
	r.events = append(r.events, "finished:"+d.ID+":"+rec.ID+":"+string(st))
}

// TestCallObserverAnnouncesBeforeRun: the resolved call is announced pre-run and its
// terminal status reported — the ADR 0009 pre-execution visibility seam.
func TestCallObserverAnnouncesBeforeRun(t *testing.T) {
	obs := &recCallObserver{}
	e := New(
		stubProposer{prop: tools.Proposal{Tool: "list_dir", Args: map[string]string{"path": "."}}},
		stubRegistry{d: tools.Descriptor{ID: "list_dir", Risk: tools.RiskRead}},
		stubGate{v: tools.Verdict{Decision: tools.Allow}},
		stubRunner{res: tools.Result{ToolID: "list_dir", Status: "ok", Exit: 0, Bytes: 9, Preview: "cmd/ docs"}},
		FSVerifier{},
		WithCallObserver(obs),
	)
	out := e.Execute(context.Background(), task.Record{ID: "task-1-1", Goal: "list the project root"})
	if out.Status != Executed {
		t.Fatalf("outcome = %s (%s), want executed", out.Status, out.Reason)
	}
	want := []string{"called:list_dir:task-1-1", "finished:list_dir:task-1-1:executed"}
	if len(obs.events) != 2 || obs.events[0] != want[0] || obs.events[1] != want[1] {
		t.Fatalf("events = %v, want %v", obs.events, want)
	}
}

// TestCallObserverReportsDenied: a policy-blocked call is announced and reported denied —
// a blocked attempt is as legible as a successful one.
func TestCallObserverReportsDenied(t *testing.T) {
	obs := &recCallObserver{}
	e := New(
		stubProposer{prop: tools.Proposal{Tool: "run_cmd", Args: map[string]string{}}},
		stubRegistry{d: tools.Descriptor{ID: "run_cmd", Risk: tools.RiskWrite}},
		stubGate{v: tools.Verdict{Decision: tools.Deny, Reason: "blacklisted"}},
		stubRunner{},
		FSVerifier{},
		WithCallObserver(obs),
	)
	out := e.Execute(context.Background(), task.Record{ID: "task-9", Goal: "run something"})
	if out.Status != Denied {
		t.Fatalf("outcome = %s, want denied", out.Status)
	}
	want := []string{"called:run_cmd:task-9", "finished:run_cmd:task-9:denied"}
	if len(obs.events) != 2 || obs.events[0] != want[0] || obs.events[1] != want[1] {
		t.Fatalf("events = %v, want %v", obs.events, want)
	}
}

// failProposer fails the test if Propose is ever called — proving a pre-resolved Task
// skips the proposer LLM hop entirely (ADR 0008 amendment).
type failProposer struct{ t *testing.T }

func (f failProposer) Propose(context.Context, string) (tools.Proposal, bool) {
	f.t.Fatal("proposer.Propose called for a record that already carried a resolved tool call")
	return tools.Proposal{}, false
}

// TestResolvedProposalSkipsProposer: a planner-produced Task (Params carries "tool"/"args"
// straight from planner.Parse, so "args" arrives as map[string]any) never calls the
// proposer, and still flows through gate/run/verify/observer normally.
func TestResolvedProposalSkipsProposer(t *testing.T) {
	obs := &recCallObserver{}
	e := New(
		failProposer{t: t},
		stubRegistry{d: tools.Descriptor{ID: "list_dir", Risk: tools.RiskRead}},
		stubGate{v: tools.Verdict{Decision: tools.Allow}},
		stubRunner{res: tools.Result{ToolID: "list_dir", Status: "ok", Exit: 0, Bytes: 9, Preview: "cmd/ docs"}},
		FSVerifier{},
		WithCallObserver(obs),
	)
	rec := task.Record{
		ID: "task-5-1", Kind: task.KindTask, Goal: "list the project root",
		Params: map[string]any{"tool": "list_dir", "args": map[string]any{"path": "."}},
	}
	out := e.Execute(context.Background(), rec)
	if out.Status != Executed {
		t.Fatalf("outcome = %s (%s), want executed", out.Status, out.Reason)
	}
	want := []string{"called:list_dir:task-5-1", "finished:list_dir:task-5-1:executed"}
	if len(obs.events) != 2 || obs.events[0] != want[0] || obs.events[1] != want[1] {
		t.Fatalf("events = %v, want %v", obs.events, want)
	}
}

// TestResolvedProposalHandlesJSONRoundTripShape: Params["args"] as map[string]string (the
// shape it would have if never JSON-round-tripped) also resolves correctly.
func TestResolvedProposalHandlesJSONRoundTripShape(t *testing.T) {
	rec := task.Record{
		ID: "task-5-2", Kind: task.KindTask,
		Params: map[string]any{"tool": "list_dir", "args": map[string]string{"path": "/tmp"}},
	}
	prop, ok := resolvedProposal(rec)
	if !ok || prop.Tool != "list_dir" || prop.Args["path"] != "/tmp" {
		t.Errorf("resolvedProposal = %+v, ok=%v", prop, ok)
	}
}

// TestResolvedProposalFalseWithoutParams: a Redispatch/Reify-route record (goal only, no
// Params) is not treated as pre-resolved.
func TestResolvedProposalFalseWithoutParams(t *testing.T) {
	if _, ok := resolvedProposal(task.Record{ID: "task-6", Goal: "do something"}); ok {
		t.Error("resolvedProposal true for a record with no Params")
	}
}
