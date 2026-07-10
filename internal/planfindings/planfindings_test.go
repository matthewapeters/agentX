package planfindings

import (
	"context"
	"testing"
)

func TestFromWithNoSourceAttached(t *testing.T) {
	if got := From(context.Background()); got != "" {
		t.Errorf("From on a bare context = %q, want \"\"", got)
	}
}

func TestWithSourceRoundTrip(t *testing.T) {
	ctx := WithSource(context.Background(), func() string { return "step one done" })
	if got := From(ctx); got != "step one done" {
		t.Errorf("From = %q, want %q", got, "step one done")
	}
}

// TestFromCallsSourceFreshEachTime confirms the source is invoked on every read (not
// snapshotted once at WithSource time), so later calls during the same drain see
// strictly more than earlier ones as the scheduler completes leaves.
func TestFromCallsSourceFreshEachTime(t *testing.T) {
	calls := 0
	ctx := WithSource(context.Background(), func() string {
		calls++
		if calls == 1 {
			return "first read"
		}
		return "second read"
	})
	if got := From(ctx); got != "first read" {
		t.Errorf("first From = %q, want %q", got, "first read")
	}
	if got := From(ctx); got != "second read" {
		t.Errorf("second From = %q, want %q", got, "second read")
	}
}

func TestFromWithNilSource(t *testing.T) {
	ctx := WithSource(context.Background(), nil)
	if got := From(ctx); got != "" {
		t.Errorf("From with a nil Source = %q, want \"\"", got)
	}
}
