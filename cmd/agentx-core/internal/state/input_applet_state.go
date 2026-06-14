package state

// InputAppletState represents persisted state for the input widget.
// Phase 1 (current): skeleton only, no runtime integration yet.
// Phase 2: will include compose-box scroll and optional input history collection.
type InputAppletState struct {
	ComposeBoxScrollOffset int      `json:"compose_box_scroll_offset"`       // Phase 2: horizontal scroll position
	RecentInputHistory     []string `json:"recent_input_history,omitempty"`  // Phase 2: optional history (max 50 entries)
	LastInputTimestamp     int64    `json:"last_input_timestamp"`            // Millisecond timestamp of last input
}

// NewInputAppletState creates a new input widget state with sensible defaults.
func NewInputAppletState() *InputAppletState {
	return &InputAppletState{
		ComposeBoxScrollOffset: 0,
		RecentInputHistory:     []string{},
		LastInputTimestamp:     0,
	}
}

// IsEmpty returns true if state has no meaningful changes from defaults.
func (i *InputAppletState) IsEmpty() bool {
	return i.ComposeBoxScrollOffset == 0 &&
		len(i.RecentInputHistory) == 0 &&
		i.LastInputTimestamp == 0
}
