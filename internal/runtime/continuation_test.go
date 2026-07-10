package runtime

import (
	"context"
	"testing"
	"time"

	"agentx/internal/session"
	"agentx/internal/state"
)

// testOrchestrator builds a minimal Orchestrator sufficient to drive
// RequestApproval/RequestVerbApproval end to end (bus + processing publisher +
// identity wired, everything else zero-valued — AppendVerb no-ops on empty paths).
func testOrchestrator() *Orchestrator {
	return &Orchestrator{
		bus:  state.NewBus(),
		proc: state.NewProcessingPublisher("test"),
		id:   session.Identity{ID: "test"},
	}
}

// TestRequestVerbApprovalMapsDecisionStrings proves RequestVerbApproval — now a
// thin wrapper over the shared RequestDecision seam — still converts every raw
// decision string the surface can send back (via the generic Resolve) into the
// correct verbDecision, exactly as the old dedicated ResolveVerb switch did.
func TestRequestVerbApprovalMapsDecisionStrings(t *testing.T) {
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
		o := testOrchestrator()
		type result struct {
			dec verbDecision
			err error
		}
		done := make(chan result, 1)
		go func() {
			dec, err := o.RequestVerbApproval(context.Background(), "revise", "let me revise the approach")
			done <- result{dec, err}
		}()

		// Wait for the request to reach the front of the queue before resolving —
		// RequestVerbApproval enqueues asynchronously relative to this goroutine.
		for !o.gate.deliver(c.decision) {
			time.Sleep(time.Millisecond)
		}

		r := <-done
		if r.err != nil {
			t.Errorf("RequestVerbApproval(%q): unexpected error %v", c.decision, r.err)
			continue
		}
		if r.dec != c.want {
			t.Errorf("RequestVerbApproval(%q) = %v, want %v", c.decision, r.dec, c.want)
		}
	}
}
