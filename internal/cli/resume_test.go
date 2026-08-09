package cli

import (
	"strings"
	"testing"
	"time"

	"agentx/internal/session"
	"agentx/internal/state"
)

func newResumeTestSession(t *testing.T, s *session.Store, name, prompt string, epoch int64) string {
	t.Helper()
	id, err := s.Create(session.WithNamer(func() string { return name }))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	err = s.Recorder(id.ID).Write(state.Event{
		Epoch: epoch, SessionID: id.ID, EventType: "USER_PROMPT",
		ContentType: state.ContentUserPrompt, Payload: map[string]any{"text": prompt}, Enabled: true,
	})
	if err != nil {
		t.Fatalf("write user_prompt: %v", err)
	}
	return id.ID
}

// GIVEN exactly one resumable session and an empty target
// WHEN ResolveResume runs
// THEN it resolves to that session directly, without prompting — no input
// is read at all.
func TestResolveResumeSingleCandidateNoPrompt(t *testing.T) {
	s := session.NewStore(t.TempDir())
	id := newResumeTestSession(t, s, "only-session", "do a thing", time.Now().UnixMilli())

	got, err := ResolveResume(s, "", strings.NewReader(""), &strings.Builder{})
	if err != nil {
		t.Fatalf("ResolveResume: %v", err)
	}
	if got != id {
		t.Errorf("ResolveResume = %q, want %q", got, id)
	}
}

// GIVEN target "last" with multiple resumable sessions
// WHEN ResolveResume runs
// THEN it resolves to the most recent one without prompting.
func TestResolveResumeLastKeyword(t *testing.T) {
	s := session.NewStore(t.TempDir())
	base := time.Now().UnixMilli()
	newResumeTestSession(t, s, "older", "older prompt", base)
	newer := newResumeTestSession(t, s, "newer", "newer prompt", base+5000)

	got, err := ResolveResume(s, "last", strings.NewReader(""), &strings.Builder{})
	if err != nil {
		t.Fatalf("ResolveResume: %v", err)
	}
	if got != newer {
		t.Errorf("ResolveResume(last) = %q, want %q (the most recent)", got, newer)
	}
}

// GIVEN a target matching a candidate's exact session ID
// WHEN ResolveResume runs
// THEN it resolves to that session.
func TestResolveResumeMatchByID(t *testing.T) {
	s := session.NewStore(t.TempDir())
	id := newResumeTestSession(t, s, "some-name", "prompt", time.Now().UnixMilli())

	got, err := ResolveResume(s, id, strings.NewReader(""), &strings.Builder{})
	if err != nil {
		t.Fatalf("ResolveResume: %v", err)
	}
	if got != id {
		t.Errorf("ResolveResume(id) = %q, want %q", got, id)
	}
}

// GIVEN a target matching a candidate's name, not its ID
// WHEN ResolveResume runs
// THEN it resolves to that session — matching either field works.
func TestResolveResumeMatchByName(t *testing.T) {
	s := session.NewStore(t.TempDir())
	id := newResumeTestSession(t, s, "friendly-name", "prompt", time.Now().UnixMilli())

	got, err := ResolveResume(s, "friendly-name", strings.NewReader(""), &strings.Builder{})
	if err != nil {
		t.Fatalf("ResolveResume: %v", err)
	}
	if got != id {
		t.Errorf("ResolveResume(name) = %q, want %q", got, id)
	}
}

// GIVEN a target that matches no candidate's ID or name
// WHEN ResolveResume runs
// THEN it fails with a clear error — never falls back to some default
// session.
func TestResolveResumeNoMatchFails(t *testing.T) {
	s := session.NewStore(t.TempDir())
	newResumeTestSession(t, s, "real-session", "prompt", time.Now().UnixMilli())

	_, err := ResolveResume(s, "nonexistent-name", strings.NewReader(""), &strings.Builder{})
	if err == nil {
		t.Fatal("ResolveResume(nonexistent) succeeded, want an error")
	}
}

// GIVEN no resumable sessions at all
// WHEN ResolveResume runs
// THEN it fails with a clear error.
func TestResolveResumeNoCandidatesFails(t *testing.T) {
	s := session.NewStore(t.TempDir())
	_, err := ResolveResume(s, "", strings.NewReader(""), &strings.Builder{})
	if err == nil {
		t.Fatal("ResolveResume with no candidates succeeded, want an error")
	}
}

// GIVEN multiple resumable sessions and an empty target
// WHEN ResolveResume runs
// THEN it prints a numbered list (name + last prompt) and reads a selection
// from the given reader, resolving to the chosen session.
func TestResolveResumeMultipleCandidatesPrompts(t *testing.T) {
	s := session.NewStore(t.TempDir())
	base := time.Now().UnixMilli()
	first := newResumeTestSession(t, s, "first-session", "implement the thing", base+5000)
	newResumeTestSession(t, s, "second-session", "review the code", base)

	var out strings.Builder
	got, err := ResolveResume(s, "", strings.NewReader("1\n"), &out)
	if err != nil {
		t.Fatalf("ResolveResume: %v", err)
	}
	if got != first {
		t.Errorf("ResolveResume(picker, selects 1) = %q, want %q (most recent, listed first)", got, first)
	}
	if !strings.Contains(out.String(), "first-session") || !strings.Contains(out.String(), "implement the thing") {
		t.Errorf("picker output = %q, want it to list the session name and last prompt", out.String())
	}
}

// GIVEN multiple candidates and an out-of-range selection
// WHEN ResolveResume runs
// THEN it fails with a clear error rather than an index panic or a silent
// wrong choice.
func TestResolveResumePickerOutOfRangeSelection(t *testing.T) {
	s := session.NewStore(t.TempDir())
	newResumeTestSession(t, s, "a", "prompt a", time.Now().UnixMilli())
	newResumeTestSession(t, s, "b", "prompt b", time.Now().UnixMilli()+1000)

	_, err := ResolveResume(s, "", strings.NewReader("99\n"), &strings.Builder{})
	if err == nil {
		t.Fatal("ResolveResume with an out-of-range selection succeeded, want an error")
	}
}

// GIVEN multiple candidates and a non-numeric selection
// WHEN ResolveResume runs
// THEN it fails with a clear error.
func TestResolveResumePickerInvalidSelection(t *testing.T) {
	s := session.NewStore(t.TempDir())
	newResumeTestSession(t, s, "a", "prompt a", time.Now().UnixMilli())
	newResumeTestSession(t, s, "b", "prompt b", time.Now().UnixMilli()+1000)

	_, err := ResolveResume(s, "", strings.NewReader("not-a-number\n"), &strings.Builder{})
	if err == nil {
		t.Fatal("ResolveResume with a non-numeric selection succeeded, want an error")
	}
}

// GIVEN a last prompt longer than the display cap
// WHEN truncateForDisplay runs
// THEN it truncates and appends an ellipsis rather than overflowing the
// picker's one-line-per-session layout.
func TestTruncateForDisplay(t *testing.T) {
	long := strings.Repeat("x", 100)
	got := truncateForDisplay(long, 10)
	if len([]rune(got)) != 11 { // 10 chars + the ellipsis rune
		t.Errorf("truncateForDisplay length = %d, want 11", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncateForDisplay(%q) = %q, want it to end with an ellipsis", long, got)
	}

	short := "short prompt"
	if got := truncateForDisplay(short, 60); got != short {
		t.Errorf("truncateForDisplay(short) = %q, want unchanged %q", got, short)
	}
}
