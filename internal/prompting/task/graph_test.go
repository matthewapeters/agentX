package task

import (
	"encoding/json"
	"testing"
)

func TestAddAssignsMonotonicSeq(t *testing.T) {
	g := NewGraph()
	ids := []string{"a", "b", "c"}
	for i, id := range ids {
		// Deliberately set a bogus/inconsistent Seq on the input to prove Add
		// ignores it and assigns its own (ADR 0012 amendment).
		if err := g.Add(Record{ID: id, Seq: 999}); err != nil {
			t.Fatalf("Add(%s): %v", id, err)
		}
		got, _ := g.Node(id)
		if got.Seq != i {
			t.Errorf("node %s Seq = %d, want %d", id, got.Seq, i)
		}
	}
}

func TestUpdatePreservesSeq(t *testing.T) {
	g := NewGraph()
	if err := g.Add(Record{ID: "a", Status: Proposed}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	before, _ := g.Node("a")
	if before.Seq != 0 {
		t.Fatalf("precondition: Seq = %d, want 0", before.Seq)
	}

	// The common real-world shape: an Update call carrying a zero-value Seq,
	// since callers are not expected to set it at all.
	if err := g.Update(Record{ID: "a", Status: Done}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	after, _ := g.Node("a")
	if after.Seq != before.Seq {
		t.Errorf("Update changed Seq from %d to %d, want it preserved", before.Seq, after.Seq)
	}
	if after.Status != Done {
		t.Errorf("Status = %q, want Done (Update must still apply the rest of the record)", after.Status)
	}
}

func TestUpdatePreservesSeqAcrossMultipleUpdates(t *testing.T) {
	g := NewGraph()
	_ = g.Add(Record{ID: "a"})
	_ = g.Add(Record{ID: "b"})
	aBefore, _ := g.Node("a")
	bBefore, _ := g.Node("b")

	_ = g.Update(Record{ID: "a", Status: Ready})
	_ = g.Update(Record{ID: "b", Status: Ready})
	_ = g.Update(Record{ID: "a", Status: Done})

	aAfter, _ := g.Node("a")
	bAfter, _ := g.Node("b")
	if aAfter.Seq != aBefore.Seq {
		t.Errorf("a: Seq = %d after repeated updates, want %d", aAfter.Seq, aBefore.Seq)
	}
	if bAfter.Seq != bBefore.Seq {
		t.Errorf("b: Seq = %d after update, want %d", bAfter.Seq, bBefore.Seq)
	}
	if aAfter.Seq == bAfter.Seq {
		t.Errorf("a and b Seq both = %d, want distinct growth positions", aAfter.Seq)
	}
}

func TestValueAndErrorPassThroughUnvalidated(t *testing.T) {
	g := NewGraph()
	if err := g.Add(Record{ID: "a", Status: Done, Value: "42"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, _ := g.Node("a")
	if got.Value != "42" {
		t.Errorf("Value = %q, want %q", got.Value, "42")
	}

	if err := g.Update(Record{ID: "a", Status: Failed, Error: "boom"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = g.Node("a")
	if got.Error != "boom" {
		t.Errorf("Error = %q, want %q", got.Error, "boom")
	}
	// Graph does not police Value/Status pairing — it stores exactly what the
	// caller sets, same as every other field. A Value left over from a prior
	// status is the caller's responsibility to clear, not Graph's to enforce.
	if err := g.Update(Record{ID: "a", Status: Failed, Value: "stale but graph doesn't care", Error: "boom"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

// Scenario 1 (ADR 0014 Phase 1): a graph containing only KindTask/KindStep
// records (today's existing shape, no Hypothesis records at all) — Ready() must
// produce byte-for-byte identical output to what it returned before this phase.
func TestReadyWithExistingKinds(t *testing.T) {
	g := NewGraph()
	_ = g.Add(Record{ID: "a", Kind: KindTask, Status: Proposed})
	_ = g.Add(Record{ID: "b", Kind: KindStep, Status: Proposed})

	got := g.Ready()
	want := []string{"a", "b"}
	if len(got) != len(want) {
		t.Fatalf("Ready = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Ready[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Scenario 2 (ADR 0014 Phase 1): the hazard's regression test — a KindHypothesis
// record with Status: Proposed and no Deps (the exact shape that would make it
// schedulable under today's Ready() logic, if Kind weren't excluded) must NOT
// appear in Ready() output, regardless of its Status.
func TestReadyExcludesHypothesis(t *testing.T) {
	g := NewGraph()
	_ = g.Add(Record{ID: "h1", Kind: KindHypothesis, Status: Proposed})

	got := g.Ready()
	if len(got) != 0 {
		t.Errorf("Ready() = %v, want empty slice (KindHypothesis must be excluded)", got)
	}
}

// Scenario 3 (ADR 0014 Phase 1): a graph containing a mix of KindTask,
// KindStep, and KindHypothesis records, all with a Status/Deps shape that would
// otherwise qualify — Ready() must return only the KindTask/KindStep IDs, in the
// same first-seen order Ready() already guarantees.
func TestReadyMixedKinds(t *testing.T) {
	g := NewGraph()
	_ = g.Add(Record{ID: "a", Kind: KindTask, Status: Proposed})
	_ = g.Add(Record{ID: "h1", Kind: KindHypothesis, Status: Proposed})
	_ = g.Add(Record{ID: "b", Kind: KindStep, Status: Proposed})
	_ = g.Add(Record{ID: "h2", Kind: KindHypothesis, Status: Ready})
	_ = g.Add(Record{ID: "c", Kind: KindTask, Status: Proposed})

	got := g.Ready()
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("Ready = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Ready[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Scenario 4 (ADR 0014 Phase 1): a Record constructed with Likelihood/Evidence/
// ResolutionCriteria/Deferred populated must succeed through Add and Update —
// validate() never inspects Kind or these new fields (it only ever inspects
// rec.Deps and rec.ID).
func TestAddUpdateAcceptsNewFields(t *testing.T) {
	g := NewGraph()
	_ = g.Add(Record{ID: "a", Kind: KindTask})
	hyp := Record{
		ID:         "h1",
		Kind:       KindHypothesis,
		Status:     Proposed,
		Likelihood: LikelihoodMedium,
		Evidence:   []Evidence{{NodeID: "a", Stance: StanceSupports}},
		ResolutionCriteria: []ResolutionAssertion{
			{Text: "x", Outcome: OutcomeAbstained, Declared: "initial"},
		},
		Deferred: true,
	}
	if err := g.Add(hyp); err != nil {
		t.Fatalf("Add hypothesis: %v", err)
	}

	// Update: change Status and Likelihood, keep the new fields.
	if err := g.Update(Record{ID: "h1", Status: Done, Likelihood: LikelihoodLow}); err != nil {
		t.Fatalf("Update hypothesis: %v", err)
	}

	got, _ := g.Node("h1")
	if got.Status != Done {
		t.Errorf("Status = %q, want Done", got.Status)
	}
	if got.Likelihood != LikelihoodLow {
		t.Errorf("Likelihood = %q, want %q", got.Likelihood, LikelihoodLow)
	}
}

// Scenario 5 (ADR 0014 Phase 1): a Record with Kind == KindHypothesis and a
// populated Evidence list referencing another node's ID by NodeID — every new
// field must round-trip through JSON serialization (json tags use snake_case,
// matching every existing Record field's convention).
func TestNewFieldsJSONRoundTrip(t *testing.T) {
	r := Record{
		ID:         "h1",
		Kind:       KindHypothesis,
		Likelihood: LikelihoodMedium,
		Evidence:   []Evidence{{NodeID: "a", Stance: StanceRefutes}},
		ResolutionCriteria: []ResolutionAssertion{
			{Text: "find root cause", Outcome: OutcomeSatisfied, Declared: "initial"},
		},
		Deferred: true,
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Record
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Likelihood != r.Likelihood {
		t.Errorf("Likelihood round-trip = %q, want %q", got.Likelihood, r.Likelihood)
	}
	if len(got.Evidence) != 1 {
		t.Fatalf("Evidence len = %d, want 1", len(got.Evidence))
	}
	if got.Evidence[0].NodeID != "a" {
		t.Errorf("Evidence[0].NodeID = %q, want %q", got.Evidence[0].NodeID, "a")
	}
	if len(got.ResolutionCriteria) != 1 {
		t.Fatalf("ResolutionCriteria len = %d, want 1", len(got.ResolutionCriteria))
	}
	if got.ResolutionCriteria[0].Text != "find root cause" {
		t.Errorf("ResolutionCriteria[0].Text = %q, want %q", got.ResolutionCriteria[0].Text, "find root cause")
	}
	if !got.Deferred {
		t.Error("Deferred round-trip = false, want true")
	}
}
