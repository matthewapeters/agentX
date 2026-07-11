package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/classify"
	"agentx/internal/prompting"
	"agentx/internal/runtime"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// toolPinWorld drives the pin affordance (PD-CTX-AF-011) end to end: it runs the
// single_tool cycle so a tool_call/tool_result is published, toggles one of them
// enabled (pinned) or disabled (unpinned) through the same orchestrator path the
// context surface uses, and inspects a later turn's captured context plus the
// context-breakdown "tools" class.
type toolPinWorld struct {
	dir           string
	orc           *runtime.Orchestrator
	captured      []prompting.Message
	pinnedCallOrd uint64
	pinnedResOrd  uint64
}

func registerToolPinSteps(sc *godog.ScenarioContext) {
	w := &toolPinWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = toolPinWorld{}
		return ctx, err
	})

	sc.Step(`^an orchestrator that runs the "([^"]*)" tool, captures context, and answers "([^"]*)"$`, w.start)
	sc.Step(`^the prompt "([^"]*)" runs the pinning tool cycle$`, w.runToolCycle)
	sc.Step(`^the last tool result is pinned$`, w.pinLastResult)
	sc.Step(`^the last tool result is unpinned$`, w.unpinLastResult)
	sc.Step(`^the last tool call is pinned$`, w.pinLastCall)
	sc.Step(`^the prompt "([^"]*)" is submitted for pinning$`, w.submit)
	sc.Step(`^the pinning-turn context includes "([^"]*)"$`, w.capturedIncludes)
	sc.Step(`^the pinning-turn context omits "([^"]*)"$`, w.capturedOmits)
	sc.Step(`^the pinning context breakdown includes class "([^"]*)"$`, w.breakdownIncludesClass)
}

// start wires an orchestrator whose classifier routes only the tool-running prompt
// ("list the project") to single_tool; every other prompt answers directly, so a
// follow-up turn's captured context reflects only pinned history — not a second
// live tool call — isolating what the pin toggle actually controls.
func (w *toolPinWorld) start(tool, reply string) error {
	dir, err := os.MkdirTemp("", "agentx-toolpin-")
	if err != nil {
		return err
	}
	w.dir = dir
	classifierChat := func(_ context.Context, msgs []prompting.Message) (string, error) {
		if len(msgs) > 0 && msgs[len(msgs)-1].Content == "list the project" {
			return `{"route": "single_tool"}`, nil
		}
		return `{"route": "respond_directly"}`, nil
	}
	proposal := fmt.Sprintf(`{"tool": %q, "args": {"path": "."}}`, tool)
	proposer := tools.NewProposer("catalog", 0, func(context.Context, []prompting.Message) (string, error) {
		return proposal, nil
	})
	runner := stubRunner{res: tools.Result{
		ToolID: tool, Status: "ok", Exit: 0, Lines: 1,
		Preview: "project listing: a.go, b.go",
	}}
	w.orc = runtime.New(
		runtime.Settings{SessionRoot: dir, OllamaModel: "stub", Instructions: "Be brief.", ToolsEnabled: true, ToolReadOnly: true},
		runtime.WithModel(stubModel{deltas: []string{reply}, captured: &w.captured}),
		runtime.WithClassifier(classify.New("", 0, classifierChat)),
		runtime.WithProposer(proposer),
		runtime.WithToolRunner(runner),
	)
	return w.orc.Start()
}

func (w *toolPinWorld) runToolCycle(text string) error {
	return w.orc.Submit(context.Background(), text)
}

func (w *toolPinWorld) submit(text string) error {
	return w.orc.Submit(context.Background(), text)
}

// lastOrdinal polls the durable log (the recorder persists off the hot path) for
// the most recent event of the given content type and returns its ordinal.
func (w *toolPinWorld) lastOrdinal(ct state.ContentType) (uint64, error) {
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

func (w *toolPinWorld) pinLastResult() error {
	ord, err := w.lastOrdinal(state.ContentToolResult)
	if err != nil {
		return err
	}
	w.pinnedResOrd = ord
	return w.orc.SetEventEnabled(ord, true)
}

func (w *toolPinWorld) unpinLastResult() error {
	if w.pinnedResOrd == 0 {
		return fmt.Errorf("no tool result has been pinned to unpin")
	}
	return w.orc.SetEventEnabled(w.pinnedResOrd, false)
}

func (w *toolPinWorld) pinLastCall() error {
	ord, err := w.lastOrdinal(state.ContentToolCall)
	if err != nil {
		return err
	}
	w.pinnedCallOrd = ord
	return w.orc.SetEventEnabled(ord, true)
}

func (w *toolPinWorld) capturedText() string {
	var b string
	for _, m := range w.captured {
		b += m.Role + ": " + m.Content + "\n"
	}
	return b
}

func (w *toolPinWorld) capturedIncludes(sub string) error {
	if !strings.Contains(w.capturedText(), sub) {
		return fmt.Errorf("captured context omits %q; got:\n%s", sub, w.capturedText())
	}
	return nil
}

func (w *toolPinWorld) capturedOmits(sub string) error {
	if strings.Contains(w.capturedText(), sub) {
		return fmt.Errorf("captured context unexpectedly includes %q; got:\n%s", sub, w.capturedText())
	}
	return nil
}

func (w *toolPinWorld) breakdownIncludesClass(class string) error {
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
