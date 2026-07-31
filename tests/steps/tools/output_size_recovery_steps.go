package toolsteps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/prompting"
	"agentx/internal/runtime"
	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// stubModel is a no-op Model: this domain never calls Propose/Chat (it drives
// Orchestrator.RequestOutputSizeDecision directly), so it only needs to satisfy
// runtime.Model well enough for Start to succeed.
type stubModel struct{}

func (stubModel) Chat(context.Context, string, []prompting.Message, []tools.ToolSchema, func(string), func(string)) (runtime.ChatResult, error) {
	return runtime.ChatResult{}, nil
}
func (stubModel) Ready(context.Context, string) error                { return nil }
func (stubModel) ContextLength(context.Context, string) (int, error) { return 4096, nil }

type outputSizeOutcome struct {
	res tools.Result
	ok  bool
	err error
}

type outputSizeWorld struct {
	dir           string
	overridesPath string
	orc           *runtime.Orchestrator
	ctx           context.Context
	cancel        context.CancelFunc
	desc          tools.Descriptor
	args          map[string]string
	res           tools.Result
	result        chan outputSizeOutcome
	out           *outputSizeOutcome
}

// registerOutputSizeSteps wires the oversized-output recovery gate steps (TOOL-6,
// Phase A).
func registerOutputSizeSteps(sc *godog.ScenarioContext) {
	w := &outputSizeWorld{}

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
		*w = outputSizeWorld{}
		return ctx, nil
	})

	sc.Step(`^a started orchestrator with a small output cap and a large absolute cap$`, func() error { return w.started(5000) })
	sc.Step(`^a started orchestrator with a small output cap and a tiny absolute cap$`, func() error { return w.started(100) })
	sc.Step(`^a persisted expand override for the command with an oversized cap$`, w.persistedOverride)
	sc.Step(`^a command that emits (\d+) numbered lines is run and truncated$`, w.runAndTruncate)
	sc.Step(`^an output-size decision is requested$`, w.requestDecision)
	sc.Step(`^an output-size decision is requested and resolves without prompting$`, w.requestDecisionNoPrompt)
	sc.Step(`^a second output-size decision for the same command is requested$`, w.requestSecondDecision)
	sc.Step(`^a second output-size decision for the same command resolves without prompting$`, w.requestSecondDecisionNoPrompt)
	sc.Step(`^the user chooses "([^"]*)"$`, w.chooseDecision)
	sc.Step(`^the awaiting output-size request is interrupted$`, w.interrupt)
	sc.Step(`^the decision is accepted$`, w.decisionAccepted)
	sc.Step(`^the decision is declined$`, w.decisionDeclined)
	sc.Step(`^the resulting result is truncated$`, w.resultTruncated)
	sc.Step(`^the resulting result is not truncated$`, w.resultNotTruncated)
	sc.Step(`^the resulting result reports (\d+) lines$`, w.resultReportsLines)
	sc.Step(`^the resulting preview mentions "([^"]*)"$`, w.previewMentions)
	sc.Step(`^the output-size request fails$`, w.requestFailed)
}

func (w *outputSizeWorld) ensureDir() error {
	if w.dir != "" {
		return nil
	}
	dir, err := os.MkdirTemp("", "agentx-outsize-")
	if err != nil {
		return err
	}
	w.dir = dir
	w.overridesPath = filepath.Join(w.dir, "agentx-tool-output-overrides.toml")
	return nil
}

func (w *outputSizeWorld) started(absoluteMaxBytes int) error {
	if err := w.ensureDir(); err != nil {
		return err
	}
	w.orc = runtime.New(runtime.Settings{
		SessionRoot:                filepath.Join(w.dir, "sessions"),
		OllamaModel:                "stub",
		ToolsEnabled:               true,
		ToolOutputMaxBytes:         20,
		ToolOutputAbsoluteMaxBytes: absoluteMaxBytes,
		ToolOutputOverridesPath:    w.overridesPath,
	}, runtime.WithModel(stubModel{}))
	w.ctx, w.cancel = context.WithCancel(context.Background())
	return w.orc.Start()
}

func (w *outputSizeWorld) persistedOverride() error {
	if err := w.ensureDir(); err != nil {
		return err
	}
	return tools.SaveOutputOverrides(w.overridesPath, []tools.OutputOverride{
		{Tool: "seq", Decision: "expand", CapBytes: 999999},
	})
}

// runAndTruncate seeds w.res with a genuinely truncated result: it runs a real
// "seq 1 N" through a throwaway, deliberately small-capped executor (separate
// from the orchestrator under test), so the recovery gate is exercised against a
// real capture, not a hand-built stand-in.
func (w *outputSizeWorld) runAndTruncate(n int) error {
	w.desc = tools.Descriptor{
		ID: "seq", Command: "seq", Argv: []string{"seq", "1", "{n}"},
		TimeoutSeconds: 30,
		Args:           []tools.ArgSpec{{Name: "n", Kind: tools.KindString, Required: true}},
	}
	w.args = map[string]string{"n": strconv.Itoa(n)}

	seedDir, err := os.MkdirTemp("", "agentx-outsize-seed-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(seedDir)
	store := session.NewStore(filepath.Join(seedDir, "sessions"))
	id, err := store.Create()
	if err != nil {
		return err
	}
	art, err := store.Artifacts(id.ID)
	if err != nil {
		return err
	}
	res, err := tools.NewExecutor(art, 20).Run(context.Background(), w.desc, w.args)
	if err != nil {
		return err
	}
	if !res.Truncated {
		return fmt.Errorf("seed run unexpectedly not truncated (bytes=%d)", res.Bytes)
	}
	w.res = res
	return nil
}

func (w *outputSizeWorld) fire() {
	w.result = make(chan outputSizeOutcome, 1)
	go func() {
		res, ok, err := w.orc.RequestOutputSizeDecision(w.ctx, w.desc, w.args, w.res)
		w.result <- outputSizeOutcome{res: res, ok: ok, err: err}
	}()
}

func (w *outputSizeWorld) requestDecision() error {
	w.out = nil
	// Reset processing state before firing: RequestDecision never itself clears
	// StateAwaitingInput on resolution (its caller normally does, e.g. finishCycle
	// at the end of a real prompt cycle) — this test calls
	// RequestOutputSizeDecision directly, bypassing that caller, so a second
	// sequential request would otherwise see the FIRST request's already-resolved
	// awaiting_input state and return immediately, racing ahead of the second
	// request's actual enqueue.
	w.orc.Processing().Set(state.StateWorking, state.PhaseNone)
	w.fire()
	return w.waitAwaiting()
}

func (w *outputSizeWorld) waitAwaiting() error {
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

// requestDecisionNoPrompt fires a request expected to resolve on its own (a
// remembered override applies) — it must complete well inside the short window
// below without ever needing a Resolve() call.
func (w *outputSizeWorld) requestDecisionNoPrompt() error {
	w.out = nil
	w.fire()
	select {
	case o := <-w.result:
		w.out = &o
		return nil
	case <-time.After(500 * time.Millisecond):
		return fmt.Errorf("request unexpectedly blocked on a prompt")
	}
}

func (w *outputSizeWorld) requestSecondDecision() error {
	if _, err := w.outcome(); err != nil {
		return err
	}
	return w.requestDecision()
}

func (w *outputSizeWorld) requestSecondDecisionNoPrompt() error {
	if _, err := w.outcome(); err != nil {
		return err
	}
	return w.requestDecisionNoPrompt()
}

func (w *outputSizeWorld) chooseDecision(decision string) error {
	w.orc.Resolve(decision)
	return nil
}

func (w *outputSizeWorld) interrupt() error {
	w.cancel()
	return nil
}

func (w *outputSizeWorld) outcome() (outputSizeOutcome, error) {
	if w.out != nil {
		return *w.out, nil
	}
	select {
	case o := <-w.result:
		w.out = &o
		return o, nil
	case <-time.After(2 * time.Second):
		return outputSizeOutcome{}, fmt.Errorf("timed out waiting for output-size outcome")
	}
}

func (w *outputSizeWorld) decisionAccepted() error {
	o, err := w.outcome()
	if err != nil {
		return err
	}
	if !o.ok {
		return fmt.Errorf("decision not accepted (err=%v)", o.err)
	}
	return nil
}

func (w *outputSizeWorld) decisionDeclined() error {
	o, err := w.outcome()
	if err != nil {
		return err
	}
	if o.ok {
		return fmt.Errorf("decision unexpectedly accepted")
	}
	return nil
}

func (w *outputSizeWorld) resultTruncated() error {
	o, err := w.outcome()
	if err != nil {
		return err
	}
	if !o.res.Truncated {
		return fmt.Errorf("result unexpectedly not truncated (bytes=%d)", o.res.Bytes)
	}
	return nil
}

func (w *outputSizeWorld) resultNotTruncated() error {
	o, err := w.outcome()
	if err != nil {
		return err
	}
	if o.res.Truncated {
		return fmt.Errorf("result unexpectedly still truncated (bytes=%d)", o.res.Bytes)
	}
	return nil
}

func (w *outputSizeWorld) resultReportsLines(n int) error {
	o, err := w.outcome()
	if err != nil {
		return err
	}
	if o.res.Lines != n {
		return fmt.Errorf("result reports %d lines, want %d", o.res.Lines, n)
	}
	return nil
}

func (w *outputSizeWorld) previewMentions(substr string) error {
	o, err := w.outcome()
	if err != nil {
		return err
	}
	if !strings.Contains(o.res.Preview, substr) {
		return fmt.Errorf("preview %q does not contain %q", o.res.Preview, substr)
	}
	return nil
}

func (w *outputSizeWorld) requestFailed() error {
	o, err := w.outcome()
	if err != nil {
		return err
	}
	if o.err == nil {
		return fmt.Errorf("request did not fail")
	}
	return nil
}
