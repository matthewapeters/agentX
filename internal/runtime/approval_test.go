package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentx/internal/tools"
)

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
// payload carries whatever pending count, since timestamp, and queued list
// the caller passes, unchanged — the piece RequestDecision wires to
// o.gate.Len(), the request's own enqueuedAt, and queuedPrompts(o.gate.Queued())
// at each of its two call sites.
func TestPublishApprovalPromptCarriesPendingCount(t *testing.T) {
	o := testOrchestrator()
	sub := o.bus.Subscribe()
	defer sub.Close()

	since := time.Now().Add(-90 * time.Second)
	o.publishApprovalPrompt("run write_file?", toolApprovalOptions, 3, since, []string{"run http_get?", "run read_file?"})

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
	if got, ok := p["since"].(int64); !ok || got != since.UnixMilli() {
		t.Errorf("payload[\"since\"] = %v (%T), want %d", p["since"], p["since"], since.UnixMilli())
	}
	if got, ok := p["queued"].([]string); !ok || len(got) != 2 || got[0] != "run http_get?" || got[1] != "run read_file?" {
		t.Errorf("payload[\"queued\"] = %v (%T), want [run http_get? run read_file?]", p["queued"], p["queued"])
	}
}

// TestQueuedPromptsExcludesFront: queuedPrompts reports every request's
// prompt EXCEPT the one at index 0 (the front, already shown as the main
// prompt) — nil for an empty or single-element queue, so the common
// "nothing else pending" case carries no batch-preview data at all.
func TestQueuedPromptsExcludesFront(t *testing.T) {
	if got := queuedPrompts(nil); got != nil {
		t.Errorf("queuedPrompts(nil) = %v, want nil", got)
	}
	one := []approvalUIRequest{{prompt: "front"}}
	if got := queuedPrompts(one); got != nil {
		t.Errorf("queuedPrompts(1 request) = %v, want nil", got)
	}
	three := []approvalUIRequest{{prompt: "front"}, {prompt: "second"}, {prompt: "third"}}
	got := queuedPrompts(three)
	if len(got) != 2 || got[0] != "second" || got[1] != "third" {
		t.Errorf("queuedPrompts(3 requests) = %v, want [second third]", got)
	}
}

// TestGateQueuedReflectsFIFOOrder: Queued() returns every payload in FIFO
// order, including the front — the source queuedPrompts reads to build the
// batch preview.
func TestGateQueuedReflectsFIFOOrder(t *testing.T) {
	g := &decisionGate{}
	if got := g.Queued(); len(got) != 0 {
		t.Fatalf("Queued() on empty gate = %v, want empty", got)
	}

	req1 := newPendingRequest[approvalUIRequest, string](approvalUIRequest{prompt: "first"})
	req2 := newPendingRequest[approvalUIRequest, string](approvalUIRequest{prompt: "second"})
	g.enqueue(req1)
	g.enqueue(req2)

	got := g.Queued()
	if len(got) != 2 || got[0].prompt != "first" || got[1].prompt != "second" {
		t.Fatalf("Queued() = %v, want [first second]", got)
	}
}

// TestNewPendingRequestStampsEnqueuedAt: enqueuedAt is set at construction to
// roughly time.Now(), not left zero-valued — the field the pending-duration
// display depends on entirely.
func TestNewPendingRequestStampsEnqueuedAt(t *testing.T) {
	before := time.Now()
	req := newPendingRequest[approvalUIRequest, string](approvalUIRequest{prompt: "x"})
	after := time.Now()

	if req.enqueuedAt.Before(before) || req.enqueuedAt.After(after) {
		t.Errorf("enqueuedAt = %v, want between %v and %v", req.enqueuedAt, before, after)
	}
}

// TestToolApprovalOptionsForIncludesPlanOnlyWithRoot: the plan-scoped option
// (docs/architecture/behavior/tool_policy_plan_scoped_approval.feature.md)
// only appears when the proposed call is actually part of a plan — offering
// it outside one would let a user "approve for a plan" that doesn't exist.
func TestToolApprovalOptionsForIncludesPlanOnlyWithRoot(t *testing.T) {
	for _, opt := range toolApprovalOptionsFor("") {
		if opt.Decision == "plan" {
			t.Fatal(`toolApprovalOptionsFor("") offers a "plan" option, want none outside a plan`)
		}
	}

	found := false
	for _, opt := range toolApprovalOptionsFor("root-1") {
		if opt.Decision == "plan" {
			found = true
		}
	}
	if !found {
		t.Fatal(`toolApprovalOptionsFor("root-1") does not offer a "plan" option, want one inside a plan`)
	}
}

// TestRequestApprovalPlanDecisionScopesToRoot: resolving a plan-scoped call
// with "plan" allows it and records the approval under exactly the root
// RequestApproval was called with — not session or global scope, and not
// visible under a different root (a later, unrelated plan reusing the same
// tool+args must still prompt).
func TestRequestApprovalPlanDecisionScopesToRoot(t *testing.T) {
	o := testOrchestrator()
	pol := tools.NewPolicy()
	d, ok := tools.DefaultRegistry().Lookup("write_file")
	if !ok {
		t.Fatal("write_file not found in default registry")
	}
	args := map[string]string{"path": "notes.txt", "content": "hello"}

	type result struct {
		verdict tools.Verdict
		err     error
	}
	done := make(chan result, 1)
	go func() {
		v, err := o.RequestApproval(context.Background(), d, args, pol, "root-1", "")
		done <- result{v, err}
	}()

	for !o.gate.deliver("plan") {
		time.Sleep(time.Millisecond)
	}
	r := <-done

	if r.err != nil || r.verdict.Decision != tools.Allow {
		t.Fatalf("RequestApproval(root-1, plan) = (%+v, %v), want Allow, nil", r.verdict, r.err)
	}
	if !pol.PlanApproved("root-1", d, args, "") {
		t.Error("PlanApproved(root-1) = false, want true after a \"plan\" decision")
	}
	if pol.PlanApproved("root-2", d, args, "") {
		t.Error("PlanApproved(root-2) = true, want false — approval must not leak across plan roots")
	}
	if v := pol.Evaluate(d, args, ""); v.Decision != tools.NeedsApproval {
		t.Errorf("Evaluate() after a plan-only approval = %v, want NeedsApproval — plan scope is deliberately not folded into Evaluate", v.Decision)
	}
}

// TestRequestApprovalGlobalDecisionReusesScopeWithoutReprompting drives a
// "global" decision on an in-project .go edit through the real approval
// gate (RequestApproval -> Policy.Approve -> persisted TOML), then confirms
// a second, different .go file inside the same project evaluates to Allow
// without a second prompt — the end-to-end version of the extension-scoped
// reuse internal/tools/policy_test.go proves at the Policy level directly.
func TestRequestApprovalGlobalDecisionReusesScopeWithoutReprompting(t *testing.T) {
	o := testOrchestrator()
	dir := t.TempDir()
	o.settings.ToolApprovalsPath = filepath.Join(dir, "agentx-tool-approvals.toml")

	pol := tools.NewPolicy()
	d, ok := tools.DefaultRegistry().Lookup("write_file")
	if !ok {
		t.Fatal("write_file not found in default registry")
	}
	root := filepath.Join(dir, "repo")
	firstArgs := map[string]string{"path": filepath.Join(root, "a.go"), "content": "package a"}

	type result struct {
		verdict tools.Verdict
		err     error
	}
	done := make(chan result, 1)
	go func() {
		v, err := o.RequestApproval(context.Background(), d, firstArgs, pol, "", root)
		done <- result{v, err}
	}()

	for !o.gate.deliver("global") {
		time.Sleep(time.Millisecond)
	}
	r := <-done
	if r.err != nil || r.verdict.Decision != tools.Allow {
		t.Fatalf("RequestApproval(global) = (%+v, %v), want Allow, nil", r.verdict, r.err)
	}

	if _, err := os.Stat(o.settings.ToolApprovalsPath); err != nil {
		t.Fatalf("global approval was not persisted to %s: %v", o.settings.ToolApprovalsPath, err)
	}

	secondArgs := map[string]string{"path": filepath.Join(root, "b.go"), "content": "package b"}
	if v := pol.Evaluate(d, secondArgs, root); v.Decision != tools.Allow {
		t.Errorf("Evaluate(different .go file, same project) = %v, want Allow — extension-scoped reuse", v.Decision)
	}

	otherExt := map[string]string{"path": filepath.Join(root, "README.md"), "content": "# hi"}
	if v := pol.Evaluate(d, otherExt, root); v.Decision != tools.NeedsApproval {
		t.Errorf("Evaluate(different extension, same project) = %v, want NeedsApproval", v.Decision)
	}
}

// TestProposalTextTruncatesArgvSubstitutedValues: an Argv-templated
// descriptor (apply_patch's diff argument is exactly this shape — edit_file
// was this test's original motivating example but is now a Go builtin, no
// Argv template) truncates a long substituted argument value identically to
// the k=v fallback path — previously only the fallback path truncated,
// letting an unbounded value (e.g. a large diff) reach the approval prompt
// verbatim (docs/architecture/behavior/approval_prompt_length_bound.feature.md).
func TestProposalTextTruncatesArgvSubstitutedValues(t *testing.T) {
	d := tools.Descriptor{
		ID:   "apply_patch",
		Argv: []string{"patch", "-p0", "{patch}"},
		Args: []tools.ArgSpec{{Name: "patch", Kind: tools.KindString}},
	}
	longScript := strings.Repeat("x", 500)
	args := map[string]string{"patch": longScript}

	got := proposalText(d, args)
	if strings.Contains(got, longScript) {
		t.Fatal("proposalText included the full untruncated script — the exact overflow this fix closes")
	}
	wantTruncated := longScript[:maxArgPreviewLen] + "…"
	if !strings.Contains(got, wantTruncated) {
		t.Errorf("proposalText = %q, want it to contain the truncated form %q", got, wantTruncated)
	}
}

// TestProposalTextArgvBranchMatchesFallbackTruncation: both proposalText
// rendering paths (Argv-templated and the k=v fallback) truncate an
// oversized value to the same length — no path is allowed to be the
// unbounded one.
func TestProposalTextArgvBranchMatchesFallbackTruncation(t *testing.T) {
	longVal := strings.Repeat("y", 500)
	want := longVal[:maxArgPreviewLen] + "…"

	argvBased := tools.Descriptor{ID: "http_get", Argv: []string{"curl", "-sSL", "--", "{url}"},
		Args: []tools.ArgSpec{{Name: "url", Kind: tools.KindString}}}
	if got := proposalText(argvBased, map[string]string{"url": longVal}); !strings.Contains(got, want) {
		t.Errorf("Argv-based proposalText = %q, want it to contain %q", got, want)
	}

	fallback := tools.Descriptor{ID: "write_file"}
	if got := proposalText(fallback, map[string]string{"content": longVal}); !strings.Contains(got, want) {
		t.Errorf("fallback proposalText = %q, want it to contain %q", got, want)
	}
}
