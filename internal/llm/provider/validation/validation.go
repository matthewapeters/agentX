// Package validation provides type-appropriate validation helpers used by the
// config surface (PD-CONFIG) when the user confirms an edited value. Each
// function takes a string (the raw TUI input, already trimmed by the editor)
// and returns a human-readable error when the value does not match the field's
// rules — see PD-CONFIG-AF-006.
//
// These helpers are pure: they do not reach the network and do not depend on
// the orchestrator. That keeps the validation layer usable from both the
// transport POST /config path and, in the future, from the Bubble Tea TUI's
// client-side editor.
package validation

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Error is a typed validation error that carries the offending key name and a
// human-readable message. Callers (the transport, the future TUI) surface it
// as an inline error next to the editor.
type Error struct {
	// Field is the config key being validated (e.g. "ollama_host", "provider").
	Field string
	// Message is the human-readable rejection reason (e.g. "must be an integer").
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Message) }

// --- int ---

// ValidateInt checks that s is a valid integer within [min, max]. Empty strings
// are rejected with "is required"; non-integer strings are rejected with
// "must be an integer"; out-of-range values are rejected with "must be ≥ min
// and ≤ max".
func ValidateInt(s string, min, max int) *Error {
	s = strings.TrimSpace(s)
	if s == "" {
		return &Error{Field: "", Message: "is required"}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return &Error{Field: "", Message: "must be an integer"}
	}
	if n < min {
		return &Error{Field: "", Message: fmt.Sprintf("must be ≥ %d", min)}
	}
	if n > max {
		return &Error{Field: "", Message: fmt.Sprintf("must be ≤ %d", max)}
	}
	return nil
}

// --- non-empty string ---

// ValidateNonEmpty rejects empty strings (after trimming) with "is required".
// Used for any string field that must carry a value (model name, host, enum
// selection, color).
func ValidateNonEmpty(s string) *Error {
	if strings.TrimSpace(s) == "" {
		return &Error{Field: "", Message: "is required"}
	}
	return nil
}

// --- bool ---

// ValidateBool accepts only "true" / "false" (case-insensitive). Anything else
// is rejected with "must be true or false". Booleans are edit-mode toggles in
// the TUI, so free-text entry is never expected — this catches transport-side
// tampering.
func ValidateBool(s string) *Error {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "true", "false":
		return nil
	}
	return &Error{Field: "", Message: "must be true or false"}
}

// --- enum ---

// ValidateEnum rejects values not in allowed. It is case-insensitive and
// trims whitespace before matching, so a TUI that uppercases or adds a stray
// space still round-trips cleanly.
func ValidateEnum(s string, allowed []string) *Error {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return &Error{Field: "", Message: "is required"}
	}
	for _, a := range allowed {
		if strings.TrimSpace(strings.ToLower(a)) == s {
			return nil
		}
	}
	return &Error{Field: "", Message: fmt.Sprintf("must be one of: %s", strings.Join(allowed, ", "))}
}

// --- color ---

// ValidateColor accepts a named color (looked up against the built-in palette),
// an ANSI 256 index ("0"–"255"), or a hex color ("#RRGGBB").
func ValidateColor(s string) *Error {
	s = strings.TrimSpace(s)
	if s == "" {
		return &Error{Field: "", Message: "is required"}
	}
	// Hex: #RRGGBB.
	if strings.HasPrefix(s, "#") && len(s) == 7 {
		var r, g, b int
		if _, err := fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &b); err == nil {
			return nil
		}
		return &Error{Field: "", Message: "must be #RRGGBB (valid hex)"}
	}
	// ANSI 256: pure digits, 0–255.
	if isAllDigits(s) {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > 255 {
			return &Error{Field: "", Message: "must be 0–255"}
		}
		return nil
	}
	// Named color (case-insensitive). The caller's palette is the source of
	// truth — we accept any non-empty name here and let the SGR resolver map
	// it (falling back to the default if unknown). This is intentionally
	// permissive: users can invent custom names like "my-blue" and have them
	// resolved later.
	if isValidIdentifier(s) {
		return nil
	}
	return &Error{Field: "", Message: "must be a name, ANSI index (0–255), or hex (#RRGGBB)"}
}

// --- host ---

// ValidateHost checks that s is a non-empty host:port string. It does NOT probe
// the endpoint — that is done by TestHost. ValidateHost only rejects empty
// input and obviously malformed addresses.
func ValidateHost(s string) *Error {
	s = strings.TrimSpace(s)
	if s == "" {
		return &Error{Field: "", Message: "is required"}
	}
	// Accept "host:port", "[ipv6]:port", "host", or "host:port".
	if _, _, err := net.SplitHostPort(s); err != nil {
		// Could be host-only (no port). Accept if it's a valid hostname/IP.
		if net.ParseIP(s) == nil {
			// Not an IP — try as hostname.
			if _, err := url.Parse("http://" + s); err != nil {
				return &Error{Field: "", Message: "must be host:port or a hostname"}
			}
		}
	}
	return nil
}

// --- model name ---

// ValidateModelName rejects empty strings and strings containing characters
// that are not allowed in Ollama/llama.cpp model identifiers (alphanumerics,
// hyphens, underscores, colons, dots, slashes). The colon-separator format
// ("model:tag") is accepted for Ollama.
func ValidateModelName(s string) *Error {
	s = strings.TrimSpace(s)
	if s == "" {
		return &Error{Field: "", Message: "is required"}
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == ':' || r == '.' || r == '/'
		if !ok {
			return &Error{Field: "", Message: "contains invalid characters"}
		}
	}
	return nil
}

// --- helpers ---

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isValidIdentifier reports whether s is a non-empty identifier made of
// letters, digits, underscores, and hyphens. Color names use this shape.
func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == ' '
		if !ok {
			return false
		}
	}
	return true
}
