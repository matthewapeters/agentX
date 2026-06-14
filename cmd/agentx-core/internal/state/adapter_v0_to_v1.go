package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ToV1 converts v0 output state (index-based or turn-keyed) to v1 semantic format.
// v0 format examples:
//   - "0", "1" (simple numeric indices for turns)
//   - "turn_1:collapsed", "turn_1:focused_entry" (turn-explicit keys)
// v1 format uses semantic keys: "session_<id>:turn_<idx>:<entry_kind>"
//
// Graceful degradation: if v0 is nil or empty, returns empty v1 state (no error).
func (a *outputAdapter) ToV1(ctx context.Context, v0 map[string]interface{}) (*VersionedAppletState, error) {
	// Graceful fallback for nil or empty map
	if v0 == nil {
		return NewVersionedAppletState(a.widget, "global", []byte("{}")), nil
	}

	v1Data := make(map[string]interface{})

	// Process v0 keys and rebuild with semantic structure
	for k, v := range v0 {
		// Try to match turn-keyed patterns: "turn_N:kind" or "N"
		semanticKey := a.rebuildOutputKey(k)
		if semanticKey != "" {
			v1Data[semanticKey] = v
		}
	}

	// Marshal to JSON payload
	payload, err := json.Marshal(v1Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal output v1 data: %w", err)
	}

	return NewVersionedAppletState(a.widget, "global", payload), nil
}

// rebuildOutputKey converts v0 key format to v1 semantic key.
// v0 formats:
//   - "turn_N:collapsed" or "turn_N:focused_entry" → "session_default:turn_N:collapsed"
//   - "N" (numeric index) → "session_default:turn_N:collapsed" (heuristic)
// If key doesn't match patterns, returns empty string (skip).
func (a *outputAdapter) rebuildOutputKey(v0Key string) string {
	// Pattern 1: "turn_N:kind" → semantic format
	if strings.HasPrefix(v0Key, "turn_") {
		// Split on first colon after "turn_"
		parts := strings.SplitN(v0Key, ":", 2)
		if len(parts) == 2 {
			// parts[0] = "turn_N", parts[1] = "kind"
			return fmt.Sprintf("session_default:%s:%s", parts[0], parts[1])
		}
	}

	// Pattern 2: simple numeric index "0", "1", etc.
	// Preserve as-is for index-based keys; rebuild in context of full state
	if isNumericKey(v0Key) {
		// Keep numeric indices as semantic markers (caller context needed for full rebuild)
		return fmt.Sprintf("session_default:turn_%s:data", v0Key)
	}

	// Pattern 3: direct key names like "collapsed", "focused_entry"
	// Map to default turn context
	if v0Key == "collapsed" || v0Key == "focused_entry" || v0Key == "scroll_pos" {
		return fmt.Sprintf("session_default:turn_0:%s", v0Key)
	}

	// Unknown pattern; skip
	return ""
}

// isNumericKey checks if key is purely numeric.
func isNumericKey(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// ToV1 converts v0 context state to v1 semantic format.
// v0 format examples:
//   - "scroll_pos", "sort_order", "filter_session"
// v1 format: "context:scroll_pos", "context:sort_order", etc.
func (a *contextAdapter) ToV1(ctx context.Context, v0 map[string]interface{}) (*VersionedAppletState, error) {
	// Graceful fallback for nil or empty map
	if v0 == nil {
		return NewVersionedAppletState(a.widget, "global", []byte("{}")), nil
	}

	v1Data := make(map[string]interface{})

	// Map v0 keys to v1 semantic keys
	contextKeys := []string{"scroll_pos", "sort_order", "filter_session", "expand_state", "selected_node"}
	for _, key := range contextKeys {
		if val, ok := v0[key]; ok {
			v1Data[fmt.Sprintf("context:%s", key)] = val
		}
	}

	// Preserve any other unrecognized keys with "context:" prefix (graceful)
	for k, v := range v0 {
		if _, isKnown := mapToSlice(contextKeys, k); !isKnown {
			v1Data[fmt.Sprintf("context:%s", k)] = v
		}
	}

	// Marshal to JSON payload
	payload, err := json.Marshal(v1Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal context v1 data: %w", err)
	}

	return NewVersionedAppletState(a.widget, "global", payload), nil
}

// ToV1 converts v0 input state to v1 semantic format.
// v0 format examples:
//   - "compose_scroll", "history_entries", "cursor_pos"
// v1 format: "input:compose_scroll", "input:history_entries", etc.
func (a *inputAdapter) ToV1(ctx context.Context, v0 map[string]interface{}) (*VersionedAppletState, error) {
	// Graceful fallback for nil or empty map
	if v0 == nil {
		return NewVersionedAppletState(a.widget, "global", []byte("{}")), nil
	}

	v1Data := make(map[string]interface{})

	// Map v0 keys to v1 semantic keys
	inputKeys := []string{"compose_scroll", "history_entries", "cursor_pos", "current_text", "input_focused"}
	for _, key := range inputKeys {
		if val, ok := v0[key]; ok {
			v1Data[fmt.Sprintf("input:%s", key)] = val
		}
	}

	// Preserve any other unrecognized keys with "input:" prefix (graceful)
	for k, v := range v0 {
		if _, isKnown := mapToSlice(inputKeys, k); !isKnown {
			v1Data[fmt.Sprintf("input:%s", k)] = v
		}
	}

	// Marshal to JSON payload
	payload, err := json.Marshal(v1Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input v1 data: %w", err)
	}

	return NewVersionedAppletState(a.widget, "global", payload), nil
}

// mapToSlice checks if key exists in slice and returns the key with found status.
func mapToSlice(keys []string, target string) (string, bool) {
	for _, k := range keys {
		if k == target {
			return k, true
		}
	}
	return "", false
}
