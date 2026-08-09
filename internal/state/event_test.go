package state

import "testing"

// GIVEN ContentSessionSwitching
// WHEN DefaultEnabled is checked
// THEN it reports false, matching ContentConfigChanged's existing
// precedent — it is a surface notification, never conversation context.
func TestDefaultEnabledSessionSwitchingIsFalse(t *testing.T) {
	if DefaultEnabled(ContentSessionSwitching) {
		t.Error("DefaultEnabled(ContentSessionSwitching) = true, want false (a surface notification, never conversation context)")
	}
}

// GIVEN an event with ContentType: ContentSessionSwitching
// WHEN Validate is called
// THEN it passes — the type is registered in validContentTypes, not
// silently rejected by the envelope contract.
func TestEventValidateAcceptsSessionSwitching(t *testing.T) {
	ev := Event{
		Epoch: 1, SessionID: "s1", EventType: "SESSION_SWITCHING",
		ContentType: ContentSessionSwitching,
		Payload:     map[string]any{"session_id": "s2", "session_name": "some-name"},
	}
	if err := ev.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}
