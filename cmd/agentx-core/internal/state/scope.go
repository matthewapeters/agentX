package state

import (
	"fmt"
	"strings"
)

// Scope constants for applet state visibility.
const (
	// APPLET_SCOPE_GLOBAL indicates state visible across all sessions.
	APPLET_SCOPE_GLOBAL = "global"

	// APPLET_SCOPE_SESSION is the prefix for session-scoped state.
	// Format: "session:<sessionID>"
	APPLET_SCOPE_SESSION = "session"

	// sessionScopePrefix is the separator used in session scope format.
	sessionScopePrefix = "session:"
)

// ScopeForSession returns a session-scoped string for the given sessionID.
// Format: "session:<sessionID>"
func ScopeForSession(sessionID string) string {
	return sessionScopePrefix + sessionID
}

// ParseSessionScope parses a scope string and returns the sessionID if it's a session scope.
// Returns (sessionID, isSession, error).
// If scope is "session:<sessionID>", returns (sessionID, true, nil).
// If scope is "global" or unrecognized, returns ("", false, nil).
// Returns error only if scope is malformed (e.g., "session:" with no ID).
func ParseSessionScope(scope string) (string, bool, error) {
	if scope == APPLET_SCOPE_GLOBAL {
		return "", false, nil
	}

	if !strings.HasPrefix(scope, sessionScopePrefix) {
		// Not a session scope, not an error
		return "", false, nil
	}

	sessionID := strings.TrimPrefix(scope, sessionScopePrefix)
	if sessionID == "" {
		return "", false, fmt.Errorf("invalid session scope: missing sessionID after '%s'", sessionScopePrefix)
	}

	return sessionID, true, nil
}
