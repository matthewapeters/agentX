package promptingsteps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"agentx/internal/prompting/reconcile"
)

type reconcileWorld struct {
	turn  reconcile.TurnSignal
	resp  reconcile.ResponseSignal
	route reconcile.Route
}

func registerReconcileSteps(sc *godog.ScenarioContext) {
	w := &reconcileWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = reconcileWorld{}
		return ctx, nil
	})

	sc.Step(`^the turn is actionable$`, w.turnActionable)
	sc.Step(`^the turn is not actionable$`, w.turnNotActionable)
	sc.Step(`^the turn classification abstained$`, w.turnAbstained)
	sc.Step(`^the turn abstained leaning toward actionable$`, w.turnAbstainedLeansActionable)
	sc.Step(`^the turn abstained with a scatter toward none$`, w.turnAbstainedScatterNone)
	sc.Step(`^the response executed the action$`, w.responseExecuted)
	sc.Step(`^the response produced the action without executing it$`, w.responseProduced)
	sc.Step(`^the response did nothing$`, w.responseNothing)
	sc.Step(`^the response abstained leaning toward produced$`, w.responseAbstainedLeansProduced)
	sc.Step(`^the response abstained with a scatter toward none$`, w.responseAbstainedScatterNone)
	sc.Step(`^the classifications are reconciled$`, w.reconcile)
	sc.Step(`^the route is "([^"]*)"$`, w.routeIs)
}

func (w *reconcileWorld) turnActionable() error {
	w.turn.Actionable = true
	return nil
}

func (w *reconcileWorld) turnNotActionable() error {
	w.turn.Actionable = false
	return nil
}

func (w *reconcileWorld) turnAbstained() error {
	w.turn.Abstained = true
	return nil
}

func (w *reconcileWorld) turnAbstainedLeansActionable() error {
	w.turn.Abstained = true
	w.turn.LeansActionable = true
	return nil
}

func (w *reconcileWorld) turnAbstainedScatterNone() error {
	w.turn.Abstained = true
	w.turn.LeansActionable = false
	return nil
}

func (w *reconcileWorld) responseExecuted() error {
	w.resp.Executed = true
	w.resp.Produced = true
	return nil
}

func (w *reconcileWorld) responseProduced() error {
	w.resp.Produced = true
	return nil
}

func (w *reconcileWorld) responseNothing() error {
	w.resp = reconcile.ResponseSignal{}
	return nil
}

func (w *reconcileWorld) responseAbstainedLeansProduced() error {
	w.resp.Abstained = true
	w.resp.LeansProduced = true
	return nil
}

func (w *reconcileWorld) responseAbstainedScatterNone() error {
	w.resp.Abstained = true
	w.resp.LeansProduced = false
	return nil
}

func (w *reconcileWorld) reconcile() error {
	w.route, _ = reconcile.Reconcile(w.turn, w.resp)
	return nil
}

func (w *reconcileWorld) routeIs(want string) error {
	if got := string(w.route); got != want {
		return fmt.Errorf("route = %q, want %q", got, want)
	}
	return nil
}
