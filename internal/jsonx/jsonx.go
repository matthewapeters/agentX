// Package jsonx provides tolerant extraction of JSON from model output. Models often wrap
// their JSON in markdown code fences or surrounding prose despite being told to emit bare
// JSON, so a strict json.Unmarshal on the raw reply fails on the leading fence. FirstObject
// recovers the payload. It is the single source of truth for this concern — the classifier,
// the tool proposer, and the planner all use it, so no parser can drift back into
// fence-intolerance (the defect that failed decomposition in session calm-pebble, where the
// planner's own naive Unmarshal choked on a ```json fence).
package jsonx

import "strings"

// FirstObject returns the first top-level balanced {...} object in s — skipping any leading
// prose or ``` code fences — or "" if there is none. Braces inside JSON string literals
// (and their backslash escapes) are ignored, so an object whose values contain "{" or "}"
// is still returned whole.
func FirstObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case ch == '\\':
				esc = true
			case ch == '"':
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
