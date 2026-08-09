package http

import (
	"fmt"
	"net"
)

// Allocate binds the first available TCP port in the inclusive range
// [start, end] on host and returns the bound listener (TRN-4). Binding ascending
// gives a deterministic lowest-free-port preference; because the bind is the
// availability check, there is no time-of-check/time-of-use gap and concurrent
// agentx instances fall through to the next free port. It returns an error if the
// whole range is occupied.
func Allocate(host string, start, end int) (net.Listener, error) {
	if start <= 0 || end < start {
		return nil, fmt.Errorf("invalid transport port range [%d, %d]", start, end)
	}
	for port := start; port <= end; port++ {
		addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("transport port range [%d, %d] on %s exhausted", start, end, host)
}

// AllocatePreferred binds preferred on host if it's free; otherwise it falls
// back to Allocate's [start, end] range scan. preferred <= 0 skips straight
// to the range scan — the ordinary case, unchanged from Allocate alone. This
// exists for session resume
// (docs/architecture/behavior/session_resume.feature.md §5): the new
// process tries to reclaim the exact port the outgoing process was just
// using, so already-attached surfaces' disk-refreshing poll retries succeed
// against the endpoint they already have, rather than needing to discover a
// changed one from scratch.
func AllocatePreferred(host string, preferred, start, end int) (net.Listener, error) {
	if preferred > 0 {
		addr := net.JoinHostPort(host, fmt.Sprintf("%d", preferred))
		if ln, err := net.Listen("tcp", addr); err == nil {
			return ln, nil
		}
	}
	return Allocate(host, start, end)
}

// Endpoint formats the http:// endpoint for a bound listener address.
func Endpoint(addr net.Addr) string {
	return "http://" + addr.String()
}

// LaunchCommand formats the explicit `agentx surface launch` command with all
// connection flags. Used when a caller cannot rely on disk auto-resolution.
func LaunchCommand(kind, session, endpoint, token string) string {
	return fmt.Sprintf("agentx surface launch %s --session %s --connect %s --token %s",
		kind, session, endpoint, token)
}

// ShortLaunchCommand formats the flagless `agentx surface launch <kind>` command;
// the peer auto-resolves the endpoint and token from the session dir on disk (SS-5).
func ShortLaunchCommand(kind string) string {
	return "agentx surface launch " + kind
}

// SessionLaunchCommand formats `agentx surface launch <kind> --session <session>` —
// the form the launch-info widget advertises. It carries no token (resolved from
// disk, SS-5) but names the session so it is unambiguous when more than one agentx
// session is running. With a single session the bare ShortLaunchCommand also works.
func SessionLaunchCommand(kind, session string) string {
	if session == "" {
		return ShortLaunchCommand(kind)
	}
	return fmt.Sprintf("agentx surface launch %s --session %s", kind, session)
}
