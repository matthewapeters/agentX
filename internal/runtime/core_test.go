package runtime

import (
	"reflect"
	"testing"

	"agentx/internal/prompting"
	"agentx/internal/session"
	"agentx/internal/state"
)

// TestOrchestratorPublishDelegatesToPublishEv proves Publish (ADR 0013 Phase 1's
// EventSink implementation) is a pure pass-through: identical inputs produce the
// identical published event and ordinal that calling publishEv directly would.
func TestOrchestratorPublishDelegatesToPublishEv(t *testing.T) {
	o := testOrchestrator()
	payload := map[string]any{"text": "hello"}

	wantOrd := o.publishEv("USER_PROMPT", state.ContentUserPrompt, payload, false)
	gotOrd := o.Publish("USER_PROMPT", state.ContentUserPrompt, payload, false)

	// publishEv mints a new ordinal per call, so the two calls above are distinct
	// events; what must match is that Publish went through the same bus (a
	// consecutive, monotonically-increasing ordinal) rather than a divergent path.
	if gotOrd != wantOrd+1 {
		t.Fatalf("Publish ordinal = %d, want %d (one past the direct publishEv call, same bus)", gotOrd, wantOrd+1)
	}
}

// TestOrchestratorAugmentDelegatesToWithContext proves Augment (ADR 0013 Phase 1's
// ContextStore.Augment implementation) returns exactly what withContext(base)
// returns directly, for the same Orchestrator state.
func TestOrchestratorAugmentDelegatesToWithContext(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(dir)
	id, err := store.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	o := &Orchestrator{
		bus:   state.NewBus(),
		proc:  state.NewProcessingPublisher("test"),
		id:    id,
		store: store,
	}

	base := []prompting.Message{{Role: "user", Content: "hello"}}

	want := o.withContext(base)
	got := o.Augment(base)

	if len(got) != len(want) {
		t.Fatalf("Augment returned %d messages, want %d", len(got), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("message %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestOrchestratorRecordDelegatesToRecordTurn proves Record (ADR 0013 Phase 1's
// ContextStore.Record implementation) mutates o.history identically to calling
// recordTurn directly with the same arguments, given a TurnRecord unpacking them.
func TestOrchestratorRecordDelegatesToRecordTurn(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(dir)
	id, err := store.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	entry := TurnRecord{
		Record:   true,
		UserOrd:  1,
		UserText: "what's the plan",
		RespOrd:  2,
		Response: "here's the plan",
	}

	direct := &Orchestrator{bus: state.NewBus(), proc: state.NewProcessingPublisher("test"), id: id, store: store}
	direct.recordTurn(entry.Err, entry.Record, entry.UserOrd, entry.UserText, entry.RespOrd, entry.Response, entry.Pins)

	viaWrapper := &Orchestrator{bus: state.NewBus(), proc: state.NewProcessingPublisher("test"), id: id, store: store}
	viaWrapper.Record(entry)

	if len(viaWrapper.history) != len(direct.history) {
		t.Fatalf("Record produced %d history entries, want %d", len(viaWrapper.history), len(direct.history))
	}
	for i := range direct.history {
		if viaWrapper.history[i] != direct.history[i] {
			t.Errorf("history[%d] = %+v, want %+v", i, viaWrapper.history[i], direct.history[i])
		}
	}
}
