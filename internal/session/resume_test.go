package session

import (
	"os"
	"sort"
	"testing"
	"time"

	"agentx/internal/state"
)

// writeUserPrompt records a non-ephemeral user_prompt event for id at the
// given epoch (milliseconds), via the real Recorder — the same code path
// production writes through, not a hand-authored JSON fixture.
func writeUserPrompt(t *testing.T, s *Store, id string, epochMillis int64, text string) {
	t.Helper()
	err := s.Recorder(id).Write(state.Event{
		Epoch:       epochMillis,
		SessionID:   id,
		EventType:   "USER_PROMPT",
		ContentType: state.ContentUserPrompt,
		Payload:     map[string]any{"text": text},
		Enabled:     true,
		Ordinal:     1,
	})
	if err != nil {
		t.Fatalf("write user_prompt event: %v", err)
	}
}

// writeEphemeralUserPrompt records the bootstrap-turn shape: a user_prompt
// event marked Ephemeral, which lastUserPrompt/HasUserPrompt/ListResumable
// must all treat as if it weren't there.
func writeEphemeralUserPrompt(t *testing.T, s *Store, id string, epochMillis int64) {
	t.Helper()
	err := s.Recorder(id).Write(state.Event{
		Epoch:       epochMillis,
		SessionID:   id,
		EventType:   "USER_PROMPT",
		ContentType: state.ContentUserPrompt,
		Payload:     map[string]any{"text": "bootstrap"},
		Enabled:     true,
		Ordinal:     1,
		Ephemeral:   true,
	})
	if err != nil {
		t.Fatalf("write ephemeral user_prompt event: %v", err)
	}
}

// GIVEN a session created via Store.Create
// WHEN Store.Load is called for its ID
// THEN it returns the same Identity Create produced.
func TestLoadRoundTripsIdentityFromCreate(t *testing.T) {
	s := NewStore(t.TempDir())
	created, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	loaded, err := s.Load(created.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded != created {
		t.Errorf("Load = %+v, want %+v", loaded, created)
	}
}

// GIVEN a session ID that was never created
// WHEN Store.Load is called
// THEN it fails with a clear error, not a zero-value success.
func TestLoadFailsForNonexistentSession(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Load("never-created"); err == nil {
		t.Fatal("Load succeeded for a nonexistent session, want an error")
	}
}

// GIVEN a session with a real, non-ephemeral user_prompt
// WHEN HasUserPrompt is called
// THEN it reports true.
func TestHasUserPromptTrueWithRealPrompt(t *testing.T) {
	s := NewStore(t.TempDir())
	id, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeUserPrompt(t, s, id.ID, time.Now().UnixMilli(), "hello")

	found, err := s.HasUserPrompt(id.ID)
	if err != nil {
		t.Fatalf("HasUserPrompt: %v", err)
	}
	if !found {
		t.Error("HasUserPrompt = false, want true")
	}
}

// GIVEN a session with only an ephemeral (bootstrap) user_prompt, no real one
// WHEN HasUserPrompt is called
// THEN it reports false — the same predicate ListResumable uses to exclude
// this session from the picker also governs whether it's a deletion
// candidate elsewhere; both must agree an ephemeral-only session is empty.
func TestHasUserPromptFalseForEphemeralOnly(t *testing.T) {
	s := NewStore(t.TempDir())
	id, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeEphemeralUserPrompt(t, s, id.ID, time.Now().UnixMilli())

	found, err := s.HasUserPrompt(id.ID)
	if err != nil {
		t.Fatalf("HasUserPrompt: %v", err)
	}
	if found {
		t.Error("HasUserPrompt = true, want false (ephemeral-only session)")
	}
}

// GIVEN a session with no events at all
// WHEN HasUserPrompt is called
// THEN it reports false without error.
func TestHasUserPromptFalseForNoEvents(t *testing.T) {
	s := NewStore(t.TempDir())
	id, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	found, err := s.HasUserPrompt(id.ID)
	if err != nil {
		t.Fatalf("HasUserPrompt: %v", err)
	}
	if found {
		t.Error("HasUserPrompt = true, want false (no events at all)")
	}
}

// GIVEN a session with no recorded user prompt (only the bootstrap exchange)
// WHEN RemoveIfEmpty runs
// THEN the session's directory is deleted and removed reports true.
func TestRemoveIfEmptyDeletesEmptySession(t *testing.T) {
	s := NewStore(t.TempDir())
	id, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeEphemeralUserPrompt(t, s, id.ID, time.Now().UnixMilli())

	removed, err := s.RemoveIfEmpty(id.ID)
	if err != nil {
		t.Fatalf("RemoveIfEmpty: %v", err)
	}
	if !removed {
		t.Error("RemoveIfEmpty = false, want true (session has no real prompt)")
	}
	if _, err := os.Stat(s.Dir(id.ID)); !os.IsNotExist(err) {
		t.Errorf("session directory still exists after RemoveIfEmpty, stat err = %v", err)
	}
}

// GIVEN a session with a real, non-ephemeral user prompt — even a session
// being abandoned mid-conversation
// WHEN RemoveIfEmpty runs
// THEN nothing is removed: removed reports false and the directory is left
// fully intact. This must hold regardless of how the session is being
// abandoned; content is the only thing that matters.
func TestRemoveIfEmptyLeavesNonEmptySessionIntact(t *testing.T) {
	s := NewStore(t.TempDir())
	id, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeUserPrompt(t, s, id.ID, time.Now().UnixMilli(), "real conversation in progress")

	removed, err := s.RemoveIfEmpty(id.ID)
	if err != nil {
		t.Fatalf("RemoveIfEmpty: %v", err)
	}
	if removed {
		t.Error("RemoveIfEmpty = true, want false (session has a real prompt, must never be deleted)")
	}
	if _, err := os.Stat(s.Dir(id.ID)); err != nil {
		t.Errorf("session directory was removed despite having real content: stat err = %v", err)
	}
	got, err := s.Load(id.ID)
	if err != nil || got != id {
		t.Errorf("session identity after RemoveIfEmpty = %+v, err=%v — want it fully intact", got, err)
	}
}

// GIVEN a session with no user_prompt events (only ephemeral ones)
// WHEN ListResumable runs
// THEN that session is excluded from the results.
func TestListResumableExcludesEmptySession(t *testing.T) {
	s := NewStore(t.TempDir())
	empty, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeEphemeralUserPrompt(t, s, empty.ID, time.Now().UnixMilli())

	withPrompt, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeUserPrompt(t, s, withPrompt.ID, time.Now().UnixMilli(), "do something")

	got, err := s.ListResumable(0)
	if err != nil {
		t.Fatalf("ListResumable: %v", err)
	}
	if len(got) != 1 || got[0].ID != withPrompt.ID {
		t.Fatalf("ListResumable = %+v, want exactly the session with a real prompt", got)
	}
}

// GIVEN a session with a real user_prompt
// WHEN ListResumable runs
// THEN the candidate carries the correct name, last-prompt text, and
// timestamp.
func TestListResumableReturnsCorrectFields(t *testing.T) {
	s := NewStore(t.TempDir())
	id, err := s.Create(WithNamer(func() string { return "test-session" }))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	epoch := time.Now().UnixMilli()
	writeUserPrompt(t, s, id.ID, epoch, "implement the thing")

	got, err := s.ListResumable(0)
	if err != nil {
		t.Fatalf("ListResumable: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListResumable returned %d candidates, want 1", len(got))
	}
	c := got[0]
	if c.ID != id.ID || c.Name != "test-session" || c.LastPrompt != "implement the thing" {
		t.Errorf("candidate = %+v, want ID=%s Name=test-session LastPrompt=%q", c, id.ID, "implement the thing")
	}
	if !c.LastPromptAt.Equal(time.UnixMilli(epoch)) {
		t.Errorf("LastPromptAt = %v, want %v", c.LastPromptAt, time.UnixMilli(epoch))
	}
}

// GIVEN a session with multiple user_prompt events at different times
// WHEN ListResumable runs
// THEN the candidate's last-prompt text/timestamp reflects the MOST RECENT
// one, not the first.
func TestListResumableUsesMostRecentPrompt(t *testing.T) {
	s := NewStore(t.TempDir())
	id, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base := time.Now().UnixMilli()
	writeUserPrompt(t, s, id.ID, base, "first prompt")
	writeUserPrompt(t, s, id.ID, base+1000, "second prompt")
	writeUserPrompt(t, s, id.ID, base+2000, "third and most recent")

	got, err := s.ListResumable(0)
	if err != nil {
		t.Fatalf("ListResumable: %v", err)
	}
	if len(got) != 1 || got[0].LastPrompt != "third and most recent" {
		t.Fatalf("ListResumable = %+v, want the most recent prompt's text", got)
	}
}

// GIVEN several sessions with prompts at different times
// WHEN ListResumable runs
// THEN they are returned most-recent-first.
func TestListResumableSortsMostRecentFirst(t *testing.T) {
	s := NewStore(t.TempDir())
	base := time.Now().UnixMilli()

	older, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeUserPrompt(t, s, older.ID, base, "older")

	newer, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeUserPrompt(t, s, newer.ID, base+5000, "newer")

	got, err := s.ListResumable(0)
	if err != nil {
		t.Fatalf("ListResumable: %v", err)
	}
	if len(got) != 2 || got[0].ID != newer.ID || got[1].ID != older.ID {
		t.Fatalf("ListResumable order = %+v, want newer then older", got)
	}
	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i].LastPromptAt.After(got[j].LastPromptAt) }) {
		t.Error("ListResumable result is not sorted most-recent-first")
	}
}

// GIVEN more resumable sessions than the requested limit
// WHEN ListResumable runs with that limit
// THEN exactly that many are returned, the most recent ones.
func TestListResumableRespectsLimit(t *testing.T) {
	s := NewStore(t.TempDir())
	base := time.Now().UnixMilli()
	for i := range 5 {
		id, err := s.Create()
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		writeUserPrompt(t, s, id.ID, base+int64(i)*1000, "prompt")
	}

	got, err := s.ListResumable(2)
	if err != nil {
		t.Fatalf("ListResumable: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListResumable(2) returned %d candidates, want 2", len(got))
	}
}

// GIVEN no sessions at all (a fresh, empty session root)
// WHEN ListResumable runs
// THEN it returns an empty result, not an error.
func TestListResumableEmptyRoot(t *testing.T) {
	s := NewStore(t.TempDir())
	got, err := s.ListResumable(0)
	if err != nil {
		t.Fatalf("ListResumable: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListResumable = %+v, want empty", got)
	}
}

// GIVEN a session root directory that doesn't exist on disk at all
// WHEN ListResumable runs
// THEN it returns an empty result, not an error — mirrors Store.usedNames'
// existing os.IsNotExist tolerance for an as-yet-unused root.
func TestListResumableNonexistentRoot(t *testing.T) {
	s := NewStore(t.TempDir() + "/does-not-exist")
	got, err := s.ListResumable(0)
	if err != nil {
		t.Fatalf("ListResumable: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListResumable = %+v, want empty", got)
	}
}

// GIVEN a directory under the session root that isn't a real session (no
// session.json, or a malformed one)
// WHEN ListResumable runs
// THEN that directory is skipped, not fatal to the rest of the listing.
func TestListResumableSkipsDirectoryWithoutValidSessionJSON(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := os.MkdirAll(s.Dir("not-a-session"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	real, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeUserPrompt(t, s, real.ID, time.Now().UnixMilli(), "real session")

	got, err := s.ListResumable(0)
	if err != nil {
		t.Fatalf("ListResumable: %v", err)
	}
	if len(got) != 1 || got[0].ID != real.ID {
		t.Fatalf("ListResumable = %+v, want exactly the one real session", got)
	}
}

// GIVEN a session whose events/ directory contains a stray subdirectory
// WHEN lastUserPrompt scans it
// THEN the subdirectory is skipped without error.
func TestLastUserPromptSkipsSubdirectoryInEvents(t *testing.T) {
	s := NewStore(t.TempDir())
	id, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeUserPrompt(t, s, id.ID, time.Now().UnixMilli(), "hello")
	if err := os.MkdirAll(s.Dir(id.ID)+"/events/stray-subdir", 0o755); err != nil {
		t.Fatalf("mkdir stray subdir: %v", err)
	}

	found, err := s.HasUserPrompt(id.ID)
	if err != nil {
		t.Fatalf("HasUserPrompt: %v", err)
	}
	if !found {
		t.Error("HasUserPrompt = false, want true (stray subdirectory must not break the scan)")
	}
}

// GIVEN a session whose events/ directory contains a file that isn't valid
// JSON (corrupted, truncated write, etc.)
// WHEN lastUserPrompt scans it
// THEN that file is skipped, not fatal to finding the real event.
func TestLastUserPromptSkipsUnreadableEventFile(t *testing.T) {
	s := NewStore(t.TempDir())
	id, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeUserPrompt(t, s, id.ID, time.Now().UnixMilli(), "hello")
	// Sorts after the real event's filename (higher epoch prefix), so the
	// reverse scan hits this corrupt file first.
	garbage := s.Dir(id.ID) + "/events/9999999999999_999999_garbage.json"
	if err := os.WriteFile(garbage, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write garbage event file: %v", err)
	}

	found, err := s.HasUserPrompt(id.ID)
	if err != nil {
		t.Fatalf("HasUserPrompt: %v", err)
	}
	if !found {
		t.Error("HasUserPrompt = false, want true (a corrupt event file must not hide a real earlier one)")
	}
}

// GIVEN a user_prompt event whose payload isn't the expected
// map[string]any{"text": ...} shape
// WHEN textOfUserPrompt extracts it
// THEN it reports ok=false rather than panicking on the type assertion.
func TestTextOfUserPromptHandlesUnexpectedPayloadShape(t *testing.T) {
	if _, ok := textOfUserPrompt(state.Event{Payload: "not a map"}); ok {
		t.Error("textOfUserPrompt ok = true for a non-map payload, want false")
	}
	if _, ok := textOfUserPrompt(state.Event{Payload: map[string]any{"text": 42}}); ok {
		t.Error("textOfUserPrompt ok = true for a non-string text field, want false")
	}
}
