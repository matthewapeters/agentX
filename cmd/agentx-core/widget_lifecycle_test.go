package main

import "testing"

func TestResolveWidgetCorePIDFromEnv_Valid(t *testing.T) {
	t.Setenv(widgetCorePIDEnv, "12345")
	if got := resolveWidgetCorePIDFromEnv(); got != 12345 {
		t.Fatalf("expected 12345, got %d", got)
	}
}

func TestResolveWidgetCorePIDFromEnv_Invalid(t *testing.T) {
	t.Setenv(widgetCorePIDEnv, "not-a-number")
	if got := resolveWidgetCorePIDFromEnv(); got != 0 {
		t.Fatalf("expected 0 for invalid pid, got %d", got)
	}
}

func TestResolveWidgetCorePIDFromEnv_Empty(t *testing.T) {
	t.Setenv(widgetCorePIDEnv, "")
	if got := resolveWidgetCorePIDFromEnv(); got != 0 {
		t.Fatalf("expected 0 for empty pid, got %d", got)
	}
}
