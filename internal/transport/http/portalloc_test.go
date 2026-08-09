package http

import (
	"net"
	"strconv"
	"testing"
)

// freePort finds a currently-unused TCP port on 127.0.0.1 by briefly binding
// port 0 (OS-assigned) and reading it back.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}
	return port
}

// GIVEN a preferred port that is currently free
// WHEN AllocatePreferred runs
// THEN it binds exactly that port, not one from the fallback range.
func TestAllocatePreferredBindsFreePreferredPort(t *testing.T) {
	preferred := freePort(t)

	ln, err := AllocatePreferred("127.0.0.1", preferred, preferred+1000, preferred+1001)
	if err != nil {
		t.Fatalf("AllocatePreferred: %v", err)
	}
	defer ln.Close()

	got := ln.Addr().(*net.TCPAddr).Port
	if got != preferred {
		t.Errorf("bound port = %d, want the preferred port %d", got, preferred)
	}
}

// GIVEN a preferred port that is currently occupied by another listener
// WHEN AllocatePreferred runs
// THEN it falls back to the [start, end] range scan instead of failing.
func TestAllocatePreferredFallsBackWhenPreferredTaken(t *testing.T) {
	occupied := freePort(t)
	blocker, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(occupied))
	if err != nil {
		t.Fatalf("occupy port %d: %v", occupied, err)
	}
	defer blocker.Close()

	fallback := freePort(t)
	ln, err := AllocatePreferred("127.0.0.1", occupied, fallback, fallback+10)
	if err != nil {
		t.Fatalf("AllocatePreferred: %v", err)
	}
	defer ln.Close()

	got := ln.Addr().(*net.TCPAddr).Port
	if got == occupied {
		t.Errorf("bound the occupied preferred port %d, want a fallback from [%d, %d]", occupied, fallback, fallback+10)
	}
	if got < fallback || got > fallback+10 {
		t.Errorf("bound port %d, want it within the fallback range [%d, %d]", got, fallback, fallback+10)
	}
}

// GIVEN preferred <= 0 (no preference)
// WHEN AllocatePreferred runs
// THEN it goes straight to the range scan, same as calling Allocate
// directly — the ordinary, non-resume case is unchanged.
func TestAllocatePreferredZeroSkipsToRangeScan(t *testing.T) {
	fallback := freePort(t)

	ln, err := AllocatePreferred("127.0.0.1", 0, fallback, fallback+10)
	if err != nil {
		t.Fatalf("AllocatePreferred: %v", err)
	}
	defer ln.Close()

	got := ln.Addr().(*net.TCPAddr).Port
	if got < fallback || got > fallback+10 {
		t.Errorf("bound port %d, want it within [%d, %d]", got, fallback, fallback+10)
	}
}
