package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	appstate "github.com/matthewapeters/agentX/cmd/agentx-core/internal/state"
)

// TestContextAppletStateRoundTrip verifies that context applet state can be saved and loaded
// with all fields preserved.
func TestContextAppletStateRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		state *appstate.ContextAppletState
	}{
		{
			name: "default state",
			state: &appstate.ContextAppletState{
				ScrollRowOffset: 0,
				FocusedRowKey:   "",
				SortKey:         "timestamp",
				SortAscending:   true,
				FilterSession:   "",
			},
		},
		{
			name: "scrolled state with focus",
			state: &appstate.ContextAppletState{
				ScrollRowOffset: 10,
				FocusedRowKey:   "user:alice:session123",
				SortKey:         "timestamp",
				SortAscending:   false,
				FilterSession:   "session123",
			},
		},
		{
			name: "custom sort key",
			state: &appstate.ContextAppletState{
				ScrollRowOffset: 5,
				FocusedRowKey:   "turn:alice:session123:3",
				SortKey:         "source",
				SortAscending:   true,
				FilterSession:   "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for the test
			tmpDir := t.TempDir()
			sessionID := "test-session-123"

			// Save state
			filePath := appstate.ContextAppletStatePathForSession(sessionID, tmpDir)
			if err := appstate.SaveContextAppletState(sessionID, tmpDir, tt.state); err != nil {
				t.Fatalf("SaveContextAppletState failed: %v", err)
			}

			// Verify file exists
			if _, err := os.Stat(filePath); err != nil {
				t.Fatalf("State file not created at %s: %v", filePath, err)
			}

			// Load state
			loaded, err := appstate.LoadContextAppletState(sessionID, tmpDir)
			if err != nil {
				t.Fatalf("LoadContextAppletState failed: %v", err)
			}

			// Verify all fields match
			if loaded.ScrollRowOffset != tt.state.ScrollRowOffset {
				t.Errorf("ScrollRowOffset mismatch: got %d, want %d", loaded.ScrollRowOffset, tt.state.ScrollRowOffset)
			}
			if loaded.FocusedRowKey != tt.state.FocusedRowKey {
				t.Errorf("FocusedRowKey mismatch: got %q, want %q", loaded.FocusedRowKey, tt.state.FocusedRowKey)
			}
			if loaded.SortKey != tt.state.SortKey {
				t.Errorf("SortKey mismatch: got %q, want %q", loaded.SortKey, tt.state.SortKey)
			}
			if loaded.SortAscending != tt.state.SortAscending {
				t.Errorf("SortAscending mismatch: got %v, want %v", loaded.SortAscending, tt.state.SortAscending)
			}
			if loaded.FilterSession != tt.state.FilterSession {
				t.Errorf("FilterSession mismatch: got %q, want %q", loaded.FilterSession, tt.state.FilterSession)
			}
		})
	}
}

// TestContextAppletStateFirstRunGracefulFallback verifies that loading from
// a non-existent file returns a sensible default state without error.
func TestContextAppletStateFirstRunGracefulFallback(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "new-session-999"

	// Try to load state that doesn't exist yet (first run)
	loaded, err := appstate.LoadContextAppletState(sessionID, tmpDir)
	if err != nil {
		t.Fatalf("LoadContextAppletState should not error on first run: %v", err)
	}

	// Verify it returns sensible defaults
	if loaded == nil {
		t.Fatal("Expected non-nil state, got nil")
	}
	if loaded.ScrollRowOffset != 0 {
		t.Errorf("ScrollRowOffset = %d, want 0", loaded.ScrollRowOffset)
	}
	if loaded.FocusedRowKey != "" {
		t.Errorf("FocusedRowKey = %q, want empty string", loaded.FocusedRowKey)
	}
	if loaded.SortKey != "timestamp" {
		t.Errorf("SortKey = %q, want 'timestamp'", loaded.SortKey)
	}
	if !loaded.SortAscending {
		t.Errorf("SortAscending = %v, want true", loaded.SortAscending)
	}
	if loaded.FilterSession != "" {
		t.Errorf("FilterSession = %q, want empty string", loaded.FilterSession)
	}
}

// TestContextAppletStateSessionIsolation verifies that different sessions
// have independent persisted appstate.
func TestContextAppletStateSessionIsolation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and save state for session A
	sessionA := "session-A-111"
	stateA := &appstate.ContextAppletState{
		ScrollRowOffset: 10,
		FocusedRowKey:   "user:alice",
		SortKey:         "timestamp",
		SortAscending:   false,
		FilterSession:   "sessionA",
	}
	if err := appstate.SaveContextAppletState(sessionA, tmpDir, stateA); err != nil {
		t.Fatalf("SaveContextAppletState for session A failed: %v", err)
	}

	// Create and save state for session B
	sessionB := "session-B-222"
	stateB := &appstate.ContextAppletState{
		ScrollRowOffset: 20,
		FocusedRowKey:   "user:bob",
		SortKey:         "source",
		SortAscending:   true,
		FilterSession:   "sessionB",
	}
	if err := appstate.SaveContextAppletState(sessionB, tmpDir, stateB); err != nil {
		t.Fatalf("SaveContextAppletState for session B failed: %v", err)
	}

	// Load state for session A and verify it's unchanged
	loadedA, err := appstate.LoadContextAppletState(sessionA, tmpDir)
	if err != nil {
		t.Fatalf("LoadContextAppletState for session A failed: %v", err)
	}
	if loadedA.ScrollRowOffset != 10 {
		t.Errorf("Session A ScrollRowOffset = %d, want 10", loadedA.ScrollRowOffset)
	}
	if loadedA.FocusedRowKey != "user:alice" {
		t.Errorf("Session A FocusedRowKey = %q, want 'user:alice'", loadedA.FocusedRowKey)
	}

	// Load state for session B and verify it's unchanged
	loadedB, err := appstate.LoadContextAppletState(sessionB, tmpDir)
	if err != nil {
		t.Fatalf("LoadContextAppletState for session B failed: %v", err)
	}
	if loadedB.ScrollRowOffset != 20 {
		t.Errorf("Session B ScrollRowOffset = %d, want 20", loadedB.ScrollRowOffset)
	}
	if loadedB.FocusedRowKey != "user:bob" {
		t.Errorf("Session B FocusedRowKey = %q, want 'user:bob'", loadedB.FocusedRowKey)
	}

	// Verify file paths are different
	pathA := appstate.ContextAppletStatePathForSession(sessionA, tmpDir)
	pathB := appstate.ContextAppletStatePathForSession(sessionB, tmpDir)
	if pathA == pathB {
		t.Errorf("Session paths should be different: %s vs %s", pathA, pathB)
	}

	// Verify both files exist
	if _, err := os.Stat(pathA); err != nil {
		t.Errorf("Session A file should exist: %v", err)
	}
	if _, err := os.Stat(pathB); err != nil {
		t.Errorf("Session B file should exist: %v", err)
	}
}

// TestContextAppletStateEmptyHandling verifies that empty state is handled correctly.
func TestContextAppletStateEmptyHandling(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session-empty"

	// Save an empty state (all defaults)
	emptyState := appstate.NewContextAppletState()
	if err := appstate.SaveContextAppletState(sessionID, tmpDir, emptyState); err != nil {
		t.Fatalf("SaveContextAppletState failed: %v", err)
	}

	// Load and verify
	loaded, err := appstate.LoadContextAppletState(sessionID, tmpDir)
	if err != nil {
		t.Fatalf("LoadContextAppletState failed: %v", err)
	}

	if !loaded.IsEmpty() {
		t.Errorf("IsEmpty() = false, want true")
	}
}

// TestContextAppletStatePathForSession verifies the file path is constructed correctly.
func TestContextAppletStatePathForSession(t *testing.T) {
	tests := []struct {
		sessionID string
		stateDir  string
		want      string
	}{
		{
			sessionID: "session123",
			stateDir:  "/tmp/state",
			want:      filepath.Join("/tmp/state", "context_applet_state_session123.json"),
		},
		{
			sessionID: "session-with-dash",
			stateDir:  "/home/user/.agentx/state",
			want:      filepath.Join("/home/user/.agentx/state", "context_applet_state_session-with-dash.json"),
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_in_%s", tt.sessionID, tt.stateDir), func(t *testing.T) {
			got := appstate.ContextAppletStatePathForSession(tt.sessionID, tt.stateDir)
			if got != tt.want {
				t.Errorf("ContextAppletStatePathForSession(%q, %q) = %q, want %q", tt.sessionID, tt.stateDir, got, tt.want)
			}
		})
	}
}

// TestContextAppletStateInvalidInputs verifies error handling for invalid inputs.
func TestContextAppletStateInvalidInputs(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		sessionID string
		stateDir  string
		state     *appstate.ContextAppletState
		wantErr   bool
	}{
		{
			name:      "empty sessionID",
			sessionID: "",
			stateDir:  tmpDir,
			state:     appstate.NewContextAppletState(),
			wantErr:   true,
		},
		{
			name:      "empty stateDir",
			sessionID: "session123",
			stateDir:  "",
			state:     appstate.NewContextAppletState(),
			wantErr:   true,
		},
		{
			name:      "nil state",
			sessionID: "session123",
			stateDir:  tmpDir,
			state:     nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := appstate.SaveContextAppletState(tt.sessionID, tt.stateDir, tt.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("SaveContextAppletState error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestContextAppletStateLoadInvalidJSON verifies that malformed JSON is handled gracefully.
func TestContextAppletStateLoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-invalid-json"
	filePath := appstate.ContextAppletStatePathForSession(sessionID, tmpDir)

	// Create directory
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Write invalid JSON to file
	invalidJSON := []byte(`{invalid json content`)
	if err := os.WriteFile(filePath, invalidJSON, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Try to load the invalid JSON
	loaded, err := appstate.LoadContextAppletState(sessionID, tmpDir)
	if err == nil {
		t.Fatal("Expected error loading invalid JSON, got nil")
	}
	if loaded != nil {
		t.Errorf("Expected nil state for invalid JSON, got %v", loaded)
	}
}

// TestContextAppletStateWithValidJSON verifies that well-formed JSON files are loaded correctly.
func TestContextAppletStateWithValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-valid-json"
	filePath := appstate.ContextAppletStatePathForSession(sessionID, tmpDir)

	// Create directory
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Write valid JSON to file
	validJSON := []byte(`{
  "scroll_row_offset": 15,
  "focused_row_key": "user:test_user",
  "sort_key": "timestamp",
  "sort_ascending": false,
  "filter_session": "test_session"
}`)
	if err := os.WriteFile(filePath, validJSON, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Load the JSON
	loaded, err := appstate.LoadContextAppletState(sessionID, tmpDir)
	if err != nil {
		t.Fatalf("LoadContextAppletState failed: %v", err)
	}

	// Verify fields match the written JSON
	if loaded.ScrollRowOffset != 15 {
		t.Errorf("ScrollRowOffset = %d, want 15", loaded.ScrollRowOffset)
	}
	if loaded.FocusedRowKey != "user:test_user" {
		t.Errorf("FocusedRowKey = %q, want 'user:test_user'", loaded.FocusedRowKey)
	}
	if loaded.SortKey != "timestamp" {
		t.Errorf("SortKey = %q, want 'timestamp'", loaded.SortKey)
	}
	if loaded.SortAscending {
		t.Errorf("SortAscending = %v, want false", loaded.SortAscending)
	}
	if loaded.FilterSession != "test_session" {
		t.Errorf("FilterSession = %q, want 'test_session'", loaded.FilterSession)
	}
}
