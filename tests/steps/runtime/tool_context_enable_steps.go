package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/prompting"
	"agentx/internal/runtime"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// toolContextEnableWorld drives enabling a tool_call/tool_result element
// (PD-CTX-AF-011) end to end: it runs the single_tool cycle so a
// tool_call/tool_result is published, toggles one of them enabled or disabled
// through the same orchestrator path the context surface uses, and inspects a
// later turn's captured context plus the context-breakdown "tools" class.
type toolContextEnableWorld struct {
	dir            string
	orc            *runtime.Orchestrator
	captured       []prompting.Message
	enabledCallOrd uint64
	enabledResOrd  uint64
}

func registerToolContextEnableSteps(sc *godog.ScenarioContext) {
	w := &toolContextEnableWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.orc != nil {
			shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = w.orc.Shutdown(shutCtx)
			cancel()
		}
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = toolContextEnableWorld{}
		return ctx, nil
	})

	sc.Step(`^an orchestrator that runs the "([^"]*)" tool, captures context, and answers "([^"]*)"$`, w.start)
	sc.Step(`^the prompt "([^"]*)" runs the enabling tool cycle$`, w.runToolCycle)
	sc.Step(`^the last tool result is enabled$`, w.enableLastResult)
	sc.Step(`^the last tool result is disabled$`, w.disableLastResult)
	sc.Step(`^the last tool call is enabled$`, w.enableLastCall)
	sc.Step(`^the prompt "([^"]*)" is submitted for enabling$`, w.submit)
	sc.Step(`^the enabling-turn context includes "([^"]*)"$`, w.capturedIncludes)
	sc.Step(`^the enabling-turn context omits "([^"]*)"$`, w.capturedOmits)
	sc.Step(`^the enabling context breakdown includes class "([^"]*)"$`, w.breakdownIncludesClass)
}

// start wires an orchestrator whose stub model calls a tool only on its very
// first turn ("list the project"); every later turn answers directly (see
// stubModel's calls-pointer "first call only" semantics), so a follow-up
// turn's captured context reflects only the enabled history — not a second
// live tool call — isolating what the enable toggle actually controls.
func (w *toolContextEnableWorld) start(tool, reply string) error {
	dir, err := os.MkdirTemp("", "agentx-toolenable-")
	if err != nil {
		return err
	}
	w.dir = dir
	runner := stubRunner{res: tools.Result{
		ToolID: tool, Status: "ok", Exit: 0, Lines: 1,
		Preview: "project listing: a.go, b.go",
	}}
	w.orc = runtime.New(
		runtime.Settings{SessionRoot: dir, OllamaModel: "stub", Instructions: "Be brief.", ToolsEnabled: true},
		runtime.WithModel(stubModel{
			deltas:    []string{reply},
			captured:  &w.captured,
			toolCalls: []prompting.ToolCall{{ID: "call_1", Name: tool, Arguments: map[string]any{"path": "."}}},
			calls:     new(int),
		}),
		runtime.WithToolRunner(runner),
	)
	if err := w.orc.Start(); err != nil {
		return err
	}
	// The live bootstrap pins ("date", "git_status"; session.BootstrapFacts) would
	// otherwise refresh through this scenario's single canned stubRunner on every
	// turn, folding "project listing: a.go, b.go" into working memory regardless
	// of the source tool — corrupting the captured-context include/omit
	// assertions this suite exists to isolate.
	_ = w.orc.DeleteFact("date")
	_ = w.orc.DeleteFact("git_status")
	return nil
}

func (w *toolContextEnableWorld) runToolCycle(text string) error {
	return w.orc.Submit(context.Background(), text)
}

func (w *toolContextEnableWorld) submit(text string) error {
	return w.orc.Submit(context.Background(), text)
}

// lastOrdinal polls the durable log (the recorder persists off the hot path) for
// the most recent event of the given content type and returns its ordinal.
func (w *toolContextEnableWorld) lastOrdinal(ct state.ContentType) (uint64, error) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		hist, err := w.orc.History()
		if err != nil {
			return 0, err
		}
		var ord uint64
		for _, ev := range hist {
			if ev.ContentType == ct {
				ord = ev.Ordinal
			}
		}
		if ord != 0 {
			return ord, nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("no %s recorded", ct)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (w *toolContextEnableWorld) enableLastResult() error {
	ord, err := w.lastOrdinal(state.ContentToolResult)
	if err != nil {
		return err
	}
	w.enabledResOrd = ord
	return w.orc.SetEventEnabled(ord, true)
}

func (w *toolContextEnableWorld) disableLastResult() error {
	if w.enabledResOrd == 0 {
		return fmt.Errorf("no tool result has been enabled to disable")
	}
	return w.orc.SetEventEnabled(w.enabledResOrd, false)
}

func (w *toolContextEnableWorld) enableLastCall() error {
	ord, err := w.lastOrdinal(state.ContentToolCall)
	if err != nil {
		return err
	}
	w.enabledCallOrd = ord
	return w.orc.SetEventEnabled(ord, true)
}

func (w *toolContextEnableWorld) capturedText() string {
	var b string
	for _, m := range w.captured {
		b += m.Role + ": " + m.Content + "\n"
	}
	return b
}

func (w *toolContextEnableWorld) capturedIncludes(sub string) error {
	if !strings.Contains(w.capturedText(), sub) {
		return fmt.Errorf("captured context omits %q; got:\n%s", sub, w.capturedText())
	}
	return nil
}

func (w *toolContextEnableWorld) capturedOmits(sub string) error {
	if strings.Contains(w.capturedText(), sub) {
		return fmt.Errorf("captured context unexpectedly includes %q; got:\n%s", sub, w.capturedText())
	}
	return nil
}

func (w *toolContextEnableWorld) breakdownIncludesClass(class string) error {
	report, err := w.orc.ContextBreakdown()
	if err != nil {
		return err
	}
	for _, c := range report.Components {
		if c.Class == class && c.Chars > 0 {
			return nil
		}
	}
	return fmt.Errorf("breakdown omits class %q (or it is zero); got %+v", class, report.Components)
}
