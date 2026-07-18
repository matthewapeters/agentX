package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/runtime"
	"agentx/internal/state"
	"agentx/internal/tools"
)

type approvalOutcome struct {
	verdict tools.Verdict
	err     error
}

type approvalWorld struct {
	dir    string
	orc    *runtime.Orchestrator
	pol    *tools.Policy
	reg    *tools.Registry
	desc   tools.Descriptor
	args   map[string]string
	ctx    context.Context
	cancel context.CancelFunc
	result chan approvalOutcome
	out    *approvalOutcome
}

// registerApprovalSteps wires the tool-approval round-trip steps (TOOL-3).
func registerApprovalSteps(sc *godog.ScenarioContext) {
	w := &approvalWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.cancel != nil {
			w.cancel()
		}
		if w.orc != nil {
			shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = w.orc.Shutdown(shutCtx)
			cancel()
		}
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = approvalWorld{}
		return ctx, nil
	})

	sc.Step(`^a started orchestrator with a tool policy$`, w.started)
	sc.Step(`^approval is requested for "([^"]*)" with:$`, w.requestApproval)
	sc.Step(`^the user approves for the "([^"]*)"$`, w.approve)
	sc.Step(`^the user denies the request$`, w.deny)
	sc.Step(`^the awaiting request is interrupted$`, w.interrupt)
	sc.Step(`^the request is allowed$`, w.requestAllowed)
	sc.Step(`^the request is denied$`, w.requestDenied)
	sc.Step(`^the request fails$`, w.requestFailed)
	sc.Step(`^"([^"]*)" with the same arguments is now allowed by policy$`, w.nowAllowed)
	sc.Step(`^"([^"]*)" with the same arguments still needs approval$`, w.stillNeedsApproval)
}

func (w *approvalWorld) started() error {
	dir, err := os.MkdirTemp("", "agentx-approval-")
	if err != nil {
		return err
	}
	w.dir = dir
	w.pol = tools.NewPolicy()
	w.reg = tools.DefaultRegistry()
	w.orc = runtime.New(runtime.Settings{SessionRoot: dir, OllamaModel: "stub"}, runtime.WithModel(stubModel{}))
	w.ctx, w.cancel = context.WithCancel(context.Background())
	return w.orc.Start()
}

func (w *approvalWorld) requestApproval(id string, table *godog.Table) error {
	d, ok := w.reg.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown tool %q", id)
	}
	w.desc = d
	w.args = argsFromTable(table)
	w.result = make(chan approvalOutcome, 1)
	go func() {
		v, err := w.orc.RequestApproval(w.ctx, d, w.args, w.pol)
		w.result <- approvalOutcome{verdict: v, err: err}
	}()
	// Wait until the cycle is parked in awaiting_input so the gate is armed before
	// the decision is delivered.
	return w.waitAwaiting()
}

func (w *approvalWorld) waitAwaiting() error {
	deadline := time.After(2 * time.Second)
	for {
		if w.orc.Processing().Current().State == state.StateAwaitingInput {
			return nil
		}
		select {
		case <-deadline:
			return fmt.Errorf("request never reached awaiting_input")
		case <-time.After(2 * time.Millisecond):
		}
	}
}

func (w *approvalWorld) approve(scope string) error {
	w.orc.Resolve(scope)
	return nil
}

func (w *approvalWorld) deny() error {
	w.orc.Resolve("deny")
	return nil
}

func (w *approvalWorld) interrupt() error {
	w.cancel()
	return nil
}

func (w *approvalWorld) outcome() (approvalOutcome, error) {
	if w.out != nil {
		return *w.out, nil
	}
	select {
	case o := <-w.result:
		w.out = &o
		return o, nil
	case <-time.After(2 * time.Second):
		return approvalOutcome{}, fmt.Errorf("timed out waiting for approval outcome")
	}
}

func (w *approvalWorld) requestAllowed() error {
	o, err := w.outcome()
	if err != nil {
		return err
	}
	if o.err != nil || o.verdict.Decision != tools.Allow {
		return fmt.Errorf("request not allowed: decision=%v err=%v", o.verdict.Decision, o.err)
	}
	return nil
}

func (w *approvalWorld) requestDenied() error {
	o, err := w.outcome()
	if err != nil {
		return err
	}
	if o.err != nil || o.verdict.Decision != tools.Deny {
		return fmt.Errorf("request not denied: decision=%v err=%v", o.verdict.Decision, o.err)
	}
	return nil
}

func (w *approvalWorld) requestFailed() error {
	o, err := w.outcome()
	if err != nil {
		return err
	}
	if o.err == nil {
		return fmt.Errorf("request did not fail: decision=%v", o.verdict.Decision)
	}
	return nil
}

func (w *approvalWorld) nowAllowed(id string) error {
	d, _ := w.reg.Lookup(id)
	if v := w.pol.Evaluate(d, w.args); v.Decision != tools.Allow {
		return fmt.Errorf("policy still does not allow %q: decision=%v", id, v.Decision)
	}
	return nil
}

func (w *approvalWorld) stillNeedsApproval(id string) error {
	d, _ := w.reg.Lookup(id)
	if v := w.pol.Evaluate(d, w.args); v.Decision != tools.NeedsApproval {
		return fmt.Errorf("policy unexpectedly changed for %q: decision=%v", id, v.Decision)
	}
	return nil
}

func argsFromTable(table *godog.Table) map[string]string {
	args := map[string]string{}
	for _, row := range table.Rows[1:] {
		args[row.Cells[0].Value] = row.Cells[1].Value
	}
	return args
}
