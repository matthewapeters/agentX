package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/classify"
	"agentx/internal/prompting"
	"agentx/internal/runtime"
	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/tools"
)

type toolCycleWorld struct {
	dir string
	orc *runtime.Orchestrator
}

// stubRunner is a deterministic ToolRunner returning a fixed result.
type stubRunner struct{ res tools.Result }

func (s stubRunner) Run(context.Context, tools.Descriptor, map[string]string) (tools.Result, error) {
	return s.res, nil
}

// registerToolCycleSteps wires the single_tool cycle steps (TOOL-4).
func registerToolCycleSteps(sc *godog.ScenarioContext) {
	w := &toolCycleWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = toolCycleWorld{}
		return ctx, err
	})

	sc.Step(`^a started orchestrator that runs the "([^"]*)" tool and replies "([^"]*)"$`, w.startRunsTool)
	sc.Step(`^a started orchestrator in read-only mode that proposes "([^"]*)" and replies "([^"]*)"$`, w.startReadOnlyProposes)
	sc.Step(`^the prompt "([^"]*)" runs the tool cycle$`, w.runCycle)
	sc.Step(`^the tool cycle's content events are, in order:$`, w.contentEvents)
	sc.Step(`^the tool cycle's final state is "([^"]*)"$`, w.finalState)
	sc.Step(`^the tool cycle answer is "([^"]*)"$`, w.answerIs)
	sc.Step(`^a tool_result records status "([^"]*)"$`, w.toolResultStatus)
}

func (w *toolCycleWorld) start(proposalJSON, reply string, readOnly bool, runner runtime.ToolRunner) error {
	dir, err := os.MkdirTemp("", "agentx-toolcycle-")
	if err != nil {
		return err
	}
	w.dir = dir
	classifierChat := func(context.Context, []prompting.Message) (string, error) {
		return `{"route": "single_tool"}`, nil
	}
	proposer := tools.NewProposer("catalog", 0, func(context.Context, []prompting.Message) (string, error) {
		return proposalJSON, nil
	})
	w.orc = runtime.New(
		runtime.Settings{SessionRoot: dir, OllamaModel: "stub", ToolsEnabled: true, ToolReadOnly: readOnly},
		runtime.WithModel(stubModel{deltas: []string{reply}}),
		runtime.WithClassifier(classify.New("", 0, classifierChat)),
		runtime.WithProposer(proposer),
		runtime.WithToolRunner(runner),
	)
	return w.orc.Start()
}

func (w *toolCycleWorld) startRunsTool(tool, reply string) error {
	proposal := fmt.Sprintf(`{"tool": %q, "args": {"path": "./x"}}`, tool)
	runner := stubRunner{res: tools.Result{
		ToolID: tool, Status: "ok", Exit: 0, Lines: 1,
		Preview: "file body", Ref: "artifacts/000000000001.txt",
	}}
	return w.start(proposal, reply, true, runner)
}

func (w *toolCycleWorld) startReadOnlyProposes(tool, reply string) error {
	proposal := fmt.Sprintf(`{"tool": %q, "args": {"path": "./x", "content": "hi"}}`, tool)
	return w.start(proposal, reply, true, stubRunner{})
}

func (w *toolCycleWorld) runCycle(text string) error {
	if err := w.orc.Submit(context.Background(), text); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return w.orc.Shutdown(ctx)
}

func (w *toolCycleWorld) timeline() ([]state.Event, error) {
	store := session.NewStore(w.dir)
	return store.Recorder(w.orc.Session().ID).Load()
}

func (w *toolCycleWorld) contentEvents(table *godog.Table) error {
	events, err := w.timeline()
	if err != nil {
		return err
	}
	var got []state.Event
	for _, ev := range events {
		if ev.ContentType == state.ContentProcessingState {
			continue
		}
		got = append(got, ev)
	}
	want := table.Rows[1:]
	if len(got) != len(want) {
		return fmt.Errorf("recorded %d content events, want %d", len(got), len(want))
	}
	for i, row := range want {
		if string(got[i].ContentType) != row.Cells[0].Value {
			return fmt.Errorf("event %d content_type = %q, want %q", i, got[i].ContentType, row.Cells[0].Value)
		}
	}
	return nil
}

func (w *toolCycleWorld) finalState(want string) error {
	if got := string(w.orc.Processing().Current().State); got != want {
		return fmt.Errorf("final state = %q, want %q", got, want)
	}
	return nil
}

func (w *toolCycleWorld) answerIs(want string) error {
	events, err := w.timeline()
	if err != nil {
		return err
	}
	answer := ""
	for _, ev := range events {
		if ev.ContentType != state.ContentAgentResponse {
			continue
		}
		if p, ok := ev.Payload.(map[string]any); ok {
			if t, ok := p["text"].(string); ok {
				answer += t
			}
		}
	}
	if answer != want {
		return fmt.Errorf("tool cycle answer = %q, want %q", answer, want)
	}
	return nil
}

func (w *toolCycleWorld) toolResultStatus(want string) error {
	events, err := w.timeline()
	if err != nil {
		return err
	}
	for _, ev := range events {
		if ev.ContentType != state.ContentToolResult {
			continue
		}
		if p, ok := ev.Payload.(map[string]any); ok {
			if s, _ := p["status"].(string); s == want {
				return nil
			}
		}
	}
	return fmt.Errorf("no tool_result with status %q", want)
}
