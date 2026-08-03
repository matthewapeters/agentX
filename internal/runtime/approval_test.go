package runtime

import "testing"

// TestDecisionGateQueuesConcurrentRequests reproduces the vivid-raven deadlock
// mechanism directly: two requests enqueued while the first is still pending must
// resolve independently — the second must never orphan the first (the old shared-
// channel bug: arming a second request silently overwrote the first's only channel,
// leaving it blocked forever on a channel nothing would ever write to again). Uses
// the shared cross-kind decisionGate/approvalUIRequest/string shape directly, since
// tool-approval and verb-continuation approval are now the same gate instantiation.
func TestDecisionGateQueuesConcurrentRequests(t *testing.T) {
	g := &decisionGate{}
	req1 := newPendingRequest[approvalUIRequest, string](approvalUIRequest{prompt: "rm -rf /tmp/x"})
	req2 := newPendingRequest[approvalUIRequest, string](approvalUIRequest{prompt: "continue investigating?"})

	if shown := g.enqueue(req1); !shown {
		t.Fatal("first request should be shown immediately (front of an empty queue)")
	}
	if shown := g.enqueue(req2); shown {
		t.Fatal("second concurrent request must queue, not show immediately")
	}

	if !g.deliver("session") {
		t.Fatal("deliver found nothing to resolve")
	}
	select {
	case dec := <-req1.resp:
		if dec != "session" {
			t.Errorf("req1 decision = %v, want %q", dec, "session")
		}
	default:
		t.Fatal("req1 was never delivered to — this is the vivid-raven deadlock reproduced")
	}
	select {
	case <-req2.resp:
		t.Fatal("req2 must not receive a decision meant for req1")
	default:
	}

	// RequestDecision's deferred dequeue(req1) fires next, advancing the queue.
	next, ok := g.dequeue(req1)
	if !ok || next != req2 {
		t.Fatalf("dequeue(req1) = (%v, %v), want (req2, true)", next, ok)
	}

	if !g.deliver("deny") {
		t.Fatal("deliver found nothing to resolve after advancing the queue")
	}
	select {
	case dec := <-req2.resp:
		if dec != "deny" {
			t.Errorf("req2 decision = %v, want %q", dec, "deny")
		}
	default:
		t.Fatal("req2 was never delivered to after becoming the front of the queue")
	}
}

// TestDecisionGateSerializesAcrossKinds proves the point of collapsing the former
// separate approvalGate/verbApprovalGate into one shared decisionGate: a
// tool-approval-shaped request and a verb-continuation-shaped request enqueued
// concurrently still serialize strictly one-at-a-time, even though nothing about the
// gate distinguishes "kind" anymore — it's the same struct either way. Previously
// there was no cross-kind serialization guarantee at all (two separate gates, two
// separate mutexes).
func TestDecisionGateSerializesAcrossKinds(t *testing.T) {
	g := &decisionGate{}
	toolReq := newPendingRequest[approvalUIRequest, string](approvalUIRequest{prompt: "run tool X?", options: toolApprovalOptions})
	verbReq := newPendingRequest[approvalUIRequest, string](approvalUIRequest{prompt: "continue investigating?", options: verbApprovalOptions})

	if shown := g.enqueue(toolReq); !shown {
		t.Fatal("tool request should be shown immediately (front of an empty queue)")
	}
	if shown := g.enqueue(verbReq); shown {
		t.Fatal("verb request arriving while the tool request is pending must queue, not show immediately")
	}

	if !g.deliver("global") {
		t.Fatal("deliver found nothing to resolve for the tool request")
	}
	select {
	case dec := <-toolReq.resp:
		if dec != "global" {
			t.Errorf("tool request decision = %v, want %q", dec, "global")
		}
	default:
		t.Fatal("tool request was never delivered to")
	}
	select {
	case <-verbReq.resp:
		t.Fatal("verb request must not receive a decision meant for the tool request")
	default:
	}

	next, ok := g.dequeue(toolReq)
	if !ok || next != verbReq {
		t.Fatalf("dequeue(toolReq) = (%v, %v), want (verbReq, true)", next, ok)
	}
	if !g.deliver("allow_always") {
		t.Fatal("deliver found nothing to resolve for the verb request")
	}
	select {
	case dec := <-verbReq.resp:
		if dec != "allow_always" {
			t.Errorf("verb request decision = %v, want %q", dec, "allow_always")
		}
	default:
		t.Fatal("verb request was never delivered to after becoming the front of the queue")
	}
}

// TestDecisionGateDequeueWhileWaiting: a request canceled/interrupted while still
// queued (never shown) is removed cleanly without disturbing the request currently
// shown, and correctly reports "no new front" since the shown request didn't change.
func TestDecisionGateDequeueWhileWaiting(t *testing.T) {
	g := &decisionGate{}
	req1 := newPendingRequest[approvalUIRequest, string](approvalUIRequest{})
	req2 := newPendingRequest[approvalUIRequest, string](approvalUIRequest{})
	g.enqueue(req1)
	g.enqueue(req2)

	if next, ok := g.dequeue(req2); ok {
		t.Fatalf("dequeue(req2) while req1 is still front should report no new front, got %v", next)
	}
	if !g.deliver("session") {
		t.Fatal("req1 should still be deliverable after an unrelated queued request was removed")
	}
	select {
	case <-req1.resp:
	default:
		t.Fatal("req1 was not delivered to")
	}
}

// TestDecisionGateDeliverWithNothingPending: a stray Resolve with no request pending
// is a harmless no-op (matches the pre-fix behavior for this case).
func TestDecisionGateDeliverWithNothingPending(t *testing.T) {
	g := &decisionGate{}
	if g.deliver("session") {
		t.Fatal("deliver on an empty queue should report false")
	}
}

// TestGateLenReflectsQueueDepth: Len() tracks the queue depth through
// enqueue/dequeue, including the front-of-queue request currently shown —
// the count publishApprovalPrompt threads into the APPROVAL_REQUEST event
// so the surface can show "1 of N" instead of leaving later-queued decisions
// invisible until resolved one at a time.
func TestGateLenReflectsQueueDepth(t *testing.T) {
	g := &decisionGate{}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() on empty gate = %d, want 0", got)
	}

	req1 := newPendingRequest[approvalUIRequest, string](approvalUIRequest{prompt: "first"})
	req2 := newPendingRequest[approvalUIRequest, string](approvalUIRequest{prompt: "second"})
	req3 := newPendingRequest[approvalUIRequest, string](approvalUIRequest{prompt: "third"})
	g.enqueue(req1)
	if got := g.Len(); got != 1 {
		t.Errorf("Len() after 1 enqueue = %d, want 1", got)
	}
	g.enqueue(req2)
	g.enqueue(req3)
	if got := g.Len(); got != 3 {
		t.Errorf("Len() after 3 enqueues = %d, want 3", got)
	}

	g.dequeue(req1)
	if got := g.Len(); got != 2 {
		t.Errorf("Len() after dequeuing the front = %d, want 2", got)
	}
}

// TestPublishApprovalPromptCarriesPendingCount: the APPROVAL_REQUEST event's
// payload carries whatever pending count the caller passes, unchanged — the
// piece RequestDecision wires to o.gate.Len() at each of its two call sites.
func TestPublishApprovalPromptCarriesPendingCount(t *testing.T) {
	o := testOrchestrator()
	sub := o.bus.Subscribe()
	defer sub.Close()

	o.publishApprovalPrompt("run write_file?", toolApprovalOptions, 3)

	ev := <-sub.C
	if ev.EventType != "APPROVAL_REQUEST" {
		t.Fatalf("event type = %q, want APPROVAL_REQUEST", ev.EventType)
	}
	p, ok := ev.Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload = %T, want map[string]any", ev.Payload)
	}
	if got, ok := p["pending"].(int); !ok || got != 3 {
		t.Errorf("payload[\"pending\"] = %v (%T), want 3", p["pending"], p["pending"])
	}
}
