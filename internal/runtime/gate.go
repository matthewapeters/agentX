package runtime

import (
	"sync"
	"time"
)

// pendingRequest is one request awaiting a decision, queued in a gate. Each request
// owns its own response channel — the fix for the concurrent-approval deadlock
// (session vivid-raven): the scheduler can dispatch several leaves that each request a
// decision on their own goroutine, and a single shared channel gets silently
// overwritten by whichever request arms it last, permanently orphaning every earlier
// one (blocked forever on a channel nothing will ever write to again — a lost wakeup,
// not a timeout). A channel per request makes that structurally impossible: delivery
// always targets the exact request it was meant for.
type pendingRequest[Req any, Resp any] struct {
	payload Req
	resp    chan Resp // size-1, owned solely by this request
	// enqueuedAt is when this request was created — "a decision became
	// necessary," not "this became the front of the queue and visible."
	// Immutable once set; the surface uses it to show how long a pending
	// decision has really been waiting (docs/architecture/behavior/
	// chat_pending_approval_duration.feature.md).
	enqueuedAt time.Time
}

func newPendingRequest[Req any, Resp any](payload Req) *pendingRequest[Req, Resp] {
	return &pendingRequest[Req, Resp]{payload: payload, resp: make(chan Resp, 1), enqueuedAt: time.Now()}
}

// gate serializes concurrent requests of type Req into a FIFO queue and shows exactly
// one at a time to the surface — "orchestrate approvals being displayed and responded
// to: one request at a time" — rather than racing several announcements against each
// other. Generalized (2026-07-10) from the tool-approval gate so a second, distinct
// kind of interactive decision (verb-continuation approval) can reuse the identical,
// already-proven queueing mechanics without duplicating them or coupling the two
// decision kinds together — each instantiates gate[Req, Resp] with its own payload
// and decision types.
type gate[Req any, Resp any] struct {
	mu      sync.Mutex
	pending []*pendingRequest[Req, Resp] // pending[0], if any, is the one currently shown
}

// enqueue adds req to the queue and reports whether it is now at the front (i.e.
// should be shown immediately rather than waiting for earlier requests to resolve).
func (g *gate[Req, Resp]) enqueue(req *pendingRequest[Req, Resp]) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pending = append(g.pending, req)
	return len(g.pending) == 1
}

// dequeue removes req from the queue (wherever it is — it may still be waiting in
// line, e.g. if its ctx was canceled before ever being shown) and reports the new
// front-of-queue request to show next, if the queue is non-empty and its front
// changed.
func (g *gate[Req, Resp]) dequeue(req *pendingRequest[Req, Resp]) (next *pendingRequest[Req, Resp], ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	wasFront := len(g.pending) > 0 && g.pending[0] == req
	for i, r := range g.pending {
		if r == req {
			g.pending = append(g.pending[:i], g.pending[i+1:]...)
			break
		}
	}
	if wasFront && len(g.pending) > 0 {
		return g.pending[0], true
	}
	return nil, false
}

// Len reports the current queue depth, including the front-of-queue request
// currently shown to the surface. Read-only — callers use it to tell the
// surface how many decisions are pending, not just that one is.
func (g *gate[Req, Resp]) Len() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.pending)
}

// Queued returns every payload currently in the queue, in FIFO order,
// including the one at the front (currently shown) — the batch-preview list
// threaded onto APPROVAL_REQUEST events (docs/architecture/behavior/
// chat_pending_approval_batch_view.feature.md). Callers that only want
// what's waiting BEHIND the front slice off index 0 themselves; the gate has
// no opinion about what "front" means to a caller, same as Len().
func (g *gate[Req, Resp]) Queued() []Req {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]Req, len(g.pending))
	for i, r := range g.pending {
		out[i] = r.payload
	}
	return out
}

// deliver resolves the currently-shown (front-of-queue) request only — the surface
// only ever answers the one request it was shown, so there is never ambiguity about
// which request a decision belongs to.
func (g *gate[Req, Resp]) deliver(d Resp) bool {
	g.mu.Lock()
	var ch chan Resp
	if len(g.pending) > 0 {
		ch = g.pending[0].resp
	}
	g.mu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- d:
		return true
	default:
		return false
	}
}
