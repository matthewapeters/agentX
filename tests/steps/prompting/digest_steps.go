package promptingsteps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/prompting/digest"
	"agentx/internal/state"
)

type digestWorld struct {
	events []state.Event
	next   uint64
	dig    digest.Digest
}

func registerDigestSteps(sc *godog.ScenarioContext) {
	w := &digestWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		*w = digestWorld{}
		return ctx, err
	})

	sc.Step(`^a user turn "([^"]*)"$`, w.userTurn)
	sc.Step(`^an agent turn "([^"]*)"$`, w.agentTurn)
	sc.Step(`^a disabled agent turn "([^"]*)"$`, w.disabledAgentTurn)
	sc.Step(`^a thinking event$`, w.thinkingEvent)
	sc.Step(`^a tool_call event$`, w.toolCallEvent)
	sc.Step(`^the digest is built keeping (\d+) turns$`, w.buildDigest)
	sc.Step(`^the digest has (\d+) turns$`, w.digestTurns)
	sc.Step(`^the digest turn count is (\d+)$`, w.digestCount)
	sc.Step(`^the digest render contains "([^"]*)"$`, w.renderContains)
	sc.Step(`^the digest render omits "([^"]*)"$`, w.renderOmits)
	sc.Step(`^the digest render is empty$`, w.renderEmpty)
	sc.Step(`^the digest cursor is (\d+)$`, w.digestCursor)
}

func (w *digestWorld) add(ct state.ContentType, text string, enabled bool) {
	w.next++
	w.events = append(w.events, state.Event{
		ContentType: ct,
		Payload:     map[string]any{"text": text},
		Enabled:     enabled,
		Ordinal:     w.next,
	})
}

func (w *digestWorld) userTurn(text string) error {
	w.add(state.ContentUserPrompt, text, true)
	return nil
}

func (w *digestWorld) agentTurn(text string) error {
	w.add(state.ContentAgentResponse, text, true)
	return nil
}

func (w *digestWorld) disabledAgentTurn(text string) error {
	w.add(state.ContentAgentResponse, text, false)
	return nil
}

func (w *digestWorld) thinkingEvent() error {
	w.add(state.ContentThinking, "reasoning...", true)
	return nil
}

func (w *digestWorld) toolCallEvent() error {
	w.add(state.ContentToolCall, "ls -la", true)
	return nil
}

func (w *digestWorld) buildDigest(maxTurns int) error {
	w.dig = digest.Build(w.events, maxTurns)
	return nil
}

func (w *digestWorld) digestTurns(n int) error {
	if len(w.dig.RecentTurns) != n {
		return fmt.Errorf("digest has %d turns, want %d", len(w.dig.RecentTurns), n)
	}
	return nil
}

func (w *digestWorld) digestCount(n int) error {
	if w.dig.TurnCount != n {
		return fmt.Errorf("digest turn count = %d, want %d", w.dig.TurnCount, n)
	}
	return nil
}

func (w *digestWorld) renderContains(want string) error {
	if !strings.Contains(w.dig.Render(), want) {
		return fmt.Errorf("render %q does not contain %q", w.dig.Render(), want)
	}
	return nil
}

func (w *digestWorld) renderOmits(unwanted string) error {
	if strings.Contains(w.dig.Render(), unwanted) {
		return fmt.Errorf("render %q should omit %q", w.dig.Render(), unwanted)
	}
	return nil
}

func (w *digestWorld) renderEmpty() error {
	if r := w.dig.Render(); r != "" {
		return fmt.Errorf("render should be empty, got %q", r)
	}
	return nil
}

func (w *digestWorld) digestCursor(n int) error {
	if w.dig.Cursor != uint64(n) {
		return fmt.Errorf("digest cursor = %d, want %d", w.dig.Cursor, n)
	}
	return nil
}
