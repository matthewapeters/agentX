package state

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// LoadOutputAppletState loads persisted output applet state for a given session.
// If the applet state file does not exist, returns an empty OutputAppletState (no error).
// Returns error for other I/O or parsing failures.
func LoadOutputAppletState(sessionID string, stateDir string) (*OutputAppletState, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID cannot be empty")
	}
	if stateDir == "" {
		return nil, fmt.Errorf("stateDir cannot be empty")
	}
	filePath := OutputAppletStatePathForSession(sessionID, stateDir)
	state := NewOutputAppletState()
	if err := state.LoadFromPath(filePath); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filePath); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		legacyPath := filepath.Join(stateDir, "output_applet_state.json")
		if legacyPath != filePath {
			if legacyErr := state.LoadFromPath(legacyPath); legacyErr != nil {
				return nil, legacyErr
			}
		}
	}
	return state, nil
}

// SaveOutputAppletState saves output applet state for a given session.
// Creates directories as needed.
func SaveOutputAppletState(sessionID string, stateDir string, state *OutputAppletState) error {
	if sessionID == "" {
		return fmt.Errorf("sessionID cannot be empty")
	}
	if stateDir == "" {
		return fmt.Errorf("stateDir cannot be empty")
	}
	if state == nil {
		return fmt.Errorf("state cannot be nil")
	}
	filePath := OutputAppletStatePathForSession(sessionID, stateDir)
	if err := state.SaveToPath(filePath); err != nil {
		// Log error but don't fail initialization; persistence is best-effort
		log.Printf("[output_widget_persistence] Failed to save state to %s: %v", filePath, err)
		return err
	}
	return nil
}

// OutputAppletStatePathForSession returns the full file path for the applet state JSON file.
// Format: {stateDir}/output_applet_state_{sessionID}.json
func OutputAppletStatePathForSession(sessionID string, stateDir string) string {
	if sessionID == "" || stateDir == "" {
		return ""
	}
	return filepath.Join(stateDir, fmt.Sprintf("output_applet_state_%s.json", sessionID))
}
