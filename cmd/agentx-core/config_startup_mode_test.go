package main

import (
	"os"
	"testing"
)

func TestNormalizeStartupMode(t *testing.T) {
	cases := []struct {
		raw      string
		expected string
		ok       bool
	}{
		{raw: "", expected: "default", ok: true},
		{raw: "default", expected: "default", ok: true},
		{raw: "visible-windows", expected: "visible-windows", ok: true},
		{raw: "VISIBLE-WINDOWS", expected: "visible-windows", ok: true},
		{raw: "broken", expected: "", ok: false},
	}

	for _, tc := range cases {
		got, ok := normalizeStartupMode(tc.raw)
		if got != tc.expected || ok != tc.ok {
			t.Fatalf("normalizeStartupMode(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.expected, tc.ok)
		}
	}
}

func TestResolveStartupModeDefault(t *testing.T) {
	oldValue, hadValue := os.LookupEnv(startupModeEnvKey)
	defer func() {
		if hadValue {
			_ = os.Setenv(startupModeEnvKey, oldValue)
		} else {
			_ = os.Unsetenv(startupModeEnvKey)
		}
	}()

	if err := os.Setenv(startupModeEnvKey, visibleWindowsStartupMode); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}
	if got := resolveStartupModeDefault(); got != visibleWindowsStartupMode {
		t.Fatalf("expected visible-windows default from env, got %q", got)
	}

	if err := os.Setenv(startupModeEnvKey, "broken"); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}
	if got := resolveStartupModeDefault(); got != defaultStartupMode {
		t.Fatalf("expected invalid env to fall back to default, got %q", got)
	}
}

func TestResolveDemoStartupMode(t *testing.T) {
	cases := []struct {
		name        string
		demoDefault  bool
		demoWindowed bool
		fallback    string
		expected    string
		wantErr     bool
	}{
		{name: "fallback default", fallback: defaultStartupMode, expected: defaultStartupMode},
		{name: "fallback windowed", fallback: visibleWindowsStartupMode, expected: visibleWindowsStartupMode},
		{name: "explicit default", demoDefault: true, expected: defaultStartupMode},
		{name: "explicit windowed", demoWindowed: true, expected: visibleWindowsStartupMode},
		{name: "conflicting flags", demoDefault: true, demoWindowed: true, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveDemoStartupMode(tc.demoDefault, tc.demoWindowed, tc.fallback)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Fatalf("resolveDemoStartupMode() = %q, want %q", got, tc.expected)
			}
		})
	}
}