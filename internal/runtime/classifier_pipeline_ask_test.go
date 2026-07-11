package runtime

import (
	"context"
	"testing"
	"time"

	"agentx/internal/prompting/task"
)

// TestResolveAskRouteYes proves the reconcile.Ask dead end is fixed: given a task record
// the reconciler already built, resolveAskRoute asks via the same shared decision gate
// RequestApproval/RequestVerbApproval use, and returns true only on an explicit "yes" —
// previously this route (maybeEmitTask's default case) just logged "skipped: reconciled
// to ask" and never asked anything (reconcile.go: "no visible follow-up — clever-raven-3").
func TestResolveAskRouteYes(t *testing.T) {
	o := testOrchestrator()
	rec := task.Record{ID: "t1", Goal: "delete the old branch"}

	done := make(chan bool, 1)
	go func() {
		done <- o.resolveAskRoute(context.Background(), rec)
	}()
	for !o.gate.deliver("yes") {
		time.Sleep(time.Millisecond)
	}
	if !<-done {
		t.Fatal(`resolveAskRoute("yes") = false, want true`)
	}
}

// TestResolveAskRouteNo: a decline returns false — the caller stays silent rather than
// dispatching an action the user didn't confirm.
func TestResolveAskRouteNo(t *testing.T) {
	o := testOrchestrator()
	rec := task.Record{ID: "t1", Goal: "delete the old branch"}

	done := make(chan bool, 1)
	go func() {
		done <- o.resolveAskRoute(context.Background(), rec)
	}()
	for !o.gate.deliver("no") {
		time.Sleep(time.Millisecond)
	}
	if <-done {
		t.Fatal(`resolveAskRoute("no") = true, want false`)
	}
}

// TestResolveAskRouteCtxCanceled: an interrupt while the decision is pending is treated
// as a decline, never as an implicit yes — dispatching an unconfirmed action would be
// worse than the original silent dead end.
func TestResolveAskRouteCtxCanceled(t *testing.T) {
	o := testOrchestrator()
	rec := task.Record{ID: "t1", Goal: "delete the old branch"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if o.resolveAskRoute(ctx, rec) {
		t.Fatal("resolveAskRoute on a canceled context = true, want false")
	}
}
