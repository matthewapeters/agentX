// Package executorsteps implements the Godog steps for internal/executor.
package executorsteps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/tools"
)

// --- stub collaborators ------------------------------------------------------

type stubProposer struct {
	tool string
	path string
	ok   bool
}

func (s stubProposer) Propose(context.Context, string) (tools.Proposal, bool) {
	path := s.path
	if path == "" {
		path = "out.txt"
	}
	return tools.Proposal{Tool: s.tool, Args: map[string]string{"path": path}}, s.ok
}

type stubRegistry struct{}

func (stubRegistry) Lookup(id string) (tools.Descriptor, bool) {
	return tools.Descriptor{ID: id}, true
}

type stubGate struct {
	decision tools.Decision
	reason   string
}

func (g stubGate) Evaluate(tools.Descriptor, map[string]string, string) tools.Verdict {
	return tools.Verdict{Decision: g.decision, Reason: g.reason}
}

type stubRunner struct {
	res tools.Result
	err error
	ran *bool
}

func (r stubRunner) Run(context.Context, tools.Descriptor, map[string]string) (tools.Result, error) {
	if r.ran != nil {
		*r.ran = true
	}
	return r.res, r.err
}

type stubVerifier struct{ ok bool }

func (v stubVerifier) Verify(tools.Descriptor, map[string]string, tools.Result) bool { return v.ok }

// --- world -------------------------------------------------------------------

type world struct {
	proposer stubProposer
	gate     stubGate
	runner   stubRunner
	verifier stubVerifier
	ranFlag  bool
	outcome  executor.Outcome

	// confinement scenarios
	root     string
	approver executor.Approver

	// filesystem-verifier scenarios
	fsRoot   string
	fsResult tools.Result
	fsArgs   map[string]string
	fsOK     bool
}

// InitializeScenario registers the executor steps.
func InitializeScenario(sc *godog.ScenarioContext) {
	w := &world{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = world{gate: stubGate{decision: tools.Allow}, fsArgs: map[string]string{}}
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.fsRoot != "" {
			_ = os.RemoveAll(w.fsRoot)
		}
		return ctx, nil
	})

	sc.Step(`^a proposer that proposes tool "([^"]*)"$`, w.proposesTool)
	sc.Step(`^a proposer that proposes writing to "([^"]*)"$`, w.proposesWriting)
	sc.Step(`^a proposer that proposes nothing$`, w.proposesNothing)
	sc.Step(`^the executor is confined to "([^"]*)"$`, w.confinedTo)
	sc.Step(`^the user approves the flagged call$`, w.userApproves)
	sc.Step(`^the user declines the flagged call$`, w.userDeclines)
	sc.Step(`^the policy allows the call$`, w.policyAllows)
	sc.Step(`^the policy denies the call$`, w.policyDenies)
	sc.Step(`^the runner reports status "([^"]*)"$`, w.runnerStatus)
	sc.Step(`^the effect verifies$`, w.effectVerifies)
	sc.Step(`^the effect does not verify$`, w.effectNotVerifies)
	sc.Step(`^the task "([^"]*)" is executed$`, w.execute)
	sc.Step(`^the executor outcome is "([^"]*)"$`, w.outcomeIs)
	sc.Step(`^the runner did not run$`, w.runnerDidNotRun)

	sc.Step(`^a runner result with status "([^"]*)" and exit (\d+)$`, w.fsRunnerResult)
	sc.Step(`^the call wrote a non-empty file at "([^"]*)"$`, w.fsWroteFile)
	sc.Step(`^the call named a file at "([^"]*)" that was never written$`, w.fsMissingFile)
	sc.Step(`^the filesystem effect is verified$`, w.fsVerify)
	sc.Step(`^the effect is confirmed$`, w.fsConfirmed)
	sc.Step(`^the effect is rejected$`, w.fsRejected)
}

// --- executor steps ----------------------------------------------------------

func (w *world) proposesTool(tool string) error {
	w.proposer = stubProposer{tool: tool, ok: true}
	return nil
}

func (w *world) proposesWriting(path string) error {
	w.proposer = stubProposer{tool: "write_file", path: path, ok: true}
	return nil
}

func (w *world) proposesNothing() error {
	w.proposer = stubProposer{ok: false}
	return nil
}

func (w *world) confinedTo(root string) error {
	w.root = root
	return nil
}

func (w *world) userApproves() error {
	w.approver = executor.ApproverFunc(func(context.Context, tools.Descriptor, map[string]string, task.Record, string) bool { return true })
	return nil
}

func (w *world) userDeclines() error {
	w.approver = executor.ApproverFunc(func(context.Context, tools.Descriptor, map[string]string, task.Record, string) bool { return false })
	return nil
}

func (w *world) policyAllows() error {
	w.gate = stubGate{decision: tools.Allow}
	return nil
}

func (w *world) policyDenies() error {
	w.gate = stubGate{decision: tools.Deny, reason: "blacklisted"}
	return nil
}

func (w *world) runnerStatus(status string) error {
	w.runner = stubRunner{res: tools.Result{Status: status, Exit: 0}, ran: &w.ranFlag}
	return nil
}

func (w *world) effectVerifies() error {
	w.verifier = stubVerifier{ok: true}
	return nil
}

func (w *world) effectNotVerifies() error {
	w.verifier = stubVerifier{ok: false}
	return nil
}

func (w *world) execute(goal string) error {
	if w.runner.ran == nil {
		w.runner.ran = &w.ranFlag
	}
	var opts []executor.Option
	if w.root != "" {
		opts = append(opts, executor.WithRoot(w.root))
	}
	if w.approver != nil {
		opts = append(opts, executor.WithApprover(w.approver))
	}
	ex := executor.New(w.proposer, stubRegistry{}, w.gate, w.runner, w.verifier, opts...)
	w.outcome = ex.Execute(context.Background(), task.Record{Goal: goal})
	return nil
}

func (w *world) outcomeIs(want string) error {
	if got := string(w.outcome.Status); got != want {
		return fmt.Errorf("outcome = %q (%s), want %q", got, w.outcome.Reason, want)
	}
	return nil
}

func (w *world) runnerDidNotRun() error {
	if w.ranFlag {
		return fmt.Errorf("runner ran but should not have")
	}
	return nil
}

// --- filesystem-verifier steps ----------------------------------------------

func (w *world) fsRunnerResult(status string, exit int) error {
	w.fsResult = tools.Result{Status: status, Exit: exit}
	return nil
}

func (w *world) fsWroteFile(name string) error {
	dir, err := os.MkdirTemp("", "exec-verify-")
	if err != nil {
		return err
	}
	w.fsRoot = dir
	if err := os.WriteFile(filepath.Join(dir, name), []byte("body"), 0o644); err != nil {
		return err
	}
	w.fsArgs["path"] = name
	return nil
}

func (w *world) fsMissingFile(name string) error {
	dir, err := os.MkdirTemp("", "exec-verify-")
	if err != nil {
		return err
	}
	w.fsRoot = dir // exists but the named file is never created
	w.fsArgs["path"] = name
	return nil
}

func (w *world) fsVerify() error {
	v := executor.FSVerifier{Root: w.fsRoot}
	w.fsOK = v.Verify(tools.Descriptor{}, w.fsArgs, w.fsResult)
	return nil
}

func (w *world) fsConfirmed() error {
	if !w.fsOK {
		return fmt.Errorf("expected the effect to be confirmed")
	}
	return nil
}

func (w *world) fsRejected() error {
	if w.fsOK {
		return fmt.Errorf("expected the effect to be rejected")
	}
	return nil
}
