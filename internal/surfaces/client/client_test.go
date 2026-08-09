package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"

	"agentx/internal/session"
	"agentx/internal/state"
	transporthttp "agentx/internal/transport/http"
)

// fakeReconnectSurface is a minimal SurfaceModel double for exercising Host's
// reconnect state machine directly (no Bubble Tea program, no real network
// for the non-integration cases below).
type fakeReconnectSurface struct {
	applied    []state.Event
	resetCount int
}

func (f *fakeReconnectSurface) Apply(ev state.Event)        { f.applied = append(f.applied, ev) }
func (f *fakeReconnectSurface) SetSize(int, int)            {}
func (f *fakeReconnectSurface) Key(tea.KeyPressMsg) tea.Cmd { return nil }
func (f *fakeReconnectSurface) View() string                { return "" }
func (f *fakeReconnectSurface) CapturesKeys() bool          { return false }
func (f *fakeReconnectSurface) Reset() {
	f.resetCount++
	f.applied = nil
}

// eventServer serves a single session's seed + live SSE stream, backing a
// real transporthttp.Client round trip so attemptReconnect exercises its
// actual HTTP path rather than a hand-built message.
func eventServer(t *testing.T, seed []state.Event) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions/current/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(seed)
	})
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// writeSessionTransport writes a session directory's transport.json and
// attach-token so ReadTransport/ReadAttachToken (the disk-resolution path
// attemptReconnect uses) can find it. It creates the session directory
// itself (WriteTransport/WriteAttachToken expect it to already exist, as it
// would after a real Store.Create) rather than routing through Create,
// which assigns its own id — these tests want fixed, readable ids like
// "target-session" to assert against.
func writeSessionTransport(t *testing.T, root, id, endpoint string) {
	t.Helper()
	store := session.NewStore(root)
	if err := os.MkdirAll(store.Dir(id), 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	if err := store.WriteTransport(id, session.TransportInfo{SessionID: id, Endpoint: endpoint}); err != nil {
		t.Fatalf("WriteTransport: %v", err)
	}
	if err := store.WriteAttachToken(id, "tok-"+id); err != nil {
		t.Fatalf("WriteAttachToken: %v", err)
	}
}

// GIVEN a Host that received a ContentSessionSwitching event naming a
// different session
// WHEN applyTrackingSwitch folds that event
// THEN switchTarget is set to the named session and the event itself is
// never handed to the surface — it carries no rendering meaning
// (docs/architecture/behavior/session_resume.feature.md §5).
func TestApplyTrackingSwitchSetsTargetWithoutRenderingEvent(t *testing.T) {
	surf := &fakeReconnectSurface{}
	h := NewHost(surf, "t", nil, nil, nil, "/root", "old-session", "surf-1")

	h.applyTrackingSwitch(state.Event{
		ContentType: state.ContentSessionSwitching,
		Payload:     map[string]any{"session_id": "new-session", "session_name": "n"},
	})

	if h.switchTarget != "new-session" {
		t.Errorf("switchTarget = %q, want %q", h.switchTarget, "new-session")
	}
	if len(surf.applied) != 0 {
		t.Errorf("surface.Apply was called %d times, want 0 for a ContentSessionSwitching event", len(surf.applied))
	}
}

// GIVEN an ordinary (non-switching) event
// WHEN applyTrackingSwitch folds it
// THEN it reaches the surface unchanged and switchTarget stays empty.
func TestApplyTrackingSwitchFoldsOrdinaryEventIntoSurface(t *testing.T) {
	surf := &fakeReconnectSurface{}
	h := NewHost(surf, "t", nil, nil, nil, "/root", "s", "surf-1")

	ev := state.Event{ContentType: state.ContentUserPrompt, Payload: map[string]any{"text": "hi"}}
	h.applyTrackingSwitch(ev)

	if h.switchTarget != "" {
		t.Errorf("switchTarget = %q, want empty", h.switchTarget)
	}
	if len(surf.applied) != 1 {
		t.Fatalf("surface applied %d events, want 1", len(surf.applied))
	}
}

// GIVEN a Host whose stream just closed and a target session with a real,
// reachable transport endpoint
// WHEN Update processes streamClosedMsg followed by the resulting
// reconnectedMsg
// THEN it resets the surface exactly once, adopts the new session ID,
// clears switchTarget, and invokes ConnectionUpdater — the full successful
// reconnect path (docs/architecture/behavior/session_resume.feature.md §5).
func TestUpdateStreamClosedThenSuccessfulReconnectResetsSurface(t *testing.T) {
	dir := t.TempDir()
	seed := []state.Event{{Ordinal: 1, ContentType: state.ContentUserPrompt, Payload: map[string]any{"text": "seeded"}}}
	srv := eventServer(t, seed)
	writeSessionTransport(t, dir, "target-session", srv.URL)

	surf := &fakeReconnectSurface{}
	h := NewHost(surf, "t", nil, make(chan state.Event), nil, dir, "target-session", "surf-1")

	m, cmd := h.Update(streamClosedMsg{})
	h = m.(Host)
	if cmd == nil {
		t.Fatal("Update(streamClosedMsg) returned no command")
	}
	msg := cmd()
	rc, ok := msg.(reconnectedMsg)
	if !ok {
		t.Fatalf("attemptReconnect produced %T, want reconnectedMsg", msg)
	}
	if rc.err != nil {
		t.Fatalf("reconnectedMsg.err = %v, want nil", rc.err)
	}
	if rc.sessionID != "target-session" {
		t.Errorf("reconnectedMsg.sessionID = %q, want %q", rc.sessionID, "target-session")
	}

	m, cmd = h.Update(rc)
	h = m.(Host)
	if surf.resetCount != 1 {
		t.Errorf("surface.Reset called %d times, want 1", surf.resetCount)
	}
	if h.sessionID != "target-session" {
		t.Errorf("h.sessionID = %q, want %q", h.sessionID, "target-session")
	}
	if h.switchTarget != "" {
		t.Errorf("h.switchTarget = %q, want empty after a successful reconnect", h.switchTarget)
	}
	if len(surf.applied) != 1 {
		t.Errorf("surface applied %d seed events after reconnect, want 1", len(surf.applied))
	}
	if cmd == nil {
		t.Error("Update(reconnectedMsg) returned no follow-up listen command")
	}
}

// connUpdaterSurface is a SurfaceModel that also implements ConnectionUpdater
// — the optional capability Host type-asserts for after a successful
// reconnect (docs/architecture/behavior/session_resume.feature.md §5).
type connUpdaterSurface struct {
	*fakeReconnectSurface
	gotToken string
	calls    int
}

func (c *connUpdaterSurface) UpdateConnection(cl *transporthttp.Client, token string) {
	c.calls++
	c.gotToken = token
}

// GIVEN a Host with a ConnectionUpdater-capable surface
// WHEN a successful reconnectedMsg arrives
// THEN UpdateConnection is called with the newly resolved client and token
// — otherwise the surface's own mutation POSTs would keep silently
// targeting the pre-reconnect session
// (docs/architecture/behavior/session_resume.feature.md §5).
func TestUpdateReconnectedMsgInvokesConnectionUpdater(t *testing.T) {
	surf := &connUpdaterSurface{fakeReconnectSurface: &fakeReconnectSurface{}}

	dir := t.TempDir()
	srv := eventServer(t, nil)
	writeSessionTransport(t, dir, "s1", srv.URL)

	h := NewHost(surf, "t", nil, make(chan state.Event), nil, dir, "s1", "surf-1")
	msg := h.attemptReconnect()()
	rc := msg.(reconnectedMsg)
	if rc.err != nil {
		t.Fatalf("attemptReconnect: %v", rc.err)
	}
	if rc.token != "tok-s1" {
		t.Errorf("reconnectedMsg.token = %q, want %q", rc.token, "tok-s1")
	}

	h.Update(rc)
	if surf.calls != 1 {
		t.Errorf("UpdateConnection called %d times, want 1", surf.calls)
	}
	if surf.gotToken != "tok-s1" {
		t.Errorf("UpdateConnection token = %q, want %q", surf.gotToken, "tok-s1")
	}
}

// GIVEN a Host stuck failing to reconnect
// WHEN reconnectedMsg carrying an error arrives repeatedly
// THEN it retries up to maxReconnectAttempts times and then quits, rather
// than polling forever against a permanently gone session
// (docs/architecture/behavior/session_resume.feature.md §5).
func TestUpdateReconnectedMsgErrorGivesUpAfterMaxAttempts(t *testing.T) {
	surf := &fakeReconnectSurface{}
	h := NewHost(surf, "t", nil, nil, nil, "/root", "s", "surf-1")

	// Each non-final attempt's command is a tea.Tick that sleeps for
	// reconnectPollInterval before firing — asserting only h.reconnectAttempt
	// (an unexported field, reachable from this in-package test) instead of
	// invoking every intermediate command keeps this test from taking
	// maxReconnectAttempts*reconnectPollInterval to run.
	failErr := reconnectedMsg{err: fmt.Errorf("boom")}
	for i := range maxReconnectAttempts - 1 {
		m, cmd := h.Update(failErr)
		h = m.(Host)
		if cmd == nil {
			t.Fatalf("attempt %d: Update returned no command", i)
		}
		if h.reconnectAttempt != i+1 {
			t.Fatalf("attempt %d: reconnectAttempt = %d, want %d", i, h.reconnectAttempt, i+1)
		}
	}

	m, cmd := h.Update(failErr)
	h = m.(Host)
	if cmd == nil {
		t.Fatal("final attempt: Update returned no command")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Errorf("after %d failed attempts, want tea.Quit, reconnectAttempt=%d", maxReconnectAttempts, h.reconnectAttempt)
	}
}

// GIVEN a session whose transport.json initially points at an unreachable
// endpoint, then is rewritten (by a new process) to point at a real one
// WHEN attemptReconnect is called twice, once per state
// THEN the second call succeeds — proving each attempt re-reads the
// endpoint from disk rather than caching the first-resolved value
// (docs/architecture/behavior/session_resume.feature.md §5, Tests).
func TestAttemptReconnectReresolvesEndpointFromDiskEachAttempt(t *testing.T) {
	dir := t.TempDir()
	writeSessionTransport(t, dir, "s1", "http://127.0.0.1:1") // nothing listens here

	surf := &fakeReconnectSurface{}
	h := NewHost(surf, "t", nil, nil, nil, dir, "s1", "surf-1")

	first := h.attemptReconnect()().(reconnectedMsg)
	if first.err == nil {
		t.Fatal("first attemptReconnect against an unreachable endpoint unexpectedly succeeded")
	}

	srv := eventServer(t, nil)
	writeSessionTransport(t, dir, "s1", srv.URL)

	second := h.attemptReconnect()().(reconnectedMsg)
	if second.err != nil {
		t.Fatalf("second attemptReconnect after rewriting transport.json: %v", second.err)
	}
}

// GIVEN a Host with a pending switchTarget different from its current
// sessionID
// WHEN attemptReconnect runs
// THEN it resolves against switchTarget, not the old sessionID — a
// deliberate session switch, not a bare-disconnect retry of the same
// session (docs/architecture/behavior/session_resume.feature.md §5).
func TestAttemptReconnectPrefersSwitchTargetOverCurrentSessionID(t *testing.T) {
	dir := t.TempDir()
	srv := eventServer(t, nil)
	writeSessionTransport(t, dir, "new-session", srv.URL)
	// old-session deliberately has no transport.json: if attemptReconnect
	// ever resolved against it instead, this would fail loudly.

	surf := &fakeReconnectSurface{}
	h := NewHost(surf, "t", nil, nil, nil, dir, "old-session", "surf-1")
	h.switchTarget = "new-session"

	msg := h.attemptReconnect()().(reconnectedMsg)
	if msg.err != nil {
		t.Fatalf("attemptReconnect: %v", msg.err)
	}
	if msg.sessionID != "new-session" {
		t.Errorf("sessionID = %q, want %q", msg.sessionID, "new-session")
	}
}

// GIVEN a Host with no pending switchTarget
// WHEN attemptReconnect runs
// THEN it resolves against the session it was already attached to — the
// bare-disconnect fallback (a crash, not a deliberate switch, still
// recovers automatically).
func TestAttemptReconnectFallsBackToCurrentSessionIDWhenNoSwitchPending(t *testing.T) {
	dir := t.TempDir()
	srv := eventServer(t, nil)
	writeSessionTransport(t, dir, "same-session", srv.URL)

	surf := &fakeReconnectSurface{}
	h := NewHost(surf, "t", nil, nil, nil, dir, "same-session", "surf-1")

	msg := h.attemptReconnect()().(reconnectedMsg)
	if msg.err != nil {
		t.Fatalf("attemptReconnect: %v", msg.err)
	}
	if msg.sessionID != "same-session" {
		t.Errorf("sessionID = %q, want %q", msg.sessionID, "same-session")
	}
}

// GIVEN a Host built with no known session root (a launch-time root
// resolution failure, or a Host predating reconnection support)
// WHEN its stream closes
// THEN it quits immediately rather than attempting to reconnect — the
// original, pre-reconnect behavior preserved exactly.
func TestStreamClosedWithNoSessionRootQuitsImmediately(t *testing.T) {
	surf := &fakeReconnectSurface{}
	h := NewHost(surf, "t", nil, nil, nil, "", "s", "surf-1")

	m, cmd := h.Update(streamClosedMsg{})
	_ = m.(Host)
	if cmd == nil {
		t.Fatal("Update(streamClosedMsg) returned no command")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Error("want tea.Quit when sessionRoot is empty")
	}
}
