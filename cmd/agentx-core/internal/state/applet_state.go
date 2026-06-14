// Package state provides versioned applet state management.
package state

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// VersionedAppletState represents a versioned snapshot of applet widget state.
// It serves as the foundation for persistence and state interchange between
// Python applets and the Go core.
type VersionedAppletState struct {
	Version      int    `json:"version"`      // Schema version for forward/backward compat
	Widget       string `json:"widget"`       // Widget name (e.g., "chat", "logs", "input")
	Scope        string `json:"scope"`        // Scope: "global" or "session:<sessionID>"
	Payload      []byte `json:"payload"`      // Opaque widget state (base64 encoded in JSON)
	LastUpdateMs int64  `json:"last_update_ms"` // Millisecond timestamp of last update
}

// NewVersionedAppletState constructs a new VersionedAppletState with the current timestamp.
func NewVersionedAppletState(widget, scope string, payload []byte) *VersionedAppletState {
	return &VersionedAppletState{
		Version:      1, // Current schema version
		Widget:       widget,
		Scope:        scope,
		Payload:      payload,
		LastUpdateMs: NowMs(),
	}
}

// NowMs returns the current time in milliseconds since epoch.
func NowMs() int64 {
	return time.Now().UnixMilli()
}

// MarshalJSON implements json.Marshaler for VersionedAppletState.
// Payload is base64-encoded in JSON to preserve binary data.
func (v *VersionedAppletState) MarshalJSON() ([]byte, error) {
	type Alias VersionedAppletState
	return json.Marshal(&struct {
		*Alias
		Payload string `json:"payload"` // base64-encoded string
	}{
		Alias:   (*Alias)(v),
		Payload: base64.StdEncoding.EncodeToString(v.Payload),
	})
}

// UnmarshalJSON implements json.Unmarshaler for VersionedAppletState.
// Reverses MarshalJSON: reconstructs Payload from base64 string.
func (v *VersionedAppletState) UnmarshalJSON(data []byte) error {
	type Alias VersionedAppletState
	aux := &struct {
		Payload string `json:"payload"` // base64-encoded string
		*Alias
	}{
		Alias: (*Alias)(v),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal applet state: %w", err)
	}
	payload, err := base64.StdEncoding.DecodeString(aux.Payload)
	if err != nil {
		return fmt.Errorf("failed to decode base64 payload: %w", err)
	}
	v.Payload = payload
	return nil
}
