package runtime

import (
	"context"
	"testing"
	"time"

	"agentx/internal/state"
)

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
