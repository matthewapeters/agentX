package runtime

import "testing"

// TestApprovalGateQueuesConcurrentRequests reproduces the vivid-raven deadlock
// mechanism directly: two requests enqueued while the first is still pending must
// resolve independently — the second must never orphan the first (the old shared-
// channel bug: arming a second request silently overwrote the first's only channel,
// leaving it blocked forever on a channel nothing would ever write to again).
func TestApprovalGateQueuesConcurrentRequests(t *testing.T) {
	g := &approvalGate{}
	req1 := &approvalRequest{resp: make(chan ApprovalDecision, 1)}
	req2 := &approvalRequest{resp: make(chan ApprovalDecision, 1)}

	if shown := g.enqueue(req1); !shown {
		t.Fatal("first request should be shown immediately (front of an empty queue)")
	}
	if shown := g.enqueue(req2); shown {
		t.Fatal("second concurrent request must queue, not show immediately")
	}

	if !g.deliver(DecisionSession) {
		t.Fatal("deliver found nothing to resolve")
	}
	select {
	case dec := <-req1.resp:
		if dec != DecisionSession {
			t.Errorf("req1 decision = %v, want DecisionSession", dec)
		}
	default:
		t.Fatal("req1 was never delivered to — this is the vivid-raven deadlock reproduced")
	}
	select {
	case <-req2.resp:
		t.Fatal("req2 must not receive a decision meant for req1")
	default:
	}

	// RequestApproval's deferred dequeue(req1) fires next, advancing the queue.
	next, ok := g.dequeue(req1)
	if !ok || next != req2 {
		t.Fatalf("dequeue(req1) = (%v, %v), want (req2, true)", next, ok)
	}

	if !g.deliver(DecisionDeny) {
		t.Fatal("deliver found nothing to resolve after advancing the queue")
	}
	select {
	case dec := <-req2.resp:
		if dec != DecisionDeny {
			t.Errorf("req2 decision = %v, want DecisionDeny", dec)
		}
	default:
		t.Fatal("req2 was never delivered to after becoming the front of the queue")
	}
}

// TestApprovalGateDequeueWhileWaiting: a request canceled/interrupted while still
// queued (never shown) is removed cleanly without disturbing the request currently
// shown, and correctly reports "no new front" since the shown request didn't change.
func TestApprovalGateDequeueWhileWaiting(t *testing.T) {
	g := &approvalGate{}
	req1 := &approvalRequest{resp: make(chan ApprovalDecision, 1)}
	req2 := &approvalRequest{resp: make(chan ApprovalDecision, 1)}
	g.enqueue(req1)
	g.enqueue(req2)

	if next, ok := g.dequeue(req2); ok {
		t.Fatalf("dequeue(req2) while req1 is still front should report no new front, got %v", next)
	}
	if !g.deliver(DecisionSession) {
		t.Fatal("req1 should still be deliverable after an unrelated queued request was removed")
	}
	select {
	case <-req1.resp:
	default:
		t.Fatal("req1 was not delivered to")
	}
}

// TestApprovalGateDeliverWithNothingPending: a stray Resolve with no request pending
// is a harmless no-op (matches the pre-fix behavior for this case).
func TestApprovalGateDeliverWithNothingPending(t *testing.T) {
	g := &approvalGate{}
	if g.deliver(DecisionSession) {
		t.Fatal("deliver on an empty queue should report false")
	}
}
