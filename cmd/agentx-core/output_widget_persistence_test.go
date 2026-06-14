package main

import (
	"testing"

	"github.com/matthewapeters/agentX/cmd/agentx-core/internal/state"
)

// TestOutputWidgetPersistence_LoadNonExistentFile verifies graceful handling
// when applet state file does not exist (first run).
func TestOutputWidgetPersistence_LoadNonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session-001"

	// Load from non-existent file
	loaded, err := state.LoadOutputAppletState(sessionID, tmpDir)
	if err != nil {
		t.Fatalf("LoadOutputAppletState should not error on missing file; got %v", err)
	}
	if loaded == nil {
		t.Fatalf("LoadOutputAppletState returned nil state")
	}

	// Verify state is initialized with defaults
	if len(loaded.CollapsedTurns) != 0 {
		t.Errorf("Expected empty collapsed turns; got %v", loaded.CollapsedTurns)
	}
	if loaded.FocusedTurnIdx != 0 {
		t.Errorf("Expected focused turn idx 0; got %d", loaded.FocusedTurnIdx)
	}
	if loaded.EntryFocusPath == nil {
		t.Errorf("Expected non-nil EntryFocusPath; got nil")
	}
}

// TestOutputWidgetPersistence_SaveAndReloadPreservesCollapsedAndFocused verifies
// collapsed turns and focused turn survive a save -> reload cycle.
func TestOutputWidgetPersistence_SaveAndReloadPreservesCollapsedAndFocused(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session-002"

	// Create initial state with collapsed turns
	persisted := state.NewOutputAppletState()
	persisted.CollapsedTurns[1] = true
	persisted.CollapsedTurns[3] = true
	persisted.FocusedTurnIdx = 2
	persisted.EntryFocusPath = []string{"response"}

	// Save state
	if err := state.SaveOutputAppletState(sessionID, tmpDir, persisted); err != nil {
		t.Fatalf("SaveOutputAppletState failed: %v", err)
	}

	// Load state back
	loaded, err := state.LoadOutputAppletState(sessionID, tmpDir)
	if err != nil {
		t.Fatalf("LoadOutputAppletState failed: %v", err)
	}

	// Verify collapsed state matches
	if !loaded.CollapsedTurns[1] {
		t.Errorf("Expected turn 1 to be collapsed")
	}
	if !loaded.CollapsedTurns[3] {
		t.Errorf("Expected turn 3 to be collapsed")
	}
	if loaded.CollapsedTurns[2] {
		t.Errorf("Expected turn 2 to not be collapsed")
	}
	if loaded.FocusedTurnIdx != 2 {
		t.Errorf("Expected focused turn 2; got %d", loaded.FocusedTurnIdx)
	}
	if len(loaded.EntryFocusPath) != 1 || loaded.EntryFocusPath[0] != "response" {
		t.Errorf("Expected entry focus path [response]; got %v", loaded.EntryFocusPath)
	}
}

// TestOutputWidgetPersistence_SessionIsolation verifies that different sessions
// maintain separate state files and do not interfere with each other.
func TestOutputWidgetPersistence_SessionIsolation(t *testing.T) {
	tmpDir := t.TempDir()

	// Session A: save state with turn 1 and 2 collapsed
	stateA := state.NewOutputAppletState()
	stateA.CollapsedTurns[1] = true
	stateA.CollapsedTurns[2] = true
	stateA.FocusedTurnIdx = 1
	if err := state.SaveOutputAppletState("session-a", tmpDir, stateA); err != nil {
		t.Fatalf("Failed to save session A state: %v", err)
	}

	// Session B: save different state with turn 5 collapsed
	stateB := state.NewOutputAppletState()
	stateB.CollapsedTurns[5] = true
	stateB.FocusedTurnIdx = 3
	if err := state.SaveOutputAppletState("session-b", tmpDir, stateB); err != nil {
		t.Fatalf("Failed to save session B state: %v", err)
	}

	// Load both back and verify they're different
	loadedA, err := state.LoadOutputAppletState("session-a", tmpDir)
	if err != nil {
		t.Fatalf("Failed to load session A state: %v", err)
	}
	loadedB, err := state.LoadOutputAppletState("session-b", tmpDir)
	if err != nil {
		t.Fatalf("Failed to load session B state: %v", err)
	}

	// Verify A state
	if !loadedA.CollapsedTurns[1] || !loadedA.CollapsedTurns[2] {
		t.Errorf("Session A: expected turns 1 and 2 collapsed")
	}
	if loadedA.FocusedTurnIdx != 1 {
		t.Errorf("Session A: expected focused turn 1; got %d", loadedA.FocusedTurnIdx)
	}

	// Verify B state
	if !loadedB.CollapsedTurns[5] {
		t.Errorf("Session B: expected turn 5 collapsed")
	}
	if loadedB.CollapsedTurns[1] || loadedB.CollapsedTurns[2] {
		t.Errorf("Session B: turns 1 and 2 should not be collapsed")
	}
	if loadedB.FocusedTurnIdx != 3 {
		t.Errorf("Session B: expected focused turn 3; got %d", loadedB.FocusedTurnIdx)
	}
}
