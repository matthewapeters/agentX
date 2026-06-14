package state

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
