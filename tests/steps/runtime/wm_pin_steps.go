package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/classify"
	"agentx/internal/prompting"
	"agentx/internal/runtime"
	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// counterRunner returns a distinct, incrementing result each call ("run 1", "run
// 2", ...), so a scenario can distinguish a frozen (static) pin from one that was
// actually re-run (live).
type counterRunner struct {
	mu    sync.Mutex
	count int
}

func (r *counterRunner) Run(context.Context, tools.Descriptor, map[string]string) (tools.Result, error) {
	r.mu.Lock()
	r.count++
	n := r.count
	r.mu.Unlock()
	return tools.Result{ToolID: "list_dir", Status: "ok", Exit: 0, Lines: 1, Preview: fmt.Sprintf("run %d", n)}, nil
}

// wmPinWorld drives the Pin-to-working-memory affordance (PD-CTX-AF-012 / PD-WM)
// end to end: it runs the single_tool cycle so a tool_result is published, pins
// it (static or live), and inspects the resulting working-memory fact, the
// source event's enabled state, and the context breakdown.
type wmPinWorld struct {
	dir          string
	orc          *runtime.Orchestrator
	pinnedKey    string
	pinnedResOrd uint64
	setLiveErr   error
}

func registerWMPinSteps(sc *godog.ScenarioContext) {
	w := &wmPinWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		// Every other runtime-domain step file shuts its orchestrator down here
		// (approval_steps.go, tool_cycle_steps.go, etc.) — this one didn't, leaking
		// each scenario's recorder goroutine and bus subscription (Orchestrator.Start,
		// orchestrator.go:292) for the rest of the test binary's life. Harmless in
		// isolation, but the accumulation across ~100+ scenarios in a full
		// @integration run was making later, unrelated scenarios intermittently slow
		// enough to trip go test's suite-wide timeout — this is what made
		// "Toggling a pin back to static stops refreshing it" flaky.
		if w.orc != nil {
			shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = w.orc.Shutdown(shutCtx)
			cancel()
		}
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = wmPinWorld{}
		return ctx, nil
	})

	sc.Step(`^an orchestrator that runs the "([^"]*)" counting tool and answers "([^"]*)"$`, w.start)
	sc.Step(`^an orchestrator with the "([^"]*)" tool blacklisted that runs it and answers "([^"]*)"$`, w.startBlacklisted)
	sc.Step(`^the prompt "([^"]*)" runs the wm-pin tool cycle$`, w.runToolCycle)
	sc.Step(`^the prompt "([^"]*)" is submitted for wm-pin$`, w.submit)
	sc.Step(`^the last tool result is pinned static to working memory$`, w.pinStatic)
	sc.Step(`^the last tool result is pinned live to working memory$`, w.pinLive)
	sc.Step(`^the pinned fact is set live$`, w.setLive)
	sc.Step(`^the pinned fact is set static$`, w.setStatic)
	sc.Step(`^setting the pinned fact live is refused$`, w.setLiveRefused)
	sc.Step(`^the pinned fact is deleted$`, w.deleteFact)
	sc.Step(`^working memory has a pin-owned fact valued "([^"]*)"$`, w.factValued)
	sc.Step(`^the pinned fact is valued "([^"]*)"$`, w.factValued)
	sc.Step(`^the pinned fact is still valued "([^"]*)"$`, w.factValued)
	sc.Step(`^the source tool result is disabled in context$`, w.sourceDisabled)
	sc.Step(`^the source tool result is still disabled in context$`, w.sourceDisabled)
	sc.Step(`^the wm-pin context breakdown includes class "([^"]*)"$`, w.breakdownIncludes)
	sc.Step(`^the wm-pin context breakdown omits class "([^"]*)"$`, w.breakdownOmits)
}

// classifierChat routes only the tool-running prompt ("list the project") to
// single_tool; every other prompt answers directly, so a follow-up turn's only
// source of tool re-execution is Orchestrator.refreshLiveFacts, isolating what a
// live pin actually controls.
func wmPinClassifierChat(_ context.Context, msgs []prompting.Message) (string, error) {
	if len(msgs) > 0 && msgs[len(msgs)-1].Content == "list the project" {
		return `{"route": "single_tool"}`, nil
	}
	return `{"route": "respond_directly"}`, nil
}

func (w *wmPinWorld) start(tool, reply string) error {
	return w.startWith(tool, reply, "")
}

// startBlacklisted denies tool unconditionally (an empty pattern rule), so the
// original cycle records a "denied" tool_result — still a real tool_result
// element, pinnable static, but its source tool is never policy-Allow, so going
// live must be refused.
func (w *wmPinWorld) startBlacklisted(tool, reply string) error {
	dir, err := os.MkdirTemp("", "agentx-wmpin-bl-")
	if err != nil {
		return err
	}
	blacklist := filepath.Join(dir, "blacklist.toml")
	content := fmt.Sprintf("[[rule]]\ntool = %q\nreason = \"test_blacklist\"\n", tool)
	if err := os.WriteFile(blacklist, []byte(content), 0o644); err != nil {
		return err
	}
	return w.startWith(tool, reply, blacklist)
}

func (w *wmPinWorld) startWith(tool, reply, blacklistPath string) error {
	dir, err := os.MkdirTemp("", "agentx-wmpin-")
	if err != nil {
		return err
	}
	w.dir = dir
	proposal := fmt.Sprintf(`{"tool": %q, "args": {"path": "."}}`, tool)
	proposer := tools.NewProposer("catalog", 0, func(context.Context, []prompting.Message) (string, error) {
		return proposal, nil
	})
	w.orc = runtime.New(
		runtime.Settings{
			SessionRoot: dir, OllamaModel: "stub", Instructions: "Be brief.",
			ToolsEnabled: true, ToolReadOnly: true, ToolBlacklistPath: blacklistPath,
		},
		runtime.WithModel(stubModel{deltas: []string{reply}}),
		runtime.WithClassifier(classify.New("", 0, wmPinClassifierChat)),
		runtime.WithProposer(proposer),
		runtime.WithToolRunner(&counterRunner{}),
	)
	if err := w.orc.Start(); err != nil {
		return err
	}
	// The live bootstrap pins ("date", "git_status"; session.BootstrapFacts) would
	// otherwise call the shared counterRunner on every refreshLiveFacts pass
	// before the scenario's own tool call, throwing off the exact "run N" counts
	// these scenarios assert. They exercise a different tool entirely (date/git
	// status, not counterRunner's tool), so dropping them here isolates the
	// counting semantics this suite is actually testing.
	_ = w.orc.DeleteFact("date")
	_ = w.orc.DeleteFact("git_status")
	return nil
}

func (w *wmPinWorld) runToolCycle(text string) error {
	return w.orc.Submit(context.Background(), text)
}

func (w *wmPinWorld) submit(text string) error {
	return w.orc.Submit(context.Background(), text)
}

// lastToolResultOrdinal polls the durable log (the recorder persists off the hot
// path) for the most recent tool_result event's ordinal.
func (w *wmPinWorld) lastToolResultOrdinal() (uint64, error) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		hist, err := w.orc.History()
		if err != nil {
			return 0, err
		}
		var ord uint64
		for _, ev := range hist {
			if ev.ContentType == state.ContentToolResult {
				ord = ev.Ordinal
			}
		}
		if ord != 0 {
			return ord, nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("no tool_result recorded")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (w *wmPinWorld) pin(live bool) error {
	ord, err := w.lastToolResultOrdinal()
	if err != nil {
		return err
	}
	w.pinnedResOrd = ord
	key, err := w.orc.PinToolEvent(ord, live)
	if err != nil {
		return err
	}
	w.pinnedKey = key
	return nil
}

func (w *wmPinWorld) pinStatic() error { return w.pin(false) }
func (w *wmPinWorld) pinLive() error   { return w.pin(true) }

func (w *wmPinWorld) setLive() error {
	err := w.orc.SetFactLive(w.pinnedKey, true)
	w.setLiveErr = err
	return err
}

func (w *wmPinWorld) setStatic() error {
	return w.orc.SetFactLive(w.pinnedKey, false)
}

func (w *wmPinWorld) setLiveRefused() error {
	if err := w.orc.SetFactLive(w.pinnedKey, true); err == nil {
		return fmt.Errorf("expected SetFactLive(true) to be refused, it succeeded")
	}
	return nil
}

func (w *wmPinWorld) deleteFact() error {
	return w.orc.DeleteFact(w.pinnedKey)
}

func (w *wmPinWorld) fact() (session.Fact, error) {
	facts, err := w.orc.WorkingMemory()
	if err != nil {
		return session.Fact{}, err
	}
	for _, f := range facts {
		if f.Key == w.pinnedKey {
			return f, nil
		}
	}
	return session.Fact{}, fmt.Errorf("no fact with key %q", w.pinnedKey)
}

func (w *wmPinWorld) factValued(want string) error {
	f, err := w.fact()
	if err != nil {
		return err
	}
	if f.Owner != session.OwnerPin {
		return fmt.Errorf("fact %q owner = %q, want pin", f.Key, f.Owner)
	}
	if !strings.Contains(f.Value, want) {
		return fmt.Errorf("fact %q value = %q, want it to contain %q", f.Key, f.Value, want)
	}
	return nil
}

func (w *wmPinWorld) sourceDisabled() error {
	deadline := time.Now().Add(2 * time.Second)
	for {
		hist, err := w.orc.History()
		if err != nil {
			return err
		}
		for _, ev := range hist {
			if ev.Ordinal == w.pinnedResOrd {
				if ev.Enabled {
					return fmt.Errorf("source ordinal %d is still enabled in context", w.pinnedResOrd)
				}
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("source ordinal %d not found", w.pinnedResOrd)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (w *wmPinWorld) breakdownIncludes(class string) error {
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

func (w *wmPinWorld) breakdownOmits(class string) error {
	report, err := w.orc.ContextBreakdown()
	if err != nil {
		return err
	}
	for _, c := range report.Components {
		if c.Class == class {
			return fmt.Errorf("breakdown unexpectedly includes class %q", class)
		}
	}
	return nil
}
