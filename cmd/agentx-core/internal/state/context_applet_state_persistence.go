package state

import (
	"fmt"
	"log"
	"path/filepath"
)

// LoadContextAppletState loads persisted context applet state for a given session.
// If the applet state file does not exist, returns an empty ContextAppletState (no error).
// Returns error for other I/O or parsing failures.
func LoadContextAppletState(sessionID string, stateDir string) (*ContextAppletState, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID cannot be empty")
	}
	if stateDir == "" {
		return nil, fmt.Errorf("stateDir cannot be empty")
	}
	filePath := ContextAppletStatePathForSession(sessionID, stateDir)
	state := NewContextAppletState()
	if err := state.LoadFromPath(filePath); err != nil {
		return nil, err
	}
	return state, nil
}

// SaveContextAppletState saves context applet state for a given session.
// Creates directories as needed.
func SaveContextAppletState(sessionID string, stateDir string, state *ContextAppletState) error {
	if sessionID == "" {
		return fmt.Errorf("sessionID cannot be empty")
	}
	if stateDir == "" {
		return fmt.Errorf("stateDir cannot be empty")
	}
	if state == nil {
		return fmt.Errorf("state cannot be nil")
	}
	filePath := ContextAppletStatePathForSession(sessionID, stateDir)
	if err := state.SaveToPath(filePath); err != nil {
		// Log error but don't fail initialization; persistence is best-effort
		log.Printf("[context_widget_persistence] Failed to save state to %s: %v", filePath, err)
		return err
	}
	return nil
}

// ContextAppletStatePathForSession returns the full file path for the applet state JSON file.
// Format: {stateDir}/context_applet_state_{sessionID}.json
func ContextAppletStatePathForSession(sessionID string, stateDir string) string {
	if sessionID == "" || stateDir == "" {
		return ""
	}
	return filepath.Join(stateDir, fmt.Sprintf("context_applet_state_%s.json", sessionID))
}
