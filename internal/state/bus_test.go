package state

import "testing"

// GIVEN a Bus created via NewBusFrom(n)
// WHEN the first event is published
// THEN its stamped ordinal is strictly greater than n — a resumed session's
// new process must never assign an ordinal that collides with one already
// on disk from the session's earlier run.
func TestNewBusFromSeedsOrdinalPastStart(t *testing.T) {
	b := NewBusFrom(41)

	ord := b.Publish(Event{ContentType: ContentUserPrompt})
	if ord <= 41 {
		t.Fatalf("first Publish after NewBusFrom(41) stamped ordinal %d, want > 41", ord)
	}
	if ord != 42 {
		t.Errorf("first Publish after NewBusFrom(41) stamped ordinal %d, want exactly 42", ord)
	}
}

// GIVEN a Bus created via NewBusFrom(0) (no prior history)
// WHEN events are published
// THEN ordinals start at 1, same as a plain NewBus() — resuming a session
// with no persisted events must not behave differently from a fresh one.
func TestNewBusFromZeroMatchesPlainNewBus(t *testing.T) {
	b := NewBusFrom(0)
	ord := b.Publish(Event{ContentType: ContentUserPrompt})
	if ord != 1 {
		t.Errorf("first Publish after NewBusFrom(0) stamped ordinal %d, want 1", ord)
	}
}

// GIVEN a Bus created via NewBusFrom
// WHEN CurrentOrdinal is checked before any Publish call
// THEN it reports the seeded start value, not 0 — a surface attaching
// immediately after resume must see the correct boundary even before any
// new event has been published.
func TestNewBusFromCurrentOrdinalReflectsSeed(t *testing.T) {
	b := NewBusFrom(178)
	if got := b.CurrentOrdinal(); got != 178 {
		t.Errorf("CurrentOrdinal() = %d, want 178", got)
	}
}
