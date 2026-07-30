// Package surfacesteps adds Godog step definitions for the config surface
// Phase 3c: conflict resolution and edge cases (two-way sync).
package surfacesteps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/state"
	"agentx/internal/surfaces/config"
)

// configConflictWorld holds state for Phase 3c config conflict resolution steps.
type configConflictWorld struct {
	model        *config.ConfigModel
	simulatedCfg map[string]any
	prevEpoch    int64
}

// registerConfigConflictResolutionSteps wires the Phase 3c config conflict
// resolution steps.
func registerConfigConflictResolutionSteps(sc *godog.ScenarioContext) {
	w := &configConflictWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w.model = nil
		w.simulatedCfg = nil
		w.prevEpoch = 0
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.model = nil
		w.simulatedCfg = nil
		w.prevEpoch = 0
		return ctx, nil
	})

	// --- Setup ---
	sc.Step(`^a config surface loaded with initial config$`, w.setupSurface)
	sc.Step(`^the config surface has unsaved TUI changes$`, w.markUnsaved)

	// --- Actions ---
	sc.Step(`^an external editor modifies the config file "([^"]*)"$`, w.externalEdit)
	sc.Step(`^a previous external change was detected at epoch (\d+)$`, w.previousExternalChange)
	sc.Step(`^an external change is detected at epoch (\d+)$`, w.externalChangeDetected)
	sc.Step(`^a second external change is detected at epoch (\d+)$`, w.secondExternalChange)
	sc.Step(`^the config surface is saving$`, w.markSaving)
	sc.Step(`^an external change is detected at epoch (\d+) while saving$`, w.externalChangeDuringSave)
	sc.Step(`^the pending external event is processed$`, w.processPending)

	// --- Assertions ---
	sc.Step(`^the config surface shows the conflict resolution dialog$`, w.conflictDialogShown)
	sc.Step(`^the conflict resolution dialog has "([^"]*)" selected$`, w.conflictDialogOptionSelected)
	sc.Step(`^the config surface shows the external change dialog \(not conflict resolution\)$`, w.normalDialogShown)
	sc.Step(`^the external change dialog has "([^"]*)" selected$`, w.externalDialogOptionSelected)
	sc.Step(`^the config view does not contain the conflict resolution dialog$`, w.conflictDialogDismissed)
	sc.Step(`^no double external change dialog was created$`, w.noDoubleDialog)
	sc.Step(`^the last external event epoch is (\d+)$`, w.lastEventEpochIs)
	sc.Step(`^the external change is queued$`, w.externalChangeQueued)
	sc.Step(`^the pending external event is cleared$`, w.pendingEventCleared)
	sc.Step(`^the status bar shows "([^"]*)"$`, w.statusBarShows)
	sc.Step(`^the hint row mentions "([^"]*)"$`, w.hintRowMentions)
}

// setupSurface creates a config surface pre-loaded with initial config.
func (w *configConflictWorld) setupSurface() error {
	cfg, schema := seedConfig()
	w.simulatedCfg = cfg
	w.model = config.NewFromConfig(cfg, schema, 80, 24, nil, "")
	return nil
}

// markUnsaved puts the TUI in unsaved state.
func (w *configConflictWorld) markUnsaved() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	w.model.Data.SaveStatus = config.SaveStateUnsaved
	w.model.Data.UnsavedChanges = true
	return nil
}

// externalEdit simulates an external file modification by changing the
// simulated config.
func (w *configConflictWorld) externalEdit(path string) error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.simulatedCfg == nil {
		return fmt.Errorf("no simulated config")
	}
	if agentx, ok := w.simulatedCfg["agentx"].(map[string]any); ok {
		if ollama, ok := agentx["ollama"].(map[string]any); ok {
			if _, ok := ollama["host"]; ok {
				ollama["host"] = "localhost:11435"
			}
		}
	}
	// Use SimulateExternalChange to trigger the full diff+dialog flow.
	return w.model.SimulateExternalChange(w.simulatedCfg, path)
}

// previousExternalChange sets the LastExternalEventAt timestamp to simulate a
// previously processed event (for debounce testing).
func (w *configConflictWorld) previousExternalChange(epoch int64) error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	w.model.Data.LastExternalEventAt = epoch
	w.prevEpoch = epoch
	return nil
}

// externalChangeDetected sends a config_changed event at the given epoch.
func (w *configConflictWorld) externalChangeDetected(epoch int64) error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	ev := state.Event{
		Epoch:       epoch,
		SessionID:   "test-session",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}
	w.model.Apply(ev)
	return nil
}

// secondExternalChange sends a second config_changed event at the given epoch.
func (w *configConflictWorld) secondExternalChange(epoch int64) error {
	return w.externalChangeDetected(epoch)
}

// markSaving puts the model in saving state.
func (w *configConflictWorld) markSaving() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	w.model.Data.SaveStatus = config.SaveStateSaving
	return nil
}

// externalChangeDuringSave sends a config_changed event while the model is
// in saving state, which should queue the event.
func (w *configConflictWorld) externalChangeDuringSave(epoch int64) error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	ev := state.Event{
		Epoch:       epoch,
		SessionID:   "test-session",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}
	w.model.Apply(ev)
	return nil
}

// processPending triggers processPendingExternalEvent.
func (w *configConflictWorld) processPending() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	w.model.ProcessPendingExternalEvent()
	return nil
}

// conflictDialogShown checks that the conflict resolution dialog is shown.
func (w *configConflictWorld) conflictDialogShown() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.Dialog == nil {
		return fmt.Errorf("no dialog shown")
	}
	view := w.model.View()
	if !strings.Contains(view, "TUI changes take precedence") {
		return fmt.Errorf("conflict resolution dialog not visible in view: %s", view)
	}
	return nil
}

// conflictDialogOptionSelected checks that the specified option is selected.
func (w *configConflictWorld) conflictDialogOptionSelected(option string) error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.Dialog == nil {
		return fmt.Errorf("no dialog shown")
	}
	if w.model.Data.Dialog.Selected != 0 {
		return fmt.Errorf("expected selected option 0, got %d", w.model.Data.Dialog.Selected)
	}
	found := false
	for _, opt := range w.model.Data.Dialog.Options {
		if opt == option {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("option %q not found in dialog options: %v", option, w.model.Data.Dialog.Options)
	}
	return nil
}

// normalDialogShown checks that the standard external change dialog is shown
// (not the conflict resolution dialog).
func (w *configConflictWorld) normalDialogShown() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.Dialog == nil {
		return fmt.Errorf("no dialog shown")
	}
	view := w.model.View()
	if !strings.Contains(view, "File changed externally") {
		return fmt.Errorf("external change dialog not visible in view: %s", view)
	}
	if strings.Contains(view, "TUI changes take precedence") {
		return fmt.Errorf("conflict resolution dialog should not be shown")
	}
	return nil
}

// externalDialogOptionSelected checks that the specified option is selected in
// the standard external change dialog.
func (w *configConflictWorld) externalDialogOptionSelected(option string) error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.Dialog == nil {
		return fmt.Errorf("no dialog shown")
	}
	found := false
	for _, opt := range w.model.Data.Dialog.Options {
		if opt == option {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("option %q not found in dialog options: %v", option, w.model.Data.Dialog.Options)
	}
	return nil
}

// conflictDialogDismissed checks that the conflict resolution dialog is gone.
func (w *configConflictWorld) conflictDialogDismissed() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	view := w.model.View()
	if strings.Contains(view, "TUI changes take precedence") {
		return fmt.Errorf("conflict resolution dialog still visible: %s", view)
	}
	return nil
}

// noDoubleDialog checks that only one external change dialog exists.
func (w *configConflictWorld) noDoubleDialog() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.ExternalChange == nil {
		return fmt.Errorf("expected ExternalChange to persist after debounce")
	}
	return nil
}

// lastEventEpochIs checks the LastExternalEventAt timestamp.
func (w *configConflictWorld) lastEventEpochIs(want int64) error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.LastExternalEventAt != want {
		return fmt.Errorf("LastExternalEventAt = %d, want %d", w.model.Data.LastExternalEventAt, want)
	}
	return nil
}

// externalChangeQueued checks that the event is queued.
func (w *configConflictWorld) externalChangeQueued() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.PendingExternalEvent == nil {
		return fmt.Errorf("expected PendingExternalEvent to be set during save")
	}
	return nil
}

// pendingEventCleared checks that the queued event is gone.
func (w *configConflictWorld) pendingEventCleared() error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if w.model.Data.PendingExternalEvent != nil {
		return fmt.Errorf("expected PendingExternalEvent to be nil after processing, got: %+v", w.model.Data.PendingExternalEvent)
	}
	return nil
}

// statusBarShows checks the status bar message.
func (w *configConflictWorld) statusBarShows(msg string) error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	if !strings.Contains(w.model.Data.SaveMsg, msg) {
		return fmt.Errorf("status bar does not contain %q (got: %q)", msg, w.model.Data.SaveMsg)
	}
	return nil
}

// hintRowMentions checks the hint row for a specific keybinding mention.
func (w *configConflictWorld) hintRowMentions(text string) error {
	if w.model == nil {
		return fmt.Errorf("no config surface")
	}
	view := w.model.View()
	if !strings.Contains(view, text) {
		return fmt.Errorf("view does not mention %q (got: %s)", text, view)
	}
	return nil
}


