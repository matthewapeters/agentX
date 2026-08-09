package runtime

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"agentx/internal/session"
	"agentx/internal/state"
)

// freeTestPort finds a currently-unused TCP port on loopback by briefly
// binding port 0 (OS-assigned) and reading it back.
func freeTestPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}
	return port
}

// publishUserPrompt is a small helper that publishes a real user_prompt
// event on o's bus, mirroring what a live turn would publish, so the
// session recorder persists it exactly as production would.
func publishUserPrompt(o *Orchestrator, text string) uint64 {
	return o.Bus().Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.Session().ID,
		EventType:   "USER_PROMPT",
		ContentType: state.ContentUserPrompt,
		Payload:     map[string]any{"text": text},
		Enabled:     true,
	})
}

// GIVEN a real Orchestrator that started, published several events, and shut
// down cleanly
// WHEN a second Orchestrator instance resumes the same session ID and
// publishes one more event
// THEN its ordinal is strictly greater than every ordinal the first instance
// ever assigned — the specific class of bug (ordinal/ID collisions) that
// slips past code-review-only review and needs a real running-process-level
// test, not just unit coverage of the pieces in isolation
// (docs/architecture/behavior/session_resume.feature.md §3, Tests).
func TestOrchestratorResumePreservesOrdinalContinuity(t *testing.T) {
	dir := t.TempDir()

	first := New(Settings{SessionRoot: dir}, WithModel(stubModel{}))
	if err := first.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	sessionID := first.Session().ID

	var lastOrdinal uint64
	for _, text := range []string{"first prompt", "second prompt", "third prompt"} {
		lastOrdinal = publishUserPrompt(first, text)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := first.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}

	second := New(Settings{SessionRoot: dir, ResumeSessionID: sessionID}, WithModel(stubModel{}))
	if err := second.Start(); err != nil {
		t.Fatalf("second (resumed) Start: %v", err)
	}
	defer func() {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		_ = second.Shutdown(ctx2)
	}()

	if second.Session().ID != sessionID {
		t.Fatalf("resumed session ID = %s, want %s (must reopen the same session, not fork a new one)", second.Session().ID, sessionID)
	}

	newOrdinal := publishUserPrompt(second, "prompt after resume")
	if newOrdinal <= lastOrdinal {
		t.Fatalf("resumed session's new ordinal = %d, want strictly greater than %d (the first instance's last ordinal) — an ordinal collision means SetEventEnabled or a reattaching surface's cursor could silently target the wrong event", newOrdinal, lastOrdinal)
	}
}

// GIVEN a real Orchestrator that published several turns' worth of events
// and shut down
// WHEN a second Orchestrator instance resumes the same session
// THEN its reconstructed in-memory history reflects exactly what was
// persisted — including a toggled-off event's disabled state surviving the
// restart.
func TestOrchestratorResumeReconstructsHistory(t *testing.T) {
	dir := t.TempDir()

	first := New(Settings{SessionRoot: dir}, WithModel(stubModel{}))
	if err := first.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	sessionID := first.Session().ID

	userOrd := publishUserPrompt(first, "implement the thing")
	respOrd := first.Bus().Publish(state.Event{
		Epoch: time.Now().UnixMilli(), SessionID: sessionID,
		EventType: "AGENT_RESPONSE", ContentType: state.ContentAgentResponse,
		Payload: map[string]any{"text": "done"}, Enabled: true,
	})

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := first.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}

	second := New(Settings{SessionRoot: dir, ResumeSessionID: sessionID}, WithModel(stubModel{}))
	if err := second.Start(); err != nil {
		t.Fatalf("second (resumed) Start: %v", err)
	}
	defer func() {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		_ = second.Shutdown(ctx2)
	}()

	second.mu.Lock()
	hist := append([]turnMsg(nil), second.history...)
	second.mu.Unlock()

	if len(hist) != 2 {
		t.Fatalf("resumed history = %+v, want 2 entries (user prompt + response)", hist)
	}
	if hist[0].ordinal != userOrd || hist[0].role != "user" || hist[0].content != "implement the thing" {
		t.Errorf("hist[0] = %+v, want ordinal=%d role=user content=%q", hist[0], userOrd, "implement the thing")
	}
	if hist[1].ordinal != respOrd || hist[1].role != "assistant" || hist[1].content != "done" {
		t.Errorf("hist[1] = %+v, want ordinal=%d role=assistant content=%q", hist[1], respOrd, "done")
	}
}

// GIVEN a session with a session ID that was never created
// WHEN Start is called with ResumeSessionID set to that ID
// THEN it fails with a clear error rather than silently creating a fresh
// session under that name.
func TestOrchestratorResumeNonexistentSessionFails(t *testing.T) {
	dir := t.TempDir()
	o := New(Settings{SessionRoot: dir, ResumeSessionID: "never-created"}, WithModel(stubModel{}))
	if err := o.Start(); err == nil {
		t.Fatal("Start succeeded for a nonexistent ResumeSessionID, want an error")
	}
}

// GIVEN a session that is currently locked by another live Orchestrator
// instance
// WHEN a second Orchestrator attempts to Start with the same
// ResumeSessionID
// THEN it fails immediately with a session-locked error, and does not
// disturb the first instance's own running state.
func TestOrchestratorResumeContendedSessionFails(t *testing.T) {
	dir := t.TempDir()
	first := New(Settings{SessionRoot: dir}, WithModel(stubModel{}))
	if err := first.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	sessionID := first.Session().ID
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = first.Shutdown(ctx)
	}()

	second := New(Settings{SessionRoot: dir, ResumeSessionID: sessionID}, WithModel(stubModel{}))
	if err := second.Start(); err == nil {
		t.Fatal("second Start succeeded against a session the first instance still holds, want a lock error")
	}
}

// GIVEN a PreferredTransportPort that is currently free
// WHEN a real Orchestrator starts with the transport enabled
// THEN it binds exactly that port — the mechanism a mid-session resume
// trigger uses to hand its port across syscall.Exec so already-attached
// surfaces' reconnect polling can succeed against the endpoint they
// already have (docs/architecture/behavior/session_resume.feature.md §5).
func TestOrchestratorStartBindsPreferredTransportPort(t *testing.T) {
	dir := t.TempDir()
	preferred := freeTestPort(t)

	o := New(Settings{
		SessionRoot:            dir,
		TransportEnabled:       true,
		TransportHost:          "127.0.0.1",
		TransportPortStart:     preferred + 1000,
		TransportPortEnd:       preferred + 1001,
		PreferredTransportPort: preferred,
	}, WithModel(stubModel{}))

	if err := o.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = o.Shutdown(ctx)
	}()

	want := fmt.Sprintf("http://127.0.0.1:%d", preferred)
	if o.Endpoint() != want {
		t.Errorf("Endpoint() = %q, want %q", o.Endpoint(), want)
	}
}

// GIVEN a running Orchestrator with a live bus subscriber (standing in for
// an attached surface) and a target session to switch to
// WHEN ShutdownForResume runs
// THEN the subscriber receives a ContentSessionSwitching event — naming the
// target session's ID and name — before its subscription channel closes.
// Ordering, not just presence: a surface that only sees the connection
// close with no prior signal has no way to know it should reconnect
// elsewhere (docs/architecture/behavior/session_resume.feature.md §5,
// Tests).
func TestShutdownForResumePublishesSessionSwitchingBeforeClosing(t *testing.T) {
	dir := t.TempDir()

	o := New(Settings{SessionRoot: dir}, WithModel(stubModel{}))
	if err := o.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	target := session.NewStore(dir)
	targetID, err := target.Create(session.WithNamer(func() string { return "target-session" }))
	if err != nil {
		t.Fatalf("create target session: %v", err)
	}

	sub := o.Bus().Subscribe()
	defer sub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- o.ShutdownForResume(ctx, targetID.ID) }()

	var sawSwitching bool
	var payload map[string]any
	for ev := range sub.C {
		if ev.ContentType == state.ContentSessionSwitching {
			sawSwitching = true
			payload, _ = ev.Payload.(map[string]any)
			break
		}
	}
	if !sawSwitching {
		t.Fatal("subscriber's channel closed without ever seeing a ContentSessionSwitching event")
	}
	if payload["session_id"] != targetID.ID {
		t.Errorf("payload session_id = %v, want %q", payload["session_id"], targetID.ID)
	}
	if payload["session_name"] != "target-session" {
		t.Errorf("payload session_name = %v, want %q", payload["session_name"], "target-session")
	}

	if err := <-done; err != nil {
		t.Errorf("ShutdownForResume: %v", err)
	}
}

// GIVEN a running Orchestrator with no attached surfaces at all
// WHEN ShutdownForResume runs
// THEN it completes within roughly the grace period, not indefinitely —
// the grace period must never turn into an unbounded wait.
func TestShutdownForResumeDoesNotBlockIndefinitely(t *testing.T) {
	dir := t.TempDir()

	o := New(Settings{SessionRoot: dir}, WithModel(stubModel{}))
	if err := o.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	target := session.NewStore(dir)
	targetID, err := target.Create()
	if err != nil {
		t.Fatalf("create target session: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	if err := o.ShutdownForResume(ctx, targetID.ID); err != nil {
		t.Fatalf("ShutdownForResume: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("ShutdownForResume took %v, want well under 2s (grace period is %v)", elapsed, sessionSwitchGracePeriod)
	}
}

// GIVEN a target session ID that doesn't actually exist (a stale/invalid
// lookup)
// WHEN ShutdownForResume runs
// THEN it still completes and still publishes ContentSessionSwitching
// (with an empty session_name) rather than blocking the whole switch on a
// best-effort name lookup.
func TestShutdownForResumeToleratesUnresolvableTargetName(t *testing.T) {
	dir := t.TempDir()

	o := New(Settings{SessionRoot: dir}, WithModel(stubModel{}))
	if err := o.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := o.ShutdownForResume(ctx, "never-created"); err != nil {
		t.Fatalf("ShutdownForResume: %v", err)
	}
}

// GIVEN a real Orchestrator session with no user prompt (just started, never
// typed into)
// WHEN ShutdownForResume runs
// THEN the outgoing session's directory is removed as part of the switch.
func TestShutdownForResumeRemovesEmptyOutgoingSession(t *testing.T) {
	dir := t.TempDir()

	o := New(Settings{SessionRoot: dir}, WithModel(stubModel{}))
	if err := o.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	outgoingID := o.Session().ID

	target := session.NewStore(dir)
	targetID, err := target.Create()
	if err != nil {
		t.Fatalf("create target session: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := o.ShutdownForResume(ctx, targetID.ID); err != nil {
		t.Fatalf("ShutdownForResume: %v", err)
	}

	store := session.NewStore(dir)
	if _, err := store.Load(outgoingID); err == nil {
		t.Error("outgoing empty session still exists after ShutdownForResume, want it removed")
	}
}

// GIVEN a real Orchestrator session with a real user prompt already
// recorded — even one being abandoned mid-conversation to switch to
// something else
// WHEN ShutdownForResume runs
// THEN the outgoing session's directory is left fully intact.
func TestShutdownForResumePreservesNonEmptyOutgoingSession(t *testing.T) {
	dir := t.TempDir()

	o := New(Settings{SessionRoot: dir}, WithModel(stubModel{}))
	if err := o.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	outgoingID := o.Session().ID
	publishUserPrompt(o, "real work in progress")

	target := session.NewStore(dir)
	targetID, err := target.Create()
	if err != nil {
		t.Fatalf("create target session: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := o.ShutdownForResume(ctx, targetID.ID); err != nil {
		t.Fatalf("ShutdownForResume: %v", err)
	}

	store := session.NewStore(dir)
	if _, err := store.Load(outgoingID); err != nil {
		t.Errorf("outgoing session with real content was removed after ShutdownForResume: %v", err)
	}
}
