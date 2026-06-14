package state

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

// TestVersionedAppletStateRoundTrip verifies that a VersionedAppletState
// correctly marshals to JSON and unmarshals back with all fields preserved.
func TestVersionedAppletStateRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		state   *VersionedAppletState
		wantErr bool
	}{
		{
			name: "simple state with empty payload",
			state: &VersionedAppletState{
				Version:      1,
				Widget:       "chat",
				Scope:        "global",
				Payload:      []byte{},
				LastUpdateMs: 1700000000000,
			},
			wantErr: false,
		},
		{
			name: "state with binary payload",
			state: &VersionedAppletState{
				Version:      1,
				Widget:       "logs",
				Scope:        "session:abc123",
				Payload:      []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd},
				LastUpdateMs: 1700000000000,
			},
			wantErr: false,
		},
		{
			name: "state with large JSON payload",
			state: &VersionedAppletState{
				Version: 1,
				Widget:  "input",
				Scope:   "global",
				Payload: []byte(`{"message":"hello world","count":42,"nested":{"key":"value"}}`),
				LastUpdateMs: 1700000001234,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			marshaled, err := json.Marshal(tt.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			// Unmarshal back
			var unmarshaled VersionedAppletState
			if err := json.Unmarshal(marshaled, &unmarshaled); err != nil {
				t.Errorf("Unmarshal() error = %v", err)
				return
			}

			// Verify all fields match
			if unmarshaled.Version != tt.state.Version {
				t.Errorf("Version mismatch: got %d, want %d", unmarshaled.Version, tt.state.Version)
			}
			if unmarshaled.Widget != tt.state.Widget {
				t.Errorf("Widget mismatch: got %s, want %s", unmarshaled.Widget, tt.state.Widget)
			}
			if unmarshaled.Scope != tt.state.Scope {
				t.Errorf("Scope mismatch: got %s, want %s", unmarshaled.Scope, tt.state.Scope)
			}
			if !bytes.Equal(unmarshaled.Payload, tt.state.Payload) {
				t.Errorf("Payload mismatch: got %v, want %v", unmarshaled.Payload, tt.state.Payload)
			}
			if unmarshaled.LastUpdateMs != tt.state.LastUpdateMs {
				t.Errorf("LastUpdateMs mismatch: got %d, want %d", unmarshaled.LastUpdateMs, tt.state.LastUpdateMs)
			}
		})
	}
}

// TestVersionedAppletStateInvalidJSON verifies that UnmarshalJSON returns
// an error for malformed JSON input.
func TestVersionedAppletStateInvalidJSON(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantErr bool
	}{
		{
			name:    "completely invalid JSON",
			jsonStr: `not valid json at all`,
			wantErr: true,
		},
		{
			name:    "missing required fields",
			jsonStr: `{"version":1}`,
			wantErr: false, // JSON allows missing fields, they'll be zero-valued
		},
		{
			name:    "truncated JSON",
			jsonStr: `{"version":1,"widget":"chat",`,
			wantErr: true,
		},
		{
			name:    "invalid payload type (should be string)",
			jsonStr: `{"version":1,"widget":"chat","scope":"global","payload":12345,"last_update_ms":0}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var state VersionedAppletState
			err := json.Unmarshal([]byte(tt.jsonStr), &state)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestNewVersionedAppletState verifies the constructor sets expected defaults
// and timestamps are reasonable.
func TestNewVersionedAppletState(t *testing.T) {
	now := NowMs()
	state := NewVersionedAppletState("chat", "global", []byte("test payload"))

	if state.Version != 1 {
		t.Errorf("Version = %d, want 1", state.Version)
	}
	if state.Widget != "chat" {
		t.Errorf("Widget = %s, want chat", state.Widget)
	}
	if state.Scope != "global" {
		t.Errorf("Scope = %s, want global", state.Scope)
	}
	if !bytes.Equal(state.Payload, []byte("test payload")) {
		t.Errorf("Payload = %v, want [test payload]", state.Payload)
	}

	// Verify timestamp is approximately now (within 100ms)
	diff := state.LastUpdateMs - now
	if diff < 0 || diff > 100 {
		t.Errorf("LastUpdateMs = %d (diff from now = %d ms), expected ~now", state.LastUpdateMs, diff)
	}
}

// TestNowMs verifies NowMs returns millisecond timestamps that increase over time.
func TestNowMs(t *testing.T) {
	t1 := NowMs()
	time.Sleep(10 * time.Millisecond)
	t2 := NowMs()

	if t2 <= t1 {
		t.Errorf("NowMs() should increase over time: t1=%d, t2=%d", t1, t2)
	}

	// Verify it's approximately milliseconds (roughly 10-13ms apart after 10ms sleep)
	diff := t2 - t1
	if diff < 5 || diff > 50 {
		t.Errorf("NowMs() diff = %d ms, expected ~10ms", diff)
	}
}

// TestScopeForSession verifies session scope formatting.
func TestScopeForSession(t *testing.T) {
	tests := []struct {
		sessionID string
		want      string
	}{
		{"abc123", "session:abc123"},
		{"session-456", "session:session-456"},
		{"", "session:"},
	}

	for _, tt := range tests {
		got := ScopeForSession(tt.sessionID)
		if got != tt.want {
			t.Errorf("ScopeForSession(%q) = %q, want %q", tt.sessionID, got, tt.want)
		}
	}
}

// TestParseSessionScope verifies session scope parsing.
func TestParseSessionScope(t *testing.T) {
	tests := []struct {
		scope      string
		wantID     string
		wantIsSession bool
		wantErr    bool
	}{
		{"global", "", false, false},
		{"session:abc123", "abc123", true, false},
		{"session:session-456", "session-456", true, false},
		{"unknown", "", false, false},
		{"session:", "", false, true}, // Missing sessionID
		{"session", "", false, false}, // No colon, not a session scope
	}

	for _, tt := range tests {
		id, isSession, err := ParseSessionScope(tt.scope)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSessionScope(%q) error = %v, wantErr %v", tt.scope, err, tt.wantErr)
		}
		if id != tt.wantID {
			t.Errorf("ParseSessionScope(%q) id = %q, want %q", tt.scope, id, tt.wantID)
		}
		if isSession != tt.wantIsSession {
			t.Errorf("ParseSessionScope(%q) isSession = %v, want %v", tt.scope, isSession, tt.wantIsSession)
		}
	}
}

// TestScopeRoundTrip verifies ScopeForSession and ParseSessionScope are inverses.
func TestScopeRoundTrip(t *testing.T) {
	tests := []string{
		"abc123",
		"session-456",
		"uuid-7890-abcd",
	}

	for _, original := range tests {
		formatted := ScopeForSession(original)
		parsed, isSession, err := ParseSessionScope(formatted)
		if err != nil {
			t.Errorf("ParseSessionScope(%q) error = %v", formatted, err)
		}
		if !isSession {
			t.Errorf("ParseSessionScope(%q) isSession = false, want true", formatted)
		}
		if parsed != original {
			t.Errorf("Round trip failed for %q: got %q", original, parsed)
		}
	}
}
