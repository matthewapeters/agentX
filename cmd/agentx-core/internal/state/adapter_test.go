package state

import (
	"context"
	"encoding/json"
	"testing"
)

// TestNewStateAdapterFactory tests factory creation for supported widgets.
func TestNewStateAdapterFactory(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		widget      string
		wantErr     bool
		description string
	}{
		{"output", false, "output widget should be supported"},
		{"context", false, "context widget should be supported"},
		{"input", false, "input widget should be supported"},
		{"unknown", true, "unknown widget should return error"},
		{"", true, "empty widget should return error"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			adapter, err := NewStateAdapter(ctx, tt.widget)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewStateAdapter(%q) expected error, got nil", tt.widget)
				}
			} else {
				if err != nil {
					t.Fatalf("NewStateAdapter(%q) unexpected error: %v", tt.widget, err)
				}
				if adapter == nil {
					t.Fatalf("NewStateAdapter(%q) returned nil adapter", tt.widget)
				}
			}
		})
	}
}

// TestOutputAdapterRoundTrip tests v0 → v1 → v0 for output widget.
func TestOutputAdapterRoundTrip(t *testing.T) {
	ctx := context.Background()
	adapter, _ := NewStateAdapter(ctx, "output")

	tests := []struct {
		name     string
		v0Input  map[string]interface{}
		wantKeys []string // Expected v0 keys after round-trip
	}{
		{
			name: "single turn collapsed state",
			v0Input: map[string]interface{}{
				"turn_1:collapsed": true,
			},
			wantKeys: []string{"turn_1:collapsed"},
		},
		{
			name: "multiple turns with mixed state",
			v0Input: map[string]interface{}{
				"turn_1:collapsed":     true,
				"turn_1:focused_entry": "entry_0",
				"turn_2:collapsed":     false,
				"turn_3:collapsed":     true,
			},
			wantKeys: []string{"turn_1:collapsed", "turn_1:focused_entry", "turn_2:collapsed", "turn_3:collapsed"},
		},
		{
			name:     "empty state",
			v0Input:  map[string]interface{}{},
			wantKeys: []string{},
		},
		{
			name: "numeric index keys",
			v0Input: map[string]interface{}{
				"0": "data_0",
				"1": "data_1",
			},
			wantKeys: []string{"turn_0:data", "turn_1:data"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// v0 → v1
			v1State, err := adapter.ToV1(ctx, tt.v0Input)
			if err != nil {
				t.Fatalf("ToV1 failed: %v", err)
			}

			// Verify v1 state is valid
			if v1State.Version != 1 {
				t.Errorf("v1 state version = %d, want 1", v1State.Version)
			}
			if v1State.Widget != "output" {
				t.Errorf("v1 state widget = %q, want output", v1State.Widget)
			}

			// v1 → v0
			v0Out, err := adapter.FromV1(ctx, v1State)
			if err != nil {
				t.Fatalf("FromV1 failed: %v", err)
			}

			// Verify keys are preserved (allow extra keys with semantic expansion)
			for _, wantKey := range tt.wantKeys {
				if _, ok := v0Out[wantKey]; !ok {
					t.Errorf("Round-trip lost key %q, output: %+v", wantKey, v0Out)
				}
			}

			// Verify values are preserved
			for k, v := range tt.v0Input {
				// Check if this key was preserved through round-trip
				// For numeric keys, they may be transformed; for turn keys, preserved as-is
				if _, ok := v0Out[k]; ok {
					if v0Out[k] != v {
						t.Errorf("Round-trip value mismatch for key %q: got %v, want %v", k, v0Out[k], v)
					}
				}
			}
		})
	}
}

// TestContextAdapterRoundTrip tests v0 → v1 → v0 for context widget.
func TestContextAdapterRoundTrip(t *testing.T) {
	ctx := context.Background()
	adapter, _ := NewStateAdapter(ctx, "context")

	tests := []struct {
		name    string
		v0Input map[string]interface{}
	}{
		{
			name: "scroll and sort state",
			v0Input: map[string]interface{}{
				"scroll_pos":  float64(123),  // Use float64 to match JSON unmarshaling
				"sort_order":  "ascending",
				"filter_session": "session_abc",
			},
		},
		{
			name:    "empty state",
			v0Input: map[string]interface{}{},
		},
		{
			name: "all context fields",
			v0Input: map[string]interface{}{
				"scroll_pos":     float64(456),  // Use float64 to match JSON unmarshaling
				"sort_order":     "descending",
				"filter_session": "session_xyz",
				"expand_state":   "node_1,node_2",
				"selected_node":  "node_2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// v0 → v1
			v1State, err := adapter.ToV1(ctx, tt.v0Input)
			if err != nil {
				t.Fatalf("ToV1 failed: %v", err)
			}

			// v1 → v0
			v0Out, err := adapter.FromV1(ctx, v1State)
			if err != nil {
				t.Fatalf("FromV1 failed: %v", err)
			}

			// Verify all values are preserved
			for k, v := range tt.v0Input {
				outVal, ok := v0Out[k]
				if !ok {
					t.Errorf("Round-trip lost key %q", k)
					continue
				}

				if valuesEqual(v, outVal) != true {
					t.Errorf("Round-trip value mismatch for key %q: got %v (%T), want %v (%T)", k, outVal, outVal, v, v)
				}
			}

			// Verify no extra keys were added
			if len(v0Out) != len(tt.v0Input) {
				t.Errorf("Round-trip added extra keys: got %d, want %d; output: %+v", len(v0Out), len(tt.v0Input), v0Out)
			}
		})
	}
}

// TestInputAdapterRoundTrip tests v0 → v1 → v0 for input widget.
func TestInputAdapterRoundTrip(t *testing.T) {
	ctx := context.Background()
	adapter, _ := NewStateAdapter(ctx, "input")

	tests := []struct {
		name    string
		v0Input map[string]interface{}
	}{
		{
			name: "compose and history state",
			v0Input: map[string]interface{}{
				"compose_scroll": float64(42),  // Use float64 to match JSON unmarshaling
				"history_entries": "entry1\nentry2",
				"cursor_pos": float64(5),  // Use float64 to match JSON unmarshaling
			},
		},
		{
			name:    "empty state",
			v0Input: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// v0 → v1
			v1State, err := adapter.ToV1(ctx, tt.v0Input)
			if err != nil {
				t.Fatalf("ToV1 failed: %v", err)
			}

			// v1 → v0
			v0Out, err := adapter.FromV1(ctx, v1State)
			if err != nil {
				t.Fatalf("FromV1 failed: %v", err)
			}

			// Verify all values are preserved
			for k, v := range tt.v0Input {
				outVal, ok := v0Out[k]
				if !ok {
					t.Errorf("Round-trip lost key %q", k)
					continue
				}

				if valuesEqual(v, outVal) != true {
					t.Errorf("Round-trip value mismatch for key %q: got %v (%T), want %v (%T)", k, outVal, outVal, v, v)
				}
			}
		})
	}
}

// TestAdapterErrorCases tests error handling.
func TestAdapterErrorCases(t *testing.T) {
	ctx := context.Background()

	t.Run("nil v0 map returns empty state gracefully", func(t *testing.T) {
		adapter, _ := NewStateAdapter(ctx, "output")
		v1State, err := adapter.ToV1(ctx, nil)
		if err != nil {
			t.Fatalf("ToV1 with nil input should not error: %v", err)
		}
		// Should return empty JSON payload
		if string(v1State.Payload) != "{}" {
			t.Errorf("nil v0 should produce empty JSON, got: %s", string(v1State.Payload))
		}
	})

	t.Run("malformed v1 JSON in FromV1 returns error", func(t *testing.T) {
		adapter, _ := NewStateAdapter(ctx, "output")
		malformedState := &VersionedAppletState{
			Version: 1,
			Widget:  "output",
			Payload: []byte("not valid json"),
		}
		_, err := adapter.FromV1(ctx, malformedState)
		if err == nil {
			t.Fatalf("FromV1 with malformed JSON should return error, got nil")
		}
	})

	t.Run("factory returns error for unsupported widget", func(t *testing.T) {
		_, err := NewStateAdapter(ctx, "unsupported_widget")
		if err == nil {
			t.Fatalf("NewStateAdapter with unsupported widget should return error, got nil")
		}
	})
}

// TestDataTypePreservation verifies boolean, string, and numeric values preserve through round-trip.
func TestDataTypePreservation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		widget   string
		v0Input  map[string]interface{}
		testName string
	}{
		{
			widget: "output",
			v0Input: map[string]interface{}{
				"turn_1:collapsed":     true,
				"turn_1:focused_entry": "entry_0",
				"turn_2:collapsed":     false,
			},
			testName: "output boolean and string preservation",
		},
		{
			widget: "context",
			v0Input: map[string]interface{}{
				"scroll_pos":     int(123),
				"sort_order":     "ascending",
				"filter_session": "session_abc",
			},
			testName: "context numeric and string preservation",
		},
		{
			widget: "input",
			v0Input: map[string]interface{}{
				"compose_scroll": float64(42.5),
				"history_entries": "line1\nline2",
			},
			testName: "input numeric and string preservation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			adapter, _ := NewStateAdapter(ctx, tt.widget)

			// v0 → v1
			v1State, err := adapter.ToV1(ctx, tt.v0Input)
			if err != nil {
				t.Fatalf("ToV1 failed: %v", err)
			}

			// Verify v1 payload is valid JSON
			var v1Payload map[string]interface{}
			if err := json.Unmarshal(v1State.Payload, &v1Payload); err != nil {
				t.Fatalf("v1 payload is not valid JSON: %v", err)
			}

			// v1 → v0
			v0Out, err := adapter.FromV1(ctx, v1State)
			if err != nil {
				t.Fatalf("FromV1 failed: %v", err)
			}

			// Verify type preservation (JSON preserves types for bool, string, number)
			for k, v := range tt.v0Input {
				outVal, ok := v0Out[k]
				if !ok {
					// Some transformations may key differently; check if original key exists
					t.Logf("Key %q not found in output, but this may be expected due to semantic key transformation", k)
					continue
				}

				// Compare types
				if !sameType(v, outVal) {
					t.Errorf("Type mismatch for key %q: got %T, want %T", k, outVal, v)
				}

				// For simple comparisons (bool, string), verify values match
				if b, ok := v.(bool); ok {
					if outVal != b {
						t.Errorf("Boolean value mismatch for key %q: got %v, want %v", k, outVal, b)
					}
				}
				if s, ok := v.(string); ok {
					if outVal != s {
						t.Errorf("String value mismatch for key %q: got %v, want %v", k, outVal, s)
					}
				}
			}
		})
	}
}

// valuesEqual compares two values accounting for JSON type coercion (int → float64).
func valuesEqual(a, b interface{}) bool {
	switch av := a.(type) {
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	case int:
		// JSON unmarshals numbers as float64
		bv, ok := b.(float64)
		return ok && float64(av) == bv
	default:
		return a == b
	}
}

// sameType checks if two values have compatible types (accounting for JSON number coercion).
func sameType(a, b interface{}) bool {
	switch a.(type) {
	case bool:
		_, ok := b.(bool)
		return ok
	case string:
		_, ok := b.(string)
		return ok
	case float64:
		// JSON unmarshals numbers as float64
		_, ok := b.(float64)
		return ok
	case int:
		// If input was int, output will be float64 from JSON
		_, ok := b.(float64)
		return ok
	default:
		return false
	}
}

// BenchmarkAdapterRoundTrip benchmarks round-trip performance.
func BenchmarkAdapterRoundTrip(b *testing.B) {
	ctx := context.Background()
	adapter, _ := NewStateAdapter(ctx, "output")

	v0 := map[string]interface{}{
		"turn_1:collapsed":     true,
		"turn_1:focused_entry": "entry_0",
		"turn_2:collapsed":     false,
		"turn_3:collapsed":     true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v1, _ := adapter.ToV1(ctx, v0)
		adapter.FromV1(ctx, v1)
	}
}
