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

// promptFilesWorld covers the user prompt-file stories (instructions prefix and
// bootstrap prompt). It uses a capturing stub model to inspect the assembled
// context handed to the LLM.
type promptFilesWorld struct {
	dir      string
	orc      *runtime.Orchestrator
	captured []prompting.Message
}

// registerPromptFilesSteps wires the instructions/bootstrap prompt steps.
func registerPromptFilesSteps(sc *godog.ScenarioContext) {
	w := &promptFilesWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = promptFilesWorld{}
		return ctx, err
	})

	sc.Step(`^a started orchestrator with instructions "([^"]*)" and a capturing model$`, w.startInstructions)
	sc.Step(`^a started orchestrator with instructions "([^"]*)" and bootstrap prompt "([^"]*)" and a capturing model that replies "([^"]*)"$`, w.startBootstrap)
	sc.Step(`^a started orchestrator with no bootstrap prompt and a capturing model that replies "([^"]*)"$`, w.startNoBootstrap)
	sc.Step(`^the user submits the prompt "([^"]*)"$`, w.submit)
	sc.Step(`^the orchestrator runs its bootstrap prompt$`, w.runBootstrap)
	sc.Step(`^the bootstrap session is flushed to disk$`, w.flush)
	sc.Step(`^the model received a system message "([^"]*)"$`, w.systemMessage)
	sc.Step(`^the model received a user message "([^"]*)"$`, w.userMessage)
	sc.Step(`^the system message precedes the user message$`, w.systemPrecedesUser)
	sc.Step(`^an agent response is recorded$`, w.agentResponseRecorded)
	sc.Step(`^no agent response is recorded$`, w.noAgentResponseRecorded)
	sc.Step(`^no user prompt is recorded$`, w.noUserPromptRecorded)
}

func (w *promptFilesWorld) start(s runtime.Settings, model runtime.Model) error {
	dir, err := os.MkdirTemp("", "agentx-prompts-")
	if err != nil {
		return err
	}
	w.dir = dir
	s.SessionRoot = dir
	s.OllamaModel = "stub"
	w.orc = runtime.New(s, runtime.WithModel(model))
	return w.orc.Start()
}

func (w *promptFilesWorld) capturingModel(deltas ...string) stubModel {
	return stubModel{deltas: deltas, captured: &w.captured}
}

func (w *promptFilesWorld) startInstructions(instructions string) error {
	return w.start(runtime.Settings{Instructions: instructions}, w.capturingModel())
}

func (w *promptFilesWorld) startBootstrap(instructions, bootstrap, reply string) error {
	return w.start(
		runtime.Settings{Instructions: instructions, BootstrapPrompt: bootstrap},
		w.capturingModel(reply),
	)
}

func (w *promptFilesWorld) startNoBootstrap(reply string) error {
	return w.start(runtime.Settings{}, w.capturingModel(reply))
}

func (w *promptFilesWorld) submit(text string) error {
	return w.orc.Submit(context.Background(), text)
}

func (w *promptFilesWorld) runBootstrap() error {
	return w.orc.SubmitBootstrap(context.Background())
}

func (w *promptFilesWorld) flush() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return w.orc.Shutdown(ctx)
}

func (w *promptFilesWorld) findRole(role string) (int, prompting.Message, bool) {
	for i, m := range w.captured {
		if m.Role == role {
			return i, m, true
		}
	}
	return -1, prompting.Message{}, false
}

func (w *promptFilesWorld) systemMessage(want string) error {
	_, m, ok := w.findRole("system")
	if !ok {
		return fmt.Errorf("no system message was sent to the model")
	}
	if m.Content != want {
		return fmt.Errorf("system message = %q, want %q", m.Content, want)
	}
	return nil
}

func (w *promptFilesWorld) userMessage(want string) error {
	_, m, ok := w.findRole("user")
	if !ok {
		return fmt.Errorf("no user message was sent to the model")
	}
	if m.Content != want {
		return fmt.Errorf("user message = %q, want %q", m.Content, want)
	}
	return nil
}

func (w *promptFilesWorld) systemPrecedesUser() error {
	si, _, sok := w.findRole("system")
	ui, _, uok := w.findRole("user")
	if !sok || !uok {
		return fmt.Errorf("expected both system and user messages")
	}
	if si >= ui {
		return fmt.Errorf("system message at %d does not precede user message at %d", si, ui)
	}
	return nil
}

func (w *promptFilesWorld) events() ([]state.Event, error) {
	store := session.NewStore(w.dir)
	return store.Recorder(w.orc.Session().ID).Load()
}

func (w *promptFilesWorld) countContent(ct state.ContentType) (int, error) {
	events, err := w.events()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, ev := range events {
		if ev.ContentType == ct {
			n++
		}
	}
	return n, nil
}

func (w *promptFilesWorld) agentResponseRecorded() error {
	n, err := w.countContent(state.ContentAgentResponse)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no agent_response event recorded")
	}
	return nil
}

func (w *promptFilesWorld) noAgentResponseRecorded() error {
	n, err := w.countContent(state.ContentAgentResponse)
	if err != nil {
		return err
	}
	if n != 0 {
		return fmt.Errorf("expected no agent_response events, found %d", n)
	}
	return nil
}

func (w *promptFilesWorld) noUserPromptRecorded() error {
	n, err := w.countContent(state.ContentUserPrompt)
	if err != nil {
		return err
	}
	if n != 0 {
		return fmt.Errorf("expected no user_prompt events, found %d", n)
	}
	return nil
}
