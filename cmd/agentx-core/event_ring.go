package main

import (
	"sync"
	"time"
)

const defaultEventRingCapacity = 200

// LogEvent is a single timestamped lifecycle or bridge event stored in the ring buffer.
type LogEvent struct {
	At      time.Time `json:"at"`
	Message string    `json:"message"`
}

// EventRing is a fixed-capacity ring buffer of LogEvents.  All methods are safe
// for concurrent use.
type EventRing struct {
	mu       sync.RWMutex
	buf      []LogEvent
	capacity int
	head     int // index of the oldest entry (next overwrite target)
	count    int
}

// NewEventRing creates a ring buffer with the given capacity (min 1).
func NewEventRing(capacity int) *EventRing {
	if capacity < 1 {
		capacity = defaultEventRingCapacity
	}
	return &EventRing{
		buf:      make([]LogEvent, capacity),
		capacity: capacity,
	}
}

// Append adds a new event, overwriting the oldest when the buffer is full.
func (r *EventRing) Append(message string) {
	if message == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.head] = LogEvent{At: time.Now().UTC(), Message: message}
	r.head = (r.head + 1) % r.capacity
	if r.count < r.capacity {
		r.count++
	}
}

// Snapshot returns all events in chronological order.
func (r *EventRing) Snapshot() []LogEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.count == 0 {
		return nil
	}
	out := make([]LogEvent, r.count)
	if r.count < r.capacity {
		// Buffer not yet full: entries start at index 0.
		copy(out, r.buf[:r.count])
		return out
	}
	// Full: oldest entry is at r.head.
	n := copy(out, r.buf[r.head:])
	copy(out[n:], r.buf[:r.head])
	return out
}

// Since returns only events with At after the given time, in chronological order.
func (r *EventRing) Since(after time.Time) []LogEvent {
	all := r.Snapshot()
	for i, ev := range all {
		if ev.At.After(after) {
			return all[i:]
		}
	}
	return nil
}
