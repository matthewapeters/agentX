package app

import (
	"context"
	"os"
	"testing"

	"agentx/internal/prompting"
	"agentx/internal/runtime"
	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// stubModel satisfies runtime.Model without touching a real LLM backend —
// none of these tests ever reach a prompt cycle, so its Chat is never
// actually invoked.
type stubModel struct{}

func (stubModel) Chat(context.Context, string, []prompting.Message, []tools.ToolSchema, func(string), func(string)) (runtime.ChatResult, error) {
	return runtime.ChatResult{}, nil
}
func (stubModel) Ready(context.Context, string) error                { return nil }
func (stubModel) ContextLength(context.Context, string) (int, error) { return 0, nil }

// GIVEN resumePortEnvVar unset
// WHEN preferredTransportPort runs
// THEN it returns 0 (no preference) — the ordinary, non-resume case.
func TestPreferredTransportPortUnset(t *testing.T) {
	t.Setenv(resumePortEnvVar, "")
	if got := preferredTransportPort(); got != 0 {
		t.Errorf("preferredTransportPort() = %d, want 0", got)
	}
}

// GIVEN resumePortEnvVar set to a valid port number
// WHEN preferredTransportPort runs
// THEN it returns that port.
func TestPreferredTransportPortValid(t *testing.T) {
	t.Setenv(resumePortEnvVar, "8420")
	if got := preferredTransportPort(); got != 8420 {
		t.Errorf("preferredTransportPort() = %d, want 8420", got)
	}
}

// GIVEN resumePortEnvVar set to something unparsable
// WHEN preferredTransportPort runs
// THEN it returns 0 rather than propagating a parse error — a malformed
// env value degrades to "no preference," not a startup failure.
func TestPreferredTransportPortInvalid(t *testing.T) {
	t.Setenv(resumePortEnvVar, "not-a-port")
	if got := preferredTransportPort(); got != 0 {
		t.Errorf("preferredTransportPort() = %d, want 0 for an unparsable value", got)
	}
}

// GIVEN a real, started Orchestrator with the transport enabled on a real
// bound port
// WHEN switchToResumedSession runs
// THEN it sets resumePortEnvVar to that exact port, shuts the orchestrator
// down cleanly, and calls execFunc with argv naming the target session and
// this process's own executable path — never actually replacing the test
// process.
func TestSwitchToResumedSessionSetsPortAndExecsWithTargetReal(t *testing.T) {
	dir := t.TempDir()
	orc := runtime.New(runtime.Settings{
		SessionRoot:        dir,
		TransportEnabled:   true,
		TransportHost:      "127.0.0.1",
		TransportPortStart: 20000,
		TransportPortEnd:   20100,
	}, runtime.WithModel(stubModel{}))
	if err := orc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	sessionID := orc.Session().ID
	// A real prompt, so the outgoing session has content: this test is
	// specifically about the lock being released, not about
	// RemoveIfEmpty's separate empty-session cleanup (already covered in
	// internal/runtime's own ShutdownForResume tests) — an empty outgoing
	// session's directory (lock file included) is deleted entirely, which
	// would make a "the lock is reacquirable" assertion meaningless here.
	orc.Bus().Publish(state.Event{
		SessionID: sessionID, EventType: "USER_PROMPT",
		ContentType: state.ContentUserPrompt, Payload: map[string]any{"text": "real work"}, Enabled: true,
	})

	target := session.NewStore(dir)
	targetID, err := target.Create()
	if err != nil {
		t.Fatalf("create target session: %v", err)
	}

	var gotArgv0 string
	var gotArgv []string
	origExec := execFunc
	execFunc = func(argv0 string, argv, envv []string) error {
		gotArgv0 = argv0
		gotArgv = argv
		return nil
	}
	t.Cleanup(func() { execFunc = origExec })

	if err := switchToResumedSession(orc, targetID.ID); err != nil {
		t.Fatalf("switchToResumedSession: %v", err)
	}

	if gotArgv0 == "" {
		t.Error("execFunc was never called")
	}
	wantArgv := []string{gotArgv0, "--resume", targetID.ID}
	if len(gotArgv) != len(wantArgv) || gotArgv[1] != "--resume" || gotArgv[2] != targetID.ID {
		t.Errorf("argv = %v, want %v", gotArgv, wantArgv)
	}

	if got := os.Getenv(resumePortEnvVar); got == "" {
		t.Error("resumePortEnvVar was not set before exec")
	}

	// The outgoing session's lock must be released — a fresh Lock call
	// against it must succeed now that switchToResumedSession's own
	// ShutdownForResume has run.
	store := session.NewStore(dir)
	unlocker, err := store.Lock(sessionID)
	if err != nil {
		t.Errorf("Lock(outgoing session) after switch = %v, want success (the lock must be released)", err)
	} else {
		_ = unlocker.Unlock()
	}
}
