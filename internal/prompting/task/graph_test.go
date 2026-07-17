package task

import "testing"

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
