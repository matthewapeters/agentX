package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// OutputAppletState represents persisted state for the output widget.
// It captures the collapsed turn entries and currently focused turn index.
type OutputAppletState struct {
	CollapsedTurns map[int]bool `json:"collapsed_turns"`  // turn idx → collapsed bool
	FocusedTurnIdx int          `json:"focused_turn_idx"` // 1-based, 0/-1 if none focused
	EntryFocusPath []string     `json:"entry_focus_path"` // optional: nested entry breadcrumb (e.g., ["response"])
}

// NewOutputAppletState creates a new OutputAppletState with initialized maps.
func NewOutputAppletState() *OutputAppletState {
	return &OutputAppletState{
		CollapsedTurns: make(map[int]bool),
		FocusedTurnIdx: 0,
		EntryFocusPath: []string{},
	}
}

// ToJSON marshals OutputAppletState to JSON bytes.
func (o *OutputAppletState) ToJSON() ([]byte, error) {
	if o == nil {
		return json.Marshal(NewOutputAppletState())
	}
	return json.MarshalIndent(o, "", "  ")
}

// FromJSON unmarshals JSON bytes into OutputAppletState.
func (o *OutputAppletState) FromJSON(data []byte) error {
	if o == nil {
		return fmt.Errorf("cannot unmarshal into nil OutputAppletState")
	}
	if len(data) == 0 {
		// Empty data: initialize with defaults
		o.CollapsedTurns = make(map[int]bool)
		o.FocusedTurnIdx = 0
		o.EntryFocusPath = []string{}
		return nil
	}
	if err := json.Unmarshal(data, o); err != nil {
		return fmt.Errorf("failed to unmarshal output applet state: %w", err)
	}
	// Ensure maps are non-nil
	if o.CollapsedTurns == nil {
		o.CollapsedTurns = make(map[int]bool)
	}
	if o.EntryFocusPath == nil {
		o.EntryFocusPath = []string{}
	}
	return nil
}

// SaveToPath writes OutputAppletState to disk as JSON.
func (o *OutputAppletState) SaveToPath(filePath string) error {
	if o == nil {
		return fmt.Errorf("cannot save nil OutputAppletState")
	}
	data, err := o.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal output applet state: %w", err)
	}
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write output applet state to %s: %w", filePath, err)
	}
	return nil
}

// LoadFromPath reads OutputAppletState from disk JSON.
// If the file does not exist, returns an empty OutputAppletState (no error).
// Other errors are returned.
func (o *OutputAppletState) LoadFromPath(filePath string) error {
	if o == nil {
		return fmt.Errorf("cannot load into nil OutputAppletState")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File not found: initialize to empty state (no error)
			o.CollapsedTurns = make(map[int]bool)
			o.FocusedTurnIdx = 0
			o.EntryFocusPath = []string{}
			return nil
		}
		return fmt.Errorf("failed to read output applet state from %s: %w", filePath, err)
	}
	return o.FromJSON(data)
}
