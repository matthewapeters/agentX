package main

import "testing"

func TestResolveLayoutFileSelection_PrefersLayout(t *testing.T) {
	resolved, err := resolveLayoutFileSelection("custom.yaml", "")
	if err != nil {
		t.Fatalf("resolveLayoutFileSelection returned error: %v", err)
	}
	if resolved != "custom.yaml" {
		t.Fatalf("resolved layout mismatch: got %q want %q", resolved, "custom.yaml")
	}
}

func TestResolveLayoutFileSelection_AllowsLegacyLayoutFile(t *testing.T) {
	resolved, err := resolveLayoutFileSelection("", "legacy.yaml")
	if err != nil {
		t.Fatalf("resolveLayoutFileSelection returned error: %v", err)
	}
	if resolved != "legacy.yaml" {
		t.Fatalf("resolved layout mismatch: got %q want %q", resolved, "legacy.yaml")
	}
}

func TestResolveLayoutFileSelection_RejectsConflictingFlags(t *testing.T) {
	_, err := resolveLayoutFileSelection("a.yaml", "b.yaml")
	if err == nil {
		t.Fatalf("expected conflicting --layout and --layout-file to fail")
	}
}

func TestResolveLayoutFileSelection_EmptyWhenUnset(t *testing.T) {
	resolved, err := resolveLayoutFileSelection("", "")
	if err != nil {
		t.Fatalf("resolveLayoutFileSelection returned error: %v", err)
	}
	if resolved != "" {
		t.Fatalf("expected empty resolved layout when unset, got %q", resolved)
	}
}
