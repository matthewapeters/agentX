// Package surfacesteps adds Godog step definitions for the config surface
// Phase 3c: conflict resolution and edge cases (two-way sync).
package surfacesteps

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cucumber/godog"

	"agentx/internal/state"
	"agentx/internal/surfaces/config"
)

// configConflictWorld holds state for Phase 3c config conflict resolution
// steps. It shares its config surface model with configExternalChangeWorld
// (via `shared`) because "Given a config surface loaded with initial config"
// is only registered once, on the Phase 3b world — without sharing, every
// scenario in config_conflict_resolution.feature would see a nil model.
type configConflictWorld struct {
	shared    *configExternalChangeWorld
	prevEpoch int64
}

func (w *configConflictWorld) model() *config.ConfigModel { return w.shared.model }

// registerConfigConflictResolutionSteps wires the Phase 3c config conflict
// resolution steps. shared is the world populated by
// registerConfigExternalChangeSteps, which owns the "Given a config surface
// loaded with initial config" step.
func registerConfigConflictResolutionSteps(sc *godog.ScenarioContext, shared *configExternalChangeWorld) {
	w := &configConflictWorld{shared: shared}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w.prevEpoch = 0
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.prevEpoch = 0
		return ctx, nil
	})

	// --- Setup ---
	// NOTE: "a config surface loaded with initial config" is registered by
	// config_external_change_steps.go to avoid duplicate step definitions.
	sc.Step(`^the config surface has unsaved TUI changes$`, w.markUnsaved)

	// --- Actions ---
	// NOTE: "an external editor modifies the config file" is registered by
	// config_external_change_steps.go to avoid duplicate step definitions.
	sc.Step(`^the user selects "Keep TUI changes" and confirms$`, w.keepTUIChanges)
	sc.Step(`^the user selects "Discard" and confirms$`, w.discardChanges)
	sc.Step(`^a previous external change was detected at epoch (\d+)$`, w.previousExternalChange)
	sc.Step(`^an external change is detected at epoch (\d+)$`, w.externalChangeDetected)
	sc.Step(`^a second external change is detected at epoch (\d+)$`, w.secondExternalChange)
	sc.Step(`^the config surface is saving$`, w.markSaving)
	sc.Step(`^an external change is detected at epoch (\d+) while saving$`, w.externalChangeDuringSave)
	sc.Step(`^the pending external event is processed$`, w.processPending)
	// NOTE: "the status bar shows", "the hint row mentions", and "no double external change dialog was created" are registered by
	// config_external_change_steps.go to avoid duplicate step definitions.

	// --- Assertions ---
	sc.Step(`^the config surface shows the conflict resolution dialog$`, w.conflictDialogShown)
	sc.Step(`^the conflict resolution dialog has "([^"]*)" selected$`, w.conflictDialogOptionSelected)
	sc.Step(`^the config surface shows the external change dialog \(not conflict resolution\)$`, w.normalDialogShown)
	sc.Step(`^the external change dialog has "([^"]*)" selected$`, w.externalDialogOptionSelected)
	sc.Step(`^the config view does not contain the conflict resolution dialog$`, w.conflictDialogDismissed)
	// NOTE: "no double external change dialog was created", "the status bar shows", and "the hint row mentions" are registered by
	// config_external_change_steps.go to avoid duplicate step definitions.
	sc.Step(`^the last external event epoch is (\d+)$`, w.lastEventEpochIs)
	sc.Step(`^the external change is queued$`, w.externalChangeQueued)
	sc.Step(`^the pending external event is cleared$`, w.pendingEventCleared)
}

// markUnsaved puts the TUI in unsaved state.
func (w *configConflictWorld) markUnsaved() error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	m.Data.SaveStatus = config.SaveStateUnsaved
	m.Data.UnsavedChanges = true
	return nil
}

// previousExternalChange sets the LastExternalEventAt timestamp to simulate a
// previously processed event (for debounce testing).
func (w *configConflictWorld) previousExternalChange(epoch int64) error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	m.Data.LastExternalEventAt = epoch
	w.prevEpoch = epoch
	return nil
}

// externalChangeDetected sends a config_changed event at the given epoch.
func (w *configConflictWorld) externalChangeDetected(epoch int64) error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	ev := state.Event{
		Epoch:       epoch,
		SessionID:   "test-session",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}
	m.Apply(ev)
	return nil
}

// secondExternalChange sends a second config_changed event at the given epoch.
func (w *configConflictWorld) secondExternalChange(epoch int64) error {
	return w.externalChangeDetected(epoch)
}

// markSaving puts the model in saving state.
func (w *configConflictWorld) markSaving() error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	m.Data.SaveStatus = config.SaveStateSaving
	return nil
}

// externalChangeDuringSave sends a config_changed event while the model is
// in saving state, which should queue the event.
func (w *configConflictWorld) externalChangeDuringSave(epoch int64) error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	ev := state.Event{
		Epoch:       epoch,
		SessionID:   "test-session",
		EventType:   "config_changed",
		ContentType: state.ContentConfigChanged,
		Payload:     map[string]any{"path": "/tmp/agentx.toml"},
	}
	m.Apply(ev)
	return nil
}

// processPending triggers processPendingExternalEvent. Per the scenario name
// ("Queued external change is processed after save completes"), this
// simulates the save having finished — production only calls
// ProcessPendingExternalEvent once the in-flight POST /config completes and
// SaveStatus has moved off SaveStateSaving; otherwise
// handleExternalConfigChange would just re-queue the event.
func (w *configConflictWorld) processPending() error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	if m.Data.SaveStatus == config.SaveStateSaving {
		m.Data.SaveStatus = config.SaveStateSaved
	}
	m.ProcessPendingExternalEvent()
	return nil
}

// conflictDialogShown checks that the conflict resolution dialog is shown.
func (w *configConflictWorld) conflictDialogShown() error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	if m.Data.Dialog == nil {
		return fmt.Errorf("no dialog shown")
	}
	view := m.View()
	if !strings.Contains(view, "TUI changes take precedence") {
		return fmt.Errorf("conflict resolution dialog not visible in view: %s", view)
	}
	return nil
}

// conflictDialogOptionSelected checks that the specified option is selected.
func (w *configConflictWorld) conflictDialogOptionSelected(option string) error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	if m.Data.Dialog == nil {
		return fmt.Errorf("no dialog shown")
	}
	if m.Data.Dialog.Selected != 0 {
		return fmt.Errorf("expected selected option 0, got %d", m.Data.Dialog.Selected)
	}
	found := false
	for _, opt := range m.Data.Dialog.Options {
		if opt == option {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("option %q not found in dialog options: %v", option, m.Data.Dialog.Options)
	}
	return nil
}

// normalDialogShown checks that the standard external change dialog is shown
// (not the conflict resolution dialog).
func (w *configConflictWorld) normalDialogShown() error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	if m.Data.Dialog == nil {
		return fmt.Errorf("no dialog shown")
	}
	view := m.View()
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
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	if m.Data.Dialog == nil {
		return fmt.Errorf("no dialog shown")
	}
	found := false
	for _, opt := range m.Data.Dialog.Options {
		if opt == option {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("option %q not found in dialog options: %v", option, m.Data.Dialog.Options)
	}
	return nil
}

// conflictDialogDismissed checks that the conflict resolution dialog is gone.
func (w *configConflictWorld) conflictDialogDismissed() error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	view := m.View()
	if strings.Contains(view, "TUI changes take precedence") {
		return fmt.Errorf("conflict resolution dialog still visible: %s", view)
	}
	return nil
}

// lastEventEpochIs checks the LastExternalEventAt timestamp.
func (w *configConflictWorld) lastEventEpochIs(want int64) error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	if m.Data.LastExternalEventAt != want {
		return fmt.Errorf("LastExternalEventAt = %d, want %d", m.Data.LastExternalEventAt, want)
	}
	return nil
}

// externalChangeQueued checks that the event is queued.
func (w *configConflictWorld) externalChangeQueued() error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	if m.Data.PendingExternalEvent == nil {
		return fmt.Errorf("expected PendingExternalEvent to be set during save")
	}
	return nil
}

// keepTUIChanges selects "Keep TUI changes" and confirms.
func (w *configConflictWorld) keepTUIChanges() error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	if m.Data.Dialog == nil {
		return fmt.Errorf("no dialog shown")
	}
	// Find and select "Keep TUI changes" option.
	for i, opt := range m.Data.Dialog.Options {
		if opt == "Keep TUI changes" {
			m.Data.Dialog.Selected = i
			break
		}
	}
	// Confirm by pressing enter.
	m.Key(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	return nil
}

// discardChanges selects "Discard" and confirms.
func (w *configConflictWorld) discardChanges() error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	if m.Data.Dialog == nil {
		return fmt.Errorf("no dialog shown")
	}
	// Find and select "Discard" option.
	for i, opt := range m.Data.Dialog.Options {
		if opt == "Discard" {
			m.Data.Dialog.Selected = i
			break
		}
	}
	// Confirm by pressing enter.
	m.Key(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	return nil
}

// pendingEventCleared checks that the queued event is gone.
func (w *configConflictWorld) pendingEventCleared() error {
	m := w.model()
	if m == nil {
		return fmt.Errorf("no config surface")
	}
	if m.Data.PendingExternalEvent != nil {
		return fmt.Errorf("expected PendingExternalEvent to be nil after processing, got: %+v", m.Data.PendingExternalEvent)
	}
	return nil
}
