package runtime

import (
	"testing"

	"agentx/internal/state"
)

func userPromptEvent(ordinal uint64, text string, enabled, ephemeral bool) state.Event {
	return state.Event{
		Ordinal: ordinal, ContentType: state.ContentUserPrompt,
		Payload: map[string]any{"text": text}, Enabled: enabled, Ephemeral: ephemeral,
	}
}

func agentResponseEvent(ordinal uint64, text string, enabled bool) state.Event {
	return state.Event{
		Ordinal: ordinal, ContentType: state.ContentAgentResponse,
		Payload: map[string]any{"text": text}, Enabled: enabled,
	}
}

func toolCallEvent(ordinal uint64, text string, enabled bool) state.Event {
	return state.Event{
		Ordinal: ordinal, ContentType: state.ContentToolCall,
		Payload: map[string]any{"text": text}, Enabled: enabled,
	}
}

func toolResultEvent(ordinal uint64, text string, enabled bool) state.Event {
	return state.Event{
		Ordinal: ordinal, ContentType: state.ContentToolResult,
		Payload: map[string]any{"text": text}, Enabled: enabled,
	}
}

// GIVEN a persisted event log with a user prompt, a tool call/result pair,
// and an agent response, in that order
// WHEN historyFromEvents reconstructs it
// THEN the resulting turnMsg slice preserves that same order, with each
// entry's role and content shape matching what recordTurn would have
// produced live (including the "[pinned tool call]"/"[pinned tool result]"
// content prefixes).
func TestHistoryFromEventsReconstructsInOrder(t *testing.T) {
	events := []state.Event{
		userPromptEvent(1, "implement the thing", true, false),
		toolCallEvent(2, "write_file — foo.go", false),
		toolResultEvent(3, "wrote 42 bytes to foo.go", false),
		agentResponseEvent(4, "done", true),
	}

	got := historyFromEvents(events)
	if len(got) != 4 {
		t.Fatalf("historyFromEvents returned %d entries, want 4: %+v", len(got), got)
	}
	want := []turnMsg{
		{ordinal: 1, role: "user", content: "implement the thing", enabled: true},
		{ordinal: 2, role: "tool", content: "[pinned tool call] write_file — foo.go", enabled: false},
		{ordinal: 3, role: "tool", content: "[pinned tool result] wrote 42 bytes to foo.go", enabled: false},
		{ordinal: 4, role: "assistant", content: "done", enabled: true},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("entry[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

// GIVEN a persisted event log whose first entry is the ephemeral bootstrap
// exchange
// WHEN historyFromEvents reconstructs it
// THEN that entry is excluded — the same exclusion
// internal/surfaces/context.Model.Apply already applies for the same
// reason: it engages the session but isn't part of the user's conversation.
func TestHistoryFromEventsSkipsEphemeralEvents(t *testing.T) {
	events := []state.Event{
		userPromptEvent(1, "bootstrap prompt", true, true),
		agentResponseEvent(2, "welcome message", true),
		userPromptEvent(3, "real prompt", true, false),
	}

	got := historyFromEvents(events)
	if len(got) != 2 {
		t.Fatalf("historyFromEvents returned %d entries, want 2 (ephemeral excluded): %+v", len(got), got)
	}
	if got[0].ordinal != 2 || got[1].ordinal != 3 {
		t.Errorf("ordinals = [%d, %d], want [2, 3] (bootstrap turn's user_prompt at ordinal 1 excluded)", got[0].ordinal, got[1].ordinal)
	}
}

// GIVEN a persisted event log where a tool_result was manually toggled off
// (disabled) by the user before the session stopped
// WHEN historyFromEvents reconstructs it
// THEN the reconstructed entry's enabled field matches the persisted
// event's Enabled value exactly — not defaulted to true (as a fresh
// recordTurn call would for a user/assistant entry) and not defaulted to
// false (as a fresh tool pin would be). This is the scenario most likely to
// be gotten wrong silently: reconstruction must read the disk state, never
// assume a default.
func TestHistoryFromEventsPreservesPersistedEnabledState(t *testing.T) {
	events := []state.Event{
		userPromptEvent(1, "prompt", false, false), // user, manually disabled
		toolCallEvent(2, "read_file", true),        // tool, manually enabled (pinned)
		toolResultEvent(3, "file contents", true),  // tool, manually enabled (pinned)
		agentResponseEvent(4, "response", false),   // assistant, manually disabled
	}

	got := historyFromEvents(events)
	if len(got) != 4 {
		t.Fatalf("historyFromEvents returned %d entries, want 4", len(got))
	}
	wantEnabled := []bool{false, true, true, false}
	for i, want := range wantEnabled {
		if got[i].enabled != want {
			t.Errorf("entry[%d] (ordinal %d) enabled = %v, want %v — must reflect the persisted event, not a fresh-turn default",
				i, got[i].ordinal, got[i].enabled, want)
		}
	}
}

// GIVEN an empty event log
// WHEN historyFromEvents reconstructs it
// THEN it returns an empty (non-nil) slice, not a panic.
func TestHistoryFromEventsEmptyLog(t *testing.T) {
	got := historyFromEvents(nil)
	if len(got) != 0 {
		t.Errorf("historyFromEvents(nil) = %+v, want empty", got)
	}
}

// GIVEN a user_prompt or agent_response event whose payload has no usable
// text (an unexpected shape, or an empty string)
// WHEN historyFromEvents reconstructs it
// THEN that entry is omitted rather than appending an empty turnMsg —
// mirrors recordTurn's own userText/response empty-string guards.
func TestHistoryFromEventsOmitsEmptyTextEntries(t *testing.T) {
	events := []state.Event{
		userPromptEvent(1, "", true, false),
		agentResponseEvent(2, "", true),
		{Ordinal: 3, ContentType: state.ContentUserPrompt, Payload: "not a map", Enabled: true},
	}

	got := historyFromEvents(events)
	if len(got) != 0 {
		t.Errorf("historyFromEvents = %+v, want empty (all entries have no usable text)", got)
	}
}
