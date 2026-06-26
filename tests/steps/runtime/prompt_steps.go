package runtimesteps

import (
	"fmt"

	"github.com/cucumber/godog"

	"agentx/internal/prompting"
)

type promptWorld struct {
	assembler *prompting.Assembler
	messages  []prompting.Message
}

// registerPromptSteps wires the prompt-assembly steps (CHT-C2).
func registerPromptSteps(sc *godog.ScenarioContext) {
	w := &promptWorld{}

	sc.Step(`^a prompt assembler with system prompt "([^"]*)"$`, w.assemblerWithSystem)
	sc.Step(`^a prompt assembler with no system prompt$`, w.assemblerNoSystem)
	sc.Step(`^a prompt is assembled for "([^"]*)"$`, w.assemble)
	sc.Step(`^there (?:are|is) (\d+) messages?$`, w.messageCount)
	sc.Step(`^message (\d+) is a "([^"]*)" message with "([^"]*)"$`, w.messageIs)
}

func (w *promptWorld) assemblerWithSystem(system string) error {
	w.assembler = prompting.New(system)
	return nil
}

func (w *promptWorld) assemblerNoSystem() error {
	w.assembler = prompting.New("")
	return nil
}

func (w *promptWorld) assemble(userText string) error {
	w.messages = w.assembler.Assemble(userText)
	return nil
}

func (w *promptWorld) messageCount(want int) error {
	if len(w.messages) != want {
		return fmt.Errorf("message count = %d, want %d", len(w.messages), want)
	}
	return nil
}

func (w *promptWorld) messageIs(index int, role, content string) error {
	i := index - 1
	if i < 0 || i >= len(w.messages) {
		return fmt.Errorf("message %d out of range (have %d)", index, len(w.messages))
	}
	m := w.messages[i]
	if m.Role != role || m.Content != content {
		return fmt.Errorf("message %d = {%q, %q}, want {%q, %q}", index, m.Role, m.Content, role, content)
	}
	return nil
}
