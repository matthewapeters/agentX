package main

import (
	"fmt"
	"sync"
)

// AppletPortAllocator hands out sequential TCP ports within a configured range.
// Each applet receives exactly one port from which it binds its render/API
// listener.  The core's own address is stored separately and passed to applets
// via AGENTX_CORE_HTTP so they can query application state.
//
// Thread-safe: multiple goroutines may call Next() concurrently.
type AppletPortAllocator struct {
	mu      sync.Mutex
	next    int
	rangeStart int
	rangeEnd   int
}

// NewAppletPortAllocator creates an allocator for the inclusive [start, end] range.
// Returns an error if the range is invalid (start < 1, end < start).
func NewAppletPortAllocator(start, end int) (*AppletPortAllocator, error) {
	if start < 1 || end < start {
		return nil, fmt.Errorf(
			"invalid applet port range [%d, %d]: start must be >= 1 and end >= start",
			start, end,
		)
	}
	return &AppletPortAllocator{
		next:       start,
		rangeStart: start,
		rangeEnd:   end,
	}, nil
}

// Next allocates the next available port in the range.
// Returns an error when the range is exhausted.
func (a *AppletPortAllocator) Next() (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.next > a.rangeEnd {
		return 0, fmt.Errorf(
			"applet port range [%d, %d] exhausted (all %d ports in use)",
			a.rangeStart, a.rangeEnd, a.rangeEnd-a.rangeStart+1,
		)
	}
	port := a.next
	a.next++
	return port, nil
}

// Remaining returns the number of ports still available.
func (a *AppletPortAllocator) Remaining() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	remaining := a.rangeEnd - a.next + 1
	if remaining < 0 {
		return 0
	}
	return remaining
}
