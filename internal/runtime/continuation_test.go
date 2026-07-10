package runtime

import "testing"

// TestVerbGateQueuesConcurrentRequests proves the generalized gate works identically
// for a second, distinct decision kind (verbDecision, not ApprovalDecision) — the
// same FIFO/per-request-channel mechanics TestApprovalGateQueuesConcurrentRequests
// already proves for tool approval, now exercised through the verb-approval
// instantiation.
func TestVerbGateQueuesConcurrentRequests(t *testing.T) {
	g := &verbApprovalGate{}
	req1 := newPendingRequest[verbPayload, verbDecision](verbPayload{verb: "revise"})
	req2 := newPendingRequest[verbPayload, verbDecision](verbPayload{verb: "restructure"})

	if shown := g.enqueue(req1); !shown {
		t.Fatal("first request should be shown immediately")
	}
	if shown := g.enqueue(req2); shown {
		t.Fatal("second concurrent request must queue, not show immediately")
	}

	if !g.deliver(VerbAllowOnce) {
		t.Fatal("deliver found nothing to resolve")
	}
	select {
	case dec := <-req1.resp:
		if dec != VerbAllowOnce {
			t.Errorf("req1 decision = %v, want VerbAllowOnce", dec)
		}
	default:
		t.Fatal("req1 was never delivered to")
	}
	select {
	case <-req2.resp:
		t.Fatal("req2 must not receive a decision meant for req1")
	default:
	}

	next, ok := g.dequeue(req1)
	if !ok || next != req2 {
		t.Fatalf("dequeue(req1) = (%v, %v), want (req2, true)", next, ok)
	}
	if !g.deliver(VerbDenyAlways) {
		t.Fatal("deliver found nothing to resolve after advancing the queue")
	}
	select {
	case dec := <-req2.resp:
		if dec != VerbDenyAlways {
			t.Errorf("req2 decision = %v, want VerbDenyAlways", dec)
		}
	default:
		t.Fatal("req2 was never delivered to after becoming the front of the queue")
	}
}

func TestResolveVerbMapsDecisionStrings(t *testing.T) {
	cases := []struct {
		decision string
		want     verbDecision
	}{
		{"allow_once", VerbAllowOnce},
		{"allow_always", VerbAllowAlways},
		{"deny_always", VerbDenyAlways},
		{"deny_once", VerbDenyOnce},
		{"anything else", VerbDenyOnce},
		{"", VerbDenyOnce},
	}
	for _, c := range cases {
		o := &Orchestrator{}
		req := newPendingRequest[verbPayload, verbDecision](verbPayload{verb: "x"})
		o.verbGate.enqueue(req)
		o.ResolveVerb(c.decision)
		select {
		case got := <-req.resp:
			if got != c.want {
				t.Errorf("ResolveVerb(%q) delivered %v, want %v", c.decision, got, c.want)
			}
		default:
			t.Errorf("ResolveVerb(%q) delivered nothing", c.decision)
		}
	}
}
