package http

import (
	"fmt"
	"net"
	"strings"
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

// Endpoint formats the http:// endpoint for a bound listener address.
func Endpoint(addr net.Addr) string {
	return "http://" + addr.String()
}

// LaunchCommand formats the canonical `agentx surface launch` command a user runs
// in another terminal to attach a surface of the given kind to this session.
func LaunchCommand(kind, session, endpoint, token string) string {
	return fmt.Sprintf("agentx surface launch %s --session %s --connect %s --token %s",
		kind, session, endpoint, token)
}

// LaunchHint renders the operator hint block listing how to attach external
// surfaces to a running session (one template line plus the available kinds).
func LaunchHint(session, endpoint, token string, kinds []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "External surfaces can attach to this session over %s\n", endpoint)
	fmt.Fprintf(&b, "  %s\n", LaunchCommand("<kind>", session, endpoint, token))
	if len(kinds) > 0 {
		fmt.Fprintf(&b, "  available kinds: %s", strings.Join(kinds, ", "))
	}
	return b.String()
}
