package session

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lockFileName is the per-session concurrency guard, alongside session.json,
// transport.json, and attach-token. See
// docs/architecture/behavior/session_resume.feature.md §1.
const lockFileName = "session.lock"

// SessionLockedError reports that a session's lock is already held by
// another live process. Holder is the PID+timestamp line the current holder
// wrote when it acquired the lock (best-effort — read back verbatim, not
// parsed, since it exists purely for a legible error message, not as the
// enforcement mechanism).
type SessionLockedError struct {
	ID     string
	Holder string
}

func (e *SessionLockedError) Error() string {
	holder := e.Holder
	if holder == "" {
		holder = "unknown holder"
	}
	return fmt.Sprintf("session %s is already running (%s)", e.ID, holder)
}

// Unlocker releases a session lock. Mirrors config.Unlocker — same pattern,
// different (non-blocking) acquisition semantics; see Lock's doc comment.
type Unlocker struct {
	f *os.File
}

// Unlock releases the lock and closes the lock file. Safe to call once; a
// second call is a no-op.
func (u *Unlocker) Unlock() error {
	if u.f == nil {
		return nil
	}
	err := syscall.Flock(int(u.f.Fd()), syscall.LOCK_UN)
	closeErr := u.f.Close()
	u.f = nil
	if err != nil {
		return fmt.Errorf("unlock: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close lock file: %w", closeErr)
	}
	return nil
}

// Lock acquires session id's concurrency guard: an flock(2) advisory lock on
// sessions/<id>/session.lock, held for the caller's entire process lifetime
// (release via the returned Unlocker, typically in Shutdown). Every session
// acquires this — a fresh one via Create as much as a resumed one via Load —
// so a later resume attempt against a session that never itself locked can't
// silently collide with it.
//
// Unlike config.LockConfig (a blocking LOCK_EX — concurrent config writers
// should politely wait their turn), this is non-blocking (LOCK_EX|LOCK_NB):
// a resume attempt against a still-running session must fail immediately
// with a clear error, not hang until the other process exits.
//
// flock, not a bare PID liveness check, is the enforcement mechanism: a
// recorded PID alone is unsound here (PIDs are recycled by the OS, and
// "check PID, decide it's dead, then act" has a race window two concurrent
// resume attempts could both slip through). flock is atomic
// (kernel-arbitrated) and self-releasing (tied to the kernel's own
// open-file-descriptor table, so a crash, kill -9, or power loss releases it
// automatically with no cleanup code required to run correctly). The PID is
// still recorded in the lock file's content — purely for the error message
// a contended Lock call returns, not for enforcement.
//
// POSIX-only (Linux/macOS/BSD) — syscall.Flock has no Windows equivalent.
func (s *Store) Lock(id string) (*Unlocker, error) {
	path := filepath.Join(s.Dir(id), lockFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		defer f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			holder, _ := io.ReadAll(f)
			return nil, &SessionLockedError{ID: id, Holder: trimHolder(holder)}
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	if err := f.Truncate(0); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return nil, fmt.Errorf("truncate lock file %s: %w", path, err)
	}
	if _, err := f.WriteAt(fmt.Appendf(nil, "%d %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339)), 0); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return nil, fmt.Errorf("write lock file %s: %w", path, err)
	}
	return &Unlocker{f: f}, nil
}

func trimHolder(b []byte) string {
	s := string(b)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
