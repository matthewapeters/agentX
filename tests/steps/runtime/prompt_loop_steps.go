package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/prompting"
	"agentx/internal/runtime"
	"agentx/internal/session"
	"agentx/internal/state"
)

// promptLoopWorld drives the native tool-calling loop end to end (see
// docs/implementation/04_llm_prompt_tooling_runtime.md, "The Prompt/Response
// Loop"): no classify step, thinking applies uniformly rather than per
// classified route, and plan_task is a tool the model calls at its own
// discretion rather than a pre-classifier route.
type promptLoopWorld struct {
	dir string
	orc *runtime.Orchestrator
}

func registerPromptLoopSteps(sc *godog.ScenarioContext) {
	w := &promptLoopWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = promptLoopWorld{}
		return ctx, nil
	})

	sc.Step(`^a started orchestrator with a stub model that replies "([^"]*)"$`, w.startPlain)
	sc.Step(`^a started orchestrator with thinking enabled whose model thinks "([^"]*)" then replies "([^"]*)"$`, w.startThinking)
	sc.Step(`^a started orchestrator with thinking enabled whose thinking stalls past the budget then replies "([^"]*)"$`, w.startThinkingStall)
	sc.Step(`^a started orchestrator with plan_task wired whose model calls plan_task then replies "([^"]*)"$`, w.startPlanTask)
	sc.Step(`^the prompt "([^"]*)" runs the loop$`, w.runLoop)
	sc.Step(`^the loop's content events are, in order:$`, w.contentEvents)
	sc.Step(`^the loop's final state is "([^"]*)"$`, w.finalState)
	sc.Step(`^the loop answer is "([^"]*)"$`, w.answerIs)
	sc.Step(`^the loop's timeline contains a task_plan event$`, w.timelineHasTaskPlan)
}

func (w *promptLoopWorld) newDir(prefix string) error {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return err
	}
	w.dir = dir
	return nil
}

func (w *promptLoopWorld) startPlain(reply string) error {
	if err := w.newDir("agentx-loop-"); err != nil {
		return err
	}
	w.orc = runtime.New(
		runtime.Settings{SessionRoot: w.dir, OllamaModel: "stub"},
		runtime.WithModel(stubModel{deltas: []string{reply}}),
	)
	return w.orc.Start()
}

func (w *promptLoopWorld) startThinking(think, reply string) error {
	if err := w.newDir("agentx-loop-think-"); err != nil {
		return err
	}
	w.orc = runtime.New(
		runtime.Settings{SessionRoot: w.dir, OllamaModel: "stub", ThinkingEnabled: true},
		runtime.WithModel(stubModel{thinks: []string{think}, deltas: []string{reply}}),
	)
	return w.orc.Start()
}

func (w *promptLoopWorld) startThinkingStall(reply string) error {
	if err := w.newDir("agentx-loop-thinkstall-"); err != nil {
		return err
	}
	w.orc = runtime.New(
		runtime.Settings{
			SessionRoot: w.dir, OllamaModel: "stub",
			ThinkingEnabled: true, ThinkingBudget: 20 * time.Millisecond,
		},
		runtime.WithModel(stubModel{thinks: []string{"stalling"}, thinkBlocks: true, deltas: []string{reply}}),
	)
	return w.orc.Start()
}

// startPlanTask wires plan_task's decomposition substrate directly (bypassing
// tools entirely — planReady only needs a decomposer + executor, see
// Orchestrator.planReady), with a decomposer that decomposes the root into no
// children, so the plan drains immediately with nothing to execute. That's
// enough to prove the wiring (schema advertised, tool call dispatched to
// runPlanTaskTool, a task_plan event published, the result folded back into
// the loop) without depending on decompose's own scheduling mechanics, which
// internal/runtime/decompose already tests directly.
func (w *promptLoopWorld) startPlanTask(reply string) error {
	if err := w.newDir("agentx-loop-plantask-"); err != nil {
		return err
	}
	w.orc = runtime.New(
		runtime.Settings{SessionRoot: w.dir, OllamaModel: "stub"},
		runtime.WithModel(stubModel{
			deltas: []string{reply},
			toolCalls: []prompting.ToolCall{
				{ID: "call_1", Name: "plan_task", Arguments: map[string]any{"goal": "review this project"}},
			},
			calls: new(int),
		}),
		runtime.WithTaskExecutor(&stubExecutor{}),
		runtime.WithDecomposition(&stubDecomposer{}),
	)
	return w.orc.Start()
}

func (w *promptLoopWorld) runLoop(text string) error {
	if err := w.orc.Submit(context.Background(), text); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return w.orc.Shutdown(ctx)
}

func (w *promptLoopWorld) timeline() ([]state.Event, error) {
	store := session.NewStore(w.dir)
	return store.Recorder(w.orc.Session().ID).Load()
}

func (w *promptLoopWorld) contentEvents(table *godog.Table) error {
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

func (w *promptLoopWorld) finalState(want string) error {
	if got := string(w.orc.Processing().Current().State); got != want {
		return fmt.Errorf("final state = %q, want %q", got, want)
	}
	return nil
}

func (w *promptLoopWorld) answerIs(want string) error {
	events, err := w.timeline()
	if err != nil {
		return err
	}
	var answer string
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
		return fmt.Errorf("loop answer = %q, want %q", answer, want)
	}
	return nil
}

func (w *promptLoopWorld) timelineHasTaskPlan() error {
	events, err := w.timeline()
	if err != nil {
		return err
	}
	for _, ev := range events {
		if ev.ContentType == state.ContentTaskPlan {
			return nil
		}
	}
	return fmt.Errorf("no task_plan event recorded")
}
