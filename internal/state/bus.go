package state

import (
	"sync"
	"sync/atomic"
)

// Bus fans Events out to every active Subscription. Each subscription has its
// own ordered, unbounded queue drained by a dedicated goroutine, so a slow
// consumer never blocks the publisher or other subscribers.
type Bus struct {
	mu      sync.Mutex
	subs    map[int]*Subscription
	next    int
	ordinal atomic.Uint64
}

// NewBus returns an empty Bus.
func NewBus() *Bus { return &Bus{subs: make(map[int]*Subscription)} }

// NewBusFrom returns an empty Bus whose ordinal counter starts past
// startOrdinal — the first Publish call stamps startOrdinal+1, not 1. This
// exists for session resume
// (docs/architecture/behavior/session_resume.feature.md §3): a resumed
// session's new process must never assign an ordinal that collides with one
// already on disk from the session's earlier run, since ordinal is the exact
// key SetEventEnabled matches against and the cursor a reattaching surface
// seeds its live-stream boundary from. Callers seed startOrdinal from the
// max Ordinal found in the session's persisted event log.
func NewBusFrom(startOrdinal uint64) *Bus {
	b := &Bus{subs: make(map[int]*Subscription)}
	b.ordinal.Store(startOrdinal)
	return b
}

// CurrentOrdinal returns the highest ordinal assigned so far (0 before any
// publish). A surface captures this at subscribe time as the boundary between the
// disk-seeded history (ordinal <= boundary) and the live stream (ordinal > boundary).
func (b *Bus) CurrentOrdinal() uint64 { return b.ordinal.Load() }

// Subscribe attaches a new subscriber and returns its Subscription. Callers read
// from Subscription.C and must call Close when done.
func (b *Bus) Subscribe() *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := &Subscription{
		bus:    b,
		id:     b.next,
		C:      make(chan Event),
		notify: make(chan struct{}, 1),
	}
	b.subs[b.next] = s
	b.next++
	go s.pump()
	return s
}

// Publish stamps ev with the next monotonic ordinal and enqueues it to every
// current subscriber in subscription order. Stamping at publish (rather than at
// disk write) means the live event and its persisted copy share one identity.
func (b *Bus) Publish(ev Event) uint64 {
	ev.Ordinal = b.ordinal.Add(1)
	b.mu.Lock()
	subs := make([]*Subscription, 0, len(b.subs))
	for _, s := range b.subs {
		subs = append(subs, s)
	}
	b.mu.Unlock()
	for _, s := range subs {
		s.enqueue(ev)
	}
	return ev.Ordinal
}

// Subscription delivers events to one consumer in published order via C.
type Subscription struct {
	bus    *Bus
	id     int
	C      chan Event
	notify chan struct{}

	mu     sync.Mutex
	buf    []Event
	closed bool
}

func (s *Subscription) enqueue(ev Event) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.buf = append(s.buf, ev)
	s.mu.Unlock()
	s.signal()
}

func (s *Subscription) signal() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *Subscription) pump() {
	for {
		s.mu.Lock()
		for len(s.buf) == 0 && !s.closed {
			s.mu.Unlock()
			<-s.notify
			s.mu.Lock()
		}
		if len(s.buf) == 0 && s.closed {
			s.mu.Unlock()
			close(s.C)
			return
		}
		ev := s.buf[0]
		s.buf = s.buf[1:]
		s.mu.Unlock()

		s.C <- ev
	}
}

// Close detaches the subscription. Its channel C is closed once any queued
// events have drained.
func (s *Subscription) Close() {
	s.bus.mu.Lock()
	delete(s.bus.subs, s.id)
	s.bus.mu.Unlock()

	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.signal()
}
