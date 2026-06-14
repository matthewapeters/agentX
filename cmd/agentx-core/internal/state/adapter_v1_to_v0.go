package state

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// FromV1 reverses v1 semantic format back to v0 index-based format.
// Extracts semantic keys "session_default:turn_N:kind" and rebuilds as v0 indices.
func (a *outputAdapter) FromV1(ctx context.Context, v1 *VersionedAppletState) (map[string]interface{}, error) {
	// Decode payload from base64 (as per VersionedAppletState marshaling)
	payload := v1.Payload
	if len(payload) == 0 {
		return make(map[string]interface{}), nil
	}

	// Try to decode as base64 first (if it's a string field in JSON)
	// If direct unmarshal as JSON succeeds, use that
	var v1Data map[string]interface{}
	if err := json.Unmarshal(payload, &v1Data); err != nil {
		// If JSON unmarshal fails, it might be base64-encoded
		decoded, decErr := base64.StdEncoding.DecodeString(string(payload))
		if decErr != nil {
			return nil, fmt.Errorf("failed to decode output v1 payload: %w", err)
		}
		if err := json.Unmarshal(decoded, &v1Data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal decoded output v1 data: %w", err)
		}
	}

	v0Data := make(map[string]interface{})

	// Reverse map: extract semantic keys and rebuild v0 format
	for k, v := range v1Data {
		v0Key := a.reverseOutputKey(k)
		if v0Key != "" {
			v0Data[v0Key] = v
		}
	}

	return v0Data, nil
}

// reverseOutputKey converts v1 semantic key back to v0 index format.
// v1 format: "session_default:turn_N:kind"
// v0 format: "turn_N:kind" (or "N" for simple indices)
func (a *outputAdapter) reverseOutputKey(v1Key string) string {
	// Expected format: "session_default:turn_N:kind"
	parts := strings.Split(v1Key, ":")
	if len(parts) < 3 {
		return ""
	}

	// parts[0] = "session_default", parts[1] = "turn_N", parts[2:] = kind
	if parts[0] != "session_default" {
		return "" // Skip non-default sessions
	}

	if !strings.HasPrefix(parts[1], "turn_") {
		return ""
	}

	// Rebuild as "turn_N:kind"
	turnPart := parts[1]
	kind := strings.Join(parts[2:], ":")
	return fmt.Sprintf("%s:%s", turnPart, kind)
}

// FromV1 reverses v1 context format back to v0.
// Extracts keys with "context:" prefix and rebuilds as v0 format.
func (a *contextAdapter) FromV1(ctx context.Context, v1 *VersionedAppletState) (map[string]interface{}, error) {
	payload := v1.Payload
	if len(payload) == 0 {
		return make(map[string]interface{}), nil
	}

	// Decode payload (same logic as outputAdapter)
	var v1Data map[string]interface{}
	if err := json.Unmarshal(payload, &v1Data); err != nil {
		decoded, decErr := base64.StdEncoding.DecodeString(string(payload))
		if decErr != nil {
			return nil, fmt.Errorf("failed to decode context v1 payload: %w", err)
		}
		if err := json.Unmarshal(decoded, &v1Data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal decoded context v1 data: %w", err)
		}
	}

	v0Data := make(map[string]interface{})

	// Strip "context:" prefix from keys
	for k, v := range v1Data {
		if strings.HasPrefix(k, "context:") {
			v0Key := strings.TrimPrefix(k, "context:")
			v0Data[v0Key] = v
		}
	}

	return v0Data, nil
}

// FromV1 reverses v1 input format back to v0.
// Extracts keys with "input:" prefix and rebuilds as v0 format.
func (a *inputAdapter) FromV1(ctx context.Context, v1 *VersionedAppletState) (map[string]interface{}, error) {
	payload := v1.Payload
	if len(payload) == 0 {
		return make(map[string]interface{}), nil
	}

	// Decode payload (same logic as outputAdapter)
	var v1Data map[string]interface{}
	if err := json.Unmarshal(payload, &v1Data); err != nil {
		decoded, decErr := base64.StdEncoding.DecodeString(string(payload))
		if decErr != nil {
			return nil, fmt.Errorf("failed to decode input v1 payload: %w", err)
		}
		if err := json.Unmarshal(decoded, &v1Data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal decoded input v1 data: %w", err)
		}
	}

	v0Data := make(map[string]interface{})

	// Strip "input:" prefix from keys
	for k, v := range v1Data {
		if strings.HasPrefix(k, "input:") {
			v0Key := strings.TrimPrefix(k, "input:")
			v0Data[v0Key] = v
		}
	}

	return v0Data, nil
}
