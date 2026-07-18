package sessionsteps

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/prompting"
	"agentx/internal/session"
)

type wmWorld struct {
	dir    string
	id     string
	wm     *session.WorkingMemory
	msg    prompting.Message
	msgSet bool
}

// registerWorkingMemorySteps wires the working-memory steps.
func registerWorkingMemorySteps(sc *godog.ScenarioContext) {
	w := &wmWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "agentx-wm-")
		if err != nil {
			return ctx, err
		}
		w.dir = dir
		w.id = "sess"
		w.wm = &session.WorkingMemory{}
		w.msg = prompting.Message{}
		w.msgSet = false
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		w.dir = ""
		return ctx, nil
	})

	sc.Step(`^a fresh session with no working memory file$`, w.freshSession)
	sc.Step(`^a working memory with a disabled "([^"]*)" fact valued "([^"]*)"$`, w.disabledFact)
	sc.Step(`^a working memory with an enabled "([^"]*)" fact valued "([^"]*)"$`, w.enabledFact)
	sc.Step(`^a disabled "([^"]*)" fact valued "([^"]*)"$`, w.disabledFact)
	sc.Step(`^bootstrap facts are seeded$`, w.seedBootstrap)
	sc.Step(`^the working memory is loaded$`, w.loadWM)
	sc.Step(`^the working memory is saved and reloaded$`, w.saveAndReload)
	sc.Step(`^the working memory context message is built$`, w.buildMessage)
	sc.Step(`^the working memory has a "([^"]*)" fact owned by "([^"]*)" and enabled$`, w.factOwnedEnabled)
	sc.Step(`^the working memory has a pin-owned "([^"]*)" fact that is (live|static) and (enabled|disabled)$`, w.pinFactState)
	sc.Step(`^the "([^"]*)" fact is still valued "([^"]*)" and disabled$`, w.factValuedDisabled)
	sc.Step(`^the working memory has (\d+) facts$`, w.factCount)
	sc.Step(`^the context message includes "([^"]*)"$`, w.messageIncludes)
	sc.Step(`^the context message excludes "([^"]*)"$`, w.messageExcludes)
	sc.Step(`^no context message is produced$`, w.noMessage)
}

func (w *wmWorld) store() *session.Store { return session.NewStore(w.dir) }

func (w *wmWorld) freshSession() error {
	w.wm = &session.WorkingMemory{}
	return nil
}

func (w *wmWorld) disabledFact(key, value string) error {
	w.wm.Facts = append(w.wm.Facts, session.Fact{
		Key: key, Value: value, Owner: session.OwnerUser, Enabled: false,
	})
	return nil
}

func (w *wmWorld) enabledFact(key, value string) error {
	w.wm.Facts = append(w.wm.Facts, session.Fact{
		Key: key, Value: value, Owner: session.OwnerUser, Enabled: true,
	})
	return nil
}

func (w *wmWorld) seedBootstrap() error {
	w.wm.SeedIfAbsent(session.BootstrapFacts()...)
	return nil
}

func (w *wmWorld) loadWM() error {
	wm, err := w.store().LoadWorkingMemory(w.id)
	if err != nil {
		return err
	}
	w.wm = wm
	return nil
}

func (w *wmWorld) saveAndReload() error {
	if err := w.store().SaveWorkingMemory(w.id, w.wm); err != nil {
		return err
	}
	return w.loadWM()
}

func (w *wmWorld) buildMessage() error {
	facts := make([]prompting.Fact, 0, len(w.wm.Facts))
	for _, f := range w.wm.Enabled() {
		facts = append(facts, prompting.Fact{Key: f.Key, Value: f.Value})
	}
	w.msg, w.msgSet = prompting.WorkingMemoryMessage(facts)
	return nil
}

func (w *wmWorld) factOwnedEnabled(key, owner string) error {
	f, ok := w.wm.Get(key)
	if !ok {
		return fmt.Errorf("fact %q not present", key)
	}
	if string(f.Owner) != owner {
		return fmt.Errorf("fact %q owner = %q, want %q", key, f.Owner, owner)
	}
	if !f.Enabled {
		return fmt.Errorf("fact %q is not enabled", key)
	}
	return nil
}

func (w *wmWorld) pinFactState(key, liveWord, enabledWord string) error {
	f, ok := w.wm.Get(key)
	if !ok {
		return fmt.Errorf("fact %q not present", key)
	}
	if f.Owner != session.OwnerPin {
		return fmt.Errorf("fact %q owner = %q, want %q", key, f.Owner, session.OwnerPin)
	}
	if f.Source == nil {
		return fmt.Errorf("fact %q has no tool source", key)
	}
	wantLive := liveWord == "live"
	if f.Live != wantLive {
		return fmt.Errorf("fact %q live = %v, want %v", key, f.Live, wantLive)
	}
	wantEnabled := enabledWord == "enabled"
	if f.Enabled != wantEnabled {
		return fmt.Errorf("fact %q enabled = %v, want %v", key, f.Enabled, wantEnabled)
	}
	return nil
}

func (w *wmWorld) factValuedDisabled(key, value string) error {
	f, ok := w.wm.Get(key)
	if !ok {
		return fmt.Errorf("fact %q not present", key)
	}
	if f.Value != value {
		return fmt.Errorf("fact %q value = %q, want %q", key, f.Value, value)
	}
	if f.Enabled {
		return fmt.Errorf("fact %q is enabled, want disabled", key)
	}
	return nil
}

func (w *wmWorld) factCount(n int) error {
	if len(w.wm.Facts) != n {
		return fmt.Errorf("fact count = %d, want %d", len(w.wm.Facts), n)
	}
	return nil
}

func (w *wmWorld) messageIncludes(sub string) error {
	if !w.msgSet {
		return fmt.Errorf("no context message was produced")
	}
	if !strings.Contains(w.msg.Content, sub) {
		return fmt.Errorf("context message %q does not contain %q", w.msg.Content, sub)
	}
	return nil
}

func (w *wmWorld) messageExcludes(sub string) error {
	if w.msgSet && strings.Contains(w.msg.Content, sub) {
		return fmt.Errorf("context message %q unexpectedly contains %q", w.msg.Content, sub)
	}
	return nil
}

func (w *wmWorld) noMessage() error {
	if w.msgSet {
		return fmt.Errorf("expected no context message, got %q", w.msg.Content)
	}
	return nil
}
