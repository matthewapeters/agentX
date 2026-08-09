package session

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
)

func newLockTestSession(t *testing.T) (*Store, string) {
	t.Helper()
	s := NewStore(t.TempDir())
	id, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return s, id.ID
}

// GIVEN a session with no existing lock
// WHEN Lock is called
// THEN it succeeds and writes the caller's PID into the lock file's content.
func TestLockSucceedsWhenUnheldAndRecordsPID(t *testing.T) {
	s, id := newLockTestSession(t)

	un, err := s.Lock(id)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	defer un.Unlock()

	data, err := os.ReadFile(s.Dir(id) + "/session.lock")
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		t.Fatalf("lock file content = %q, want at least a PID field", data)
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("lock file's first field = %q, want a parseable PID: %v", fields[0], err)
	}
	if pid != os.Getpid() {
		t.Errorf("recorded PID = %d, want %d", pid, os.Getpid())
	}
}

// GIVEN a session whose lock is already held
// WHEN a second Lock call is made for the same ID
// THEN it fails immediately (non-blocking) with a SessionLockedError
// carrying the first holder's recorded content — not a hang, not a generic
// error.
func TestLockContendedFailsImmediatelyWithHolderInfo(t *testing.T) {
	s, id := newLockTestSession(t)

	first, err := s.Lock(id)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}
	defer first.Unlock()

	_, err = s.Lock(id)
	if err == nil {
		t.Fatal("second Lock succeeded, want SessionLockedError")
	}
	var lockedErr *SessionLockedError
	if !errors.As(err, &lockedErr) {
		t.Fatalf("second Lock error = %v (%T), want *SessionLockedError", err, err)
	}
	if lockedErr.ID != id {
		t.Errorf("SessionLockedError.ID = %q, want %q", lockedErr.ID, id)
	}
	if !strings.Contains(lockedErr.Holder, strconv.Itoa(os.Getpid())) {
		t.Errorf("SessionLockedError.Holder = %q, want it to contain the holder's PID %d", lockedErr.Holder, os.Getpid())
	}
}

// GIVEN a lock held then released via Unlock
// WHEN a second Lock call is made for the same ID afterward
// THEN it succeeds — releasing genuinely frees the lock for reacquisition.
func TestLockReacquirableAfterUnlock(t *testing.T) {
	s, id := newLockTestSession(t)

	first, err := s.Lock(id)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}
	if err := first.Unlock(); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	second, err := s.Lock(id)
	if err != nil {
		t.Fatalf("second Lock after Unlock: %v", err)
	}
	defer second.Unlock()
}

// GIVEN a lock held by a file descriptor that is closed without calling
// Unlock (simulating a crash — no cleanup code runs)
// WHEN a second Lock call is made for the same ID
// THEN it still succeeds — flock's self-release is tied to the kernel's own
// open-file-descriptor table, not to application cleanup code running
// correctly, so a crash releases the lock automatically.
func TestLockSelfReleasesOnUnmanagedFileClose(t *testing.T) {
	s, id := newLockTestSession(t)

	first, err := s.Lock(id)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}
	// Simulate a crash: close the underlying fd directly, bypassing Unlock
	// (which would call syscall.Flock LOCK_UN explicitly — this test wants
	// to prove the lock releases even without that explicit call).
	if err := first.f.Close(); err != nil {
		t.Fatalf("close underlying fd: %v", err)
	}

	second, err := s.Lock(id)
	if err != nil {
		t.Fatalf("Lock after simulated crash = %v, want success (flock must self-release on fd close)", err)
	}
	defer second.Unlock()
}

// GIVEN a lock already released
// WHEN Unlock is called a second time
// THEN it is a no-op, not an error — callers that defer Unlock() after an
// earlier explicit Unlock() must not see a spurious failure.
func TestUnlockTwiceIsNoOp(t *testing.T) {
	s, id := newLockTestSession(t)

	un, err := s.Lock(id)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := un.Unlock(); err != nil {
		t.Fatalf("first Unlock: %v", err)
	}
	if err := un.Unlock(); err != nil {
		t.Errorf("second Unlock = %v, want nil (no-op)", err)
	}
}

// GIVEN a SessionLockedError
// WHEN Error() is called
// THEN it produces a legible message naming the session and the holder.
func TestSessionLockedErrorMessage(t *testing.T) {
	err := &SessionLockedError{ID: "abc123", Holder: "48213 2026-08-08T14:03:11Z"}
	msg := err.Error()
	if !strings.Contains(msg, "abc123") || !strings.Contains(msg, "48213") {
		t.Errorf("Error() = %q, want it to mention both the session ID and the holder", msg)
	}
}

// GIVEN a SessionLockedError with no recorded holder content (the lock file
// existed but was empty or unreadable)
// WHEN Error() is called
// THEN it falls back to a legible placeholder instead of an empty parenthetical.
func TestSessionLockedErrorMessageEmptyHolder(t *testing.T) {
	err := &SessionLockedError{ID: "abc123", Holder: ""}
	if !strings.Contains(err.Error(), "unknown holder") {
		t.Errorf("Error() = %q, want the empty-holder fallback text", err.Error())
	}
}

// GIVEN a session ID whose directory was never created
// WHEN Lock is called
// THEN it fails with a clear error (the lock file's parent directory does
// not exist) rather than silently creating one — Lock is never the thing
// that establishes a session's directory.
func TestLockFailsForNonexistentSessionDir(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Lock("never-created"); err == nil {
		t.Fatal("Lock succeeded for a session directory that was never created, want an error")
	}
}

// GIVEN a lock whose underlying file was already closed out from under the
// Unlocker (e.g. by something other than Unlock itself)
// WHEN Unlock is called
// THEN it returns an error instead of silently reporting success.
func TestUnlockSurfacesUnderlyingError(t *testing.T) {
	s, id := newLockTestSession(t)
	un, err := s.Lock(id)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := un.f.Close(); err != nil {
		t.Fatalf("closing underlying fd directly: %v", err)
	}
	if err := un.Unlock(); err == nil {
		t.Error("Unlock() on an already-closed fd = nil, want an error")
	}
}
