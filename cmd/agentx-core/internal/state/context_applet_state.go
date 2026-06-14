package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ContextAppletState represents persisted state for the context widget:
// scroll position, focused row, and sort filter preferences.
type ContextAppletState struct {
	ScrollRowOffset int    `json:"scroll_row_offset"`      // viewport top row offset
	FocusedRowKey   string `json:"focused_row_key"`        // stable row key for focus restoration
	SortKey         string `json:"sort_key"`               // e.g., "timestamp", "source", "role"
	SortAscending   bool   `json:"sort_ascending"`         // true = oldest first, false = newest first
	FilterSession   string `json:"filter_session"`         // optional: active session filter (session ID)
}

// NewContextAppletState creates a new context widget state with sensible defaults.
func NewContextAppletState() *ContextAppletState {
	return &ContextAppletState{
		ScrollRowOffset: 0,
		FocusedRowKey:   "",
		SortKey:         "timestamp",
		SortAscending:   true,
		FilterSession:   "",
	}
}

// IsEmpty returns true if state has no meaningful changes from defaults.
func (c *ContextAppletState) IsEmpty() bool {
	return c.ScrollRowOffset == 0 &&
		c.FocusedRowKey == "" &&
		c.SortKey == "timestamp" &&
		c.SortAscending &&
		c.FilterSession == ""
}

// ToJSON marshals ContextAppletState to JSON bytes.
func (c *ContextAppletState) ToJSON() ([]byte, error) {
	if c == nil {
		return json.Marshal(NewContextAppletState())
	}
	return json.MarshalIndent(c, "", "  ")
}

// FromJSON unmarshals JSON bytes into ContextAppletState.
func (c *ContextAppletState) FromJSON(data []byte) error {
	if c == nil {
		return fmt.Errorf("cannot unmarshal into nil ContextAppletState")
	}
	if len(data) == 0 {
		// Empty data: initialize with defaults
		*c = *NewContextAppletState()
		return nil
	}
	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to unmarshal context applet state: %w", err)
	}
	return nil
}

// SaveToPath writes ContextAppletState to disk as JSON.
func (c *ContextAppletState) SaveToPath(filePath string) error {
	if c == nil {
		return fmt.Errorf("cannot save nil ContextAppletState")
	}
	data, err := c.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal context applet state: %w", err)
	}
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write context applet state to %s: %w", filePath, err)
	}
	return nil
}

// LoadFromPath reads ContextAppletState from disk JSON.
// If the file does not exist, returns an empty ContextAppletState (no error).
// Other errors are returned.
func (c *ContextAppletState) LoadFromPath(filePath string) error {
	if c == nil {
		return fmt.Errorf("cannot load into nil ContextAppletState")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File not found: initialize to empty state (no error)
			*c = *NewContextAppletState()
			return nil
		}
		return fmt.Errorf("failed to read context applet state from %s: %w", filePath, err)
	}
	return c.FromJSON(data)
}
