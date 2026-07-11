package runtime

import (
	"testing"

	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// testOrchestratorWithStore builds a minimal Orchestrator sufficient to drive
// mutateWorkingMemory/persistReadGrantIfNeeded/liveReadGrants end to end, backed by a
// real session.Store rooted in a temp directory (SaveWorkingMemory creates the session
// dir on demand, so no explicit store.Create call is needed).
func testOrchestratorWithStore(t *testing.T) *Orchestrator {
	t.Helper()
	return &Orchestrator{
		bus:   state.NewBus(),
		proc:  state.NewProcessingPublisher("test"),
		id:    session.Identity{ID: "test"},
		store: session.NewStore(t.TempDir()),
	}
}

// TestPersistReadGrantIfNeededGrantsOutOfRootRead reproduces the witty-falcon fix
// directly: an approval granted specifically because a RiskRead call's path escaped the
// confinement root persists as a working-memory read grant, so a standing grant — not
// just tools.Policy's exact-args allowlist — covers later calls under that path.
func TestPersistReadGrantIfNeededGrantsOutOfRootRead(t *testing.T) {
	o := testOrchestratorWithStore(t)
	d := tools.Descriptor{ID: "list_dir", Risk: tools.RiskRead}

	o.persistReadGrantIfNeeded(d, map[string]string{"path": "/Projects/agentX"}, "outside working directory")

	wm, err := o.store.LoadWorkingMemory(o.id.ID)
	if err != nil {
		t.Fatalf("LoadWorkingMemory: %v", err)
	}
	if !(session.WMReadGrants{WM: wm}).Allows("/Projects/agentX") {
		t.Error("path was not granted after persistReadGrantIfNeeded")
	}
}

// TestPersistReadGrantIfNeededSkipsOtherReasons: an approval granted for any reason OTHER
// than escaping root (e.g. a normal RequiresApproval call within root) must not persist a
// read grant — this seam only exists to suppress escapesRoot's own override.
func TestPersistReadGrantIfNeededSkipsOtherReasons(t *testing.T) {
	o := testOrchestratorWithStore(t)
	d := tools.Descriptor{ID: "list_dir", Risk: tools.RiskRead}

	o.persistReadGrantIfNeeded(d, map[string]string{"path": "/Projects/agentX"}, "approval_required")

	wm, err := o.store.LoadWorkingMemory(o.id.ID)
	if err != nil {
		t.Fatalf("LoadWorkingMemory: %v", err)
	}
	if len(session.PermittedReadPaths(wm)) != 0 {
		t.Errorf("PermittedReadPaths = %v, want none persisted for a non-root-escape reason", session.PermittedReadPaths(wm))
	}
}

// TestPersistReadGrantIfNeededSkipsWriteRisk: a write/network tool must never gain a
// standing grant this way, even if it somehow carried the escape reason — WithReadGrants'
// own contract is "never permits a mutating call".
func TestPersistReadGrantIfNeededSkipsWriteRisk(t *testing.T) {
	o := testOrchestratorWithStore(t)
	d := tools.Descriptor{ID: "write_file", Risk: tools.RiskWrite}

	o.persistReadGrantIfNeeded(d, map[string]string{"path": "/Projects/agentX/out.txt"}, "outside working directory")

	wm, err := o.store.LoadWorkingMemory(o.id.ID)
	if err != nil {
		t.Fatalf("LoadWorkingMemory: %v", err)
	}
	if len(session.PermittedReadPaths(wm)) != 0 {
		t.Errorf("PermittedReadPaths = %v, want none persisted for a RiskWrite tool", session.PermittedReadPaths(wm))
	}
}

// TestLiveReadGrantsReloadsOnEveryCheck proves the "live" half of liveReadGrants: a grant
// persisted AFTER the ReadGrants value was constructed is still honored on the very next
// check, because it reloads working memory each call rather than snapshotting it once.
func TestLiveReadGrantsReloadsOnEveryCheck(t *testing.T) {
	o := testOrchestratorWithStore(t)
	g := liveReadGrants{store: o.store, sessionID: o.id.ID}

	if g.Allows("/Projects/agentX") {
		t.Fatal("Allows returned true before any grant was persisted")
	}

	o.persistReadGrantIfNeeded(tools.Descriptor{ID: "list_dir", Risk: tools.RiskRead},
		map[string]string{"path": "/Projects/agentX"}, "outside working directory")

	if !g.Allows("/Projects/agentX") {
		t.Error("Allows returned false after a grant was persisted — liveReadGrants did not reload")
	}
	if !g.Allows("/Projects/agentX/docs") {
		t.Error("Allows returned false for a subpath of a granted directory")
	}
	if g.Allows("/etc/shadow") {
		t.Error("Allows returned true for an unrelated, ungranted path")
	}
}
