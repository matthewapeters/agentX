// Package surfacesteps adds Godog step definitions for the config surface
// Phase 3b: external file change detection, diff highlighting, and reload
// prompt (AF-008).
package surfacesteps

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cucumber/godog"

	"agentx/internal/llm/provider"
	"agentx/internal/surfaces/config"
)

// configExternalChangeWorld holds state for Phase 3b config surface steps.
type configExternalChangeWorld struct {
	model           *config.ConfigModel
	simulatedConfig map[string]any
	oldHash         string
}

// registerConfigExternalChangeSteps wires the Phase 3b config surface steps.
// It returns the world so Phase 3c (config_conflict_resolution_steps.go) can
// share the same config surface model instance rather than standing up its
// own, which "Given a config surface loaded with initial config" never
// populates.
func registerConfigExternalChangeSteps(sc *godog.ScenarioContext) *configExternalChangeWorld {
	w := &configExternalChangeWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w.model = nil
		w.simulatedConfig = nil
		w.oldHash = ""
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.model = nil
		w.simulatedConfig = nil
		w.oldHash = ""
		return ctx, nil
	})

	// --- Setup ---
	sc.Step(`^a config surface loaded with initial config$`, w.setupSurface)

	// --- Actions ---
	sc.Step(`^an external editor modifies the config file "([^"]*)"$`, w.externalEdit)
	sc.Step(`^the config surface detects an external change$`, w.detectChange)
	sc.Step(`^the config surface highlights changed keys$`, w.highlightChanged)
	sc.Step(`^the config surface shows the reload prompt dialog$`, w.showDialog)
	sc.Step(`^the user presses "r" to reload from file$`, w.reloadFromR)
	sc.Step(`^the user selects "Keep changes" and confirms$`, w.keepChanges)
	sc.Step(`^the user selects "Discard changes" and confirms$`, w.discardChanges)
	sc.Step(`^the user presses escape to dismiss the external change dialog$`, w.dismissDialog)
	sc.Step(`^a second external change is detected while the first is pending$`, w.debounceSecondChange)

	// --- Assertions ---
	sc.Step(`^the config view contains the external change dialog$`, w.viewContainsDialog)
	sc.Step(`^the config view does not contain the external change dialog$`, w.viewNotContainsDialog)
	sc.Step(`^the status bar shows "([^"]*)"$`, w.statusBarShows)
	sc.Step(`^the hint row mentions "([^"]*)"$`, w.hintRowMentions)
	sc.Step(`^no double external change dialog was created$`, w.noDoubleDialog)

	return w
}

// setupSurface creates a config surface pre-loaded with initial config. It
// wires a fetch override so handleExternalConfigChange's real re-fetch path
// (m.FetchConfig, used by config_changed events applied via w.model.Apply)
// succeeds against w.simulatedConfig instead of requiring a live transport
// client — otherwise every non-conflict external-change event would fail at
// the fetch step with "no transport client".
func (w *configExternalChangeWorld) setupSurface() error {
	cfg, schema := seedConfig()
	w.simulatedConfig = cfg
	w.model = config.NewFromConfig(cfg, schema, 80, 24, nil, "")
	w.model.SetTestFetchOverride(func() (map[string]any, map[string]provider.SchemaField, error) {
		return w.simulatedConfig, schema, nil
	})
	return nil
}

// externalEdit simulates an external file modification by changing the
// simulated config (what the orchestrator would return on fetch).
func (w *configExternalChangeWorld) externalEdit(path string) error {
	// Store old hash for assertion.
	w.oldHash = config.ConfigHashForTest(w.simulatedConfig)

	if w.simulatedConfig == nil {
		return fmt.Errorf("no simulated config")
	}

	// Modify the simulated config to represent the external edit.
	if agentx, ok := w.simulatedConfig["agentx"].(map[string]any); ok {
		if ollama, ok := agentx["ollama"].(map[string]any); ok {
			if _, ok := ollama["host"]; ok {
				ollama["host"] = "localhost:11435"
			}
		}
	}
	return nil
}

// detectChange triggers a simulated config_changed event on the surface.
// Since the godog world has no transport client, we use the model's test helper
// to simulate the external change detection flow (diff + dialog).
func (w *configExternalChangeWorld) detectChange() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.simulatedConfig == nil {
		return fmt.Errorf("no simulated config")
	}
	return w.model.SimulateExternalChange(w.simulatedConfig, "/tmp/agentx.toml")
}

// highlightChanged checks that the model's highlighted keys are populated.
func (w *configExternalChangeWorld) highlightChanged() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.HighlightedKeys == nil || len(w.model.Data.HighlightedKeys) == 0 {
		return fmt.Errorf("no highlighted keys found")
	}
	return nil
}

// showDialog verifies the dialog is shown.
func (w *configExternalChangeWorld) showDialog() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.Dialog == nil {
		return fmt.Errorf("no dialog shown")
	}
	view := w.model.View()
	if !strings.Contains(view, "File changed externally") {
		return fmt.Errorf("dialog not visible in view: %s", view)
	}
	return nil
}

// reloadFromR simulates pressing 'r' to reload.
func (w *configExternalChangeWorld) reloadFromR() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	w.model.Key(testKey("r"))
	return nil
}

// keepChanges simulates selecting "Keep changes".
func (w *configExternalChangeWorld) keepChanges() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.Dialog.Selected != 1 {
		w.model.Data.Dialog.Selected = 1
	}
	w.model.Key(testKey("enter"))
	return nil
}

// discardChanges simulates selecting "Discard changes".
func (w *configExternalChangeWorld) discardChanges() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.Dialog.Selected != 2 {
		w.model.Data.Dialog.Selected = 2
	}
	w.model.Key(testKey("enter"))
	return nil
}

// dismissDialog simulates pressing escape to dismiss.
func (w *configExternalChangeWorld) dismissDialog() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	w.model.Key(testKey("escape"))
	return nil
}

// debounceSecondChange simulates a second config_changed event while the first
// is still pending (dialog shown). Goes through SimulateExternalChange (same
// as the "detects an external change" step) rather than a raw event, since
// SimulateExternalChange has no epoch of its own to be "recent" relative to —
// its debounce guard instead coalesces based on whether an external-change
// dialog is already pending.
func (w *configExternalChangeWorld) debounceSecondChange() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.simulatedConfig == nil {
		return fmt.Errorf("no simulated config")
	}
	return w.model.SimulateExternalChange(w.simulatedConfig, "/tmp/agentx.toml")
}

// viewContainsDialog checks for the external change dialog in the view.
func (w *configExternalChangeWorld) viewContainsDialog() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	view := w.model.View()
	if !strings.Contains(view, "File changed externally") {
		return fmt.Errorf("view does not contain external change dialog (got: %s)", view)
	}
	return nil
}

// viewNotContainsDialog checks that the dialog is not in the view.
func (w *configExternalChangeWorld) viewNotContainsDialog() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	view := w.model.View()
	if strings.Contains(view, "File changed externally") {
		return fmt.Errorf("view unexpectedly contains external change dialog: %s", view)
	}
	return nil
}

// statusBarShows checks the status bar message.
func (w *configExternalChangeWorld) statusBarShows(msg string) error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if !strings.Contains(w.model.Data.SaveMsg, msg) {
		return fmt.Errorf("status bar does not contain %q (got: %q)", msg, w.model.Data.SaveMsg)
	}
	return nil
}

// hintRowMentions checks the hint row for a specific keybinding mention.
func (w *configExternalChangeWorld) hintRowMentions(text string) error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	view := w.model.View()
	if !strings.Contains(view, text) {
		return fmt.Errorf("view does not mention %q (got: %s)", text, view)
	}
	return nil
}

// noDoubleDialog checks that only one external change dialog exists
// (debounce prevents a second one from being created).
func (w *configExternalChangeWorld) noDoubleDialog() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.ExternalChange == nil {
		return fmt.Errorf("expected external change to persist after debounce")
	}
	// w.oldHash is only populated by the Phase 3b "an external editor
	// modifies the config file" step (config_external_change.feature). The
	// epoch-driven Phase 3c debounce scenarios (config_conflict_resolution.feature)
	// exercise handleExternalConfigChange directly via config_changed events
	// and never call externalEdit, so there is no pre-edit hash to compare
	// against — the ExternalChange-is-non-nil check above is sufficient there.
	if w.oldHash != "" && w.model.Data.ExternalChange.OldHash != w.oldHash {
		return fmt.Errorf("debounce failed: hash changed from %s to %s", w.oldHash, w.model.Data.ExternalChange.OldHash)
	}
	return nil
}

// testKey creates a KeyPressMsg from a key name.
func testKey(name string) tea.KeyPressMsg {
	switch name {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"}
	case "escape", "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape, Text: "esc"}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace, Text: "backspace"}
	}
	if len(name) == 1 {
		r := rune(name[0])
		return tea.KeyPressMsg{Code: r, Text: name}
	}
	r := []rune(name)[0]
	return tea.KeyPressMsg{Code: r, Text: name}
}
